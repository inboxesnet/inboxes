package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *PgStore) HasAPIKey(ctx context.Context, orgID string) (bool, error) {
	var hasKey bool
	err := s.q.QueryRow(ctx,
		"SELECT (resend_api_key_encrypted IS NOT NULL) FROM orgs WHERE id = $1", orgID,
	).Scan(&hasKey)
	return hasKey, err
}

func (s *PgStore) CountVisibleDomains(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.q.QueryRow(ctx,
		"SELECT COUNT(*) FROM domains WHERE org_id = $1 AND hidden = false", orgID,
	).Scan(&count)
	return count, err
}

func (s *PgStore) GetActiveSyncJob(ctx context.Context, orgID string) (jobID, phase string, err error) {
	err = s.q.QueryRow(ctx,
		`SELECT id, phase FROM sync_jobs WHERE org_id = $1 AND status IN ('pending', 'running')
		 ORDER BY created_at DESC LIMIT 1`, orgID,
	).Scan(&jobID, &phase)
	return
}

func (s *PgStore) CountEmails(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.q.QueryRow(ctx,
		"SELECT COUNT(*) FROM emails WHERE org_id = $1", orgID,
	).Scan(&count)
	return count, err
}

func (s *PgStore) StoreEncryptedAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2,
		 resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
		ciphertext, iv, tag, orgID,
	)
	return err
}

func (s *PgStore) UpsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, records json.RawMessage, order int) (string, error) {
	var domainID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO domains (org_id, domain, resend_domain_id, status, dns_records, display_order)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (domain) WHERE status NOT IN ('deleted') DO UPDATE SET resend_domain_id = $3, status = $4, dns_records = $5, updated_at = now()
		 RETURNING id`,
		orgID, domain, resendDomainID, status, records, order,
	).Scan(&domainID)
	return domainID, err
}

func (s *PgStore) SelectDomains(ctx context.Context, orgID string, domainIDs []string) error {
	// Hide all domains for this org
	_, err := s.q.Exec(ctx,
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`,
		orgID,
	)
	if err != nil {
		return err
	}
	// Unhide selected domains
	if len(domainIDs) > 0 {
		_, err = s.q.Exec(ctx,
			`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
			orgID, domainIDs,
		)
	}
	return err
}

func (s *PgStore) StoreWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1,
		 resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = $2, resend_webhook_secret_iv = $3, resend_webhook_secret_tag = $4,
		 updated_at = now() WHERE id = $5`,
		webhookID, encSecret, encIV, encTag, orgID,
	)
	return err
}

func (s *PgStore) GetDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT da.id, da.address, da.local_part, da.type,
		        (SELECT COUNT(*) FROM emails e
		         WHERE e.domain_id = da.domain_id
		           AND (e.from_address = da.address
		                OR e.to_addresses @> to_jsonb(da.address)
		                OR e.cc_addresses @> to_jsonb(da.address))
		        ) as email_count
		 FROM discovered_addresses da
		 JOIN domains d ON d.id = da.domain_id
		 WHERE d.org_id = $1 AND d.hidden = false
		 ORDER BY email_count DESC
		 LIMIT 500`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) SetupAddress(ctx context.Context, orgID, userID, address, addrType, name string) error {
	parts := strings.Split(address, "@")
	if len(parts) != 2 {
		return nil
	}
	domain := parts[1]

	var domainID string
	if err := s.q.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
		orgID, domain,
	).Scan(&domainID); err != nil {
		return nil // unknown domain, skip
	}

	switch addrType {
	case "individual":
		if name == "" {
			name = parts[0]
		}
		// Create placeholder user for this individual address
		var placeholderUserID string
		if err := s.q.QueryRow(ctx,
			`INSERT INTO users (org_id, email, name, status)
			 VALUES ($1, $2, $3, 'placeholder')
			 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			orgID, address, name,
		).Scan(&placeholderUserID); err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}
		// Reassign emails and threads to the placeholder user
		if _, err := s.q.Exec(ctx,
			`UPDATE emails SET user_id = $1 WHERE domain_id = $2 AND from_address = $3`,
			placeholderUserID, domainID, address,
		); err != nil {
			return fmt.Errorf("reassign emails: %w", err)
		}
		if _, err := s.q.Exec(ctx,
			`UPDATE threads SET user_id = $1 WHERE domain_id = $2 AND id IN (
				SELECT DISTINCT thread_id FROM emails WHERE domain_id = $2 AND user_id = $1
			)`, placeholderUserID, domainID,
		); err != nil {
			return fmt.Errorf("reassign threads: %w", err)
		}
		// Create alias so this address is visible in the sidebar
		var aliasID string
		if err := s.q.QueryRow(ctx,
			`INSERT INTO aliases (org_id, domain_id, address, name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (org_id, address) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			orgID, domainID, address, name,
		).Scan(&aliasID); err != nil {
			return fmt.Errorf("upsert alias for individual: %w", err)
		}
		// Assign admin user to the alias
		if _, err := s.q.Exec(ctx,
			`INSERT INTO alias_users (alias_id, user_id, can_send_as)
			 VALUES ($1, $2, true)
			 ON CONFLICT (alias_id, user_id) DO NOTHING`,
			aliasID, userID,
		); err != nil {
			return fmt.Errorf("assign alias to admin: %w", err)
		}
		// Assign placeholder user to the alias
		if _, err := s.q.Exec(ctx,
			`INSERT INTO alias_users (alias_id, user_id, can_send_as)
			 VALUES ($1, $2, true)
			 ON CONFLICT (alias_id, user_id) DO NOTHING`,
			aliasID, placeholderUserID,
		); err != nil {
			return fmt.Errorf("assign alias to placeholder: %w", err)
		}
		// Backfill alias thread labels
		if _, err := s.q.Exec(ctx,
			`INSERT INTO thread_labels (thread_id, org_id, label)
			 SELECT DISTINCT e.thread_id, e.org_id, 'alias:' || $1
			 FROM emails e
			 WHERE e.domain_id = $2
			   AND (e.from_address = $1 OR e.to_addresses @> $3::jsonb OR e.cc_addresses @> $3::jsonb)
			 ON CONFLICT DO NOTHING`,
			address, domainID, fmt.Sprintf(`["%s"]`, address),
		); err != nil {
			return fmt.Errorf("backfill alias labels: %w", err)
		}
		// Update discovered_addresses with both user and alias
		if _, err := s.q.Exec(ctx,
			`UPDATE discovered_addresses SET type = 'individual', user_id = $1, alias_id = $2
			 WHERE domain_id = $3 AND address = $4`,
			placeholderUserID, aliasID, domainID, address,
		); err != nil {
			return fmt.Errorf("update discovered_addresses: %w", err)
		}

	case "group":
		if name == "" {
			name = parts[0]
		}
		var aliasID string
		if err := s.q.QueryRow(ctx,
			`INSERT INTO aliases (org_id, domain_id, address, name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (org_id, address) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			orgID, domainID, address, name,
		).Scan(&aliasID); err != nil {
			return fmt.Errorf("upsert alias: %w", err)
		}
		if _, err := s.q.Exec(ctx,
			`INSERT INTO alias_users (alias_id, user_id, can_send_as)
			 VALUES ($1, $2, true)
			 ON CONFLICT (alias_id, user_id) DO NOTHING`,
			aliasID, userID,
		); err != nil {
			return fmt.Errorf("assign alias to user: %w", err)
		}
		if _, err := s.q.Exec(ctx,
			`UPDATE discovered_addresses SET type = 'group', alias_id = $1
			 WHERE domain_id = $2 AND address = $3`,
			aliasID, domainID, address,
		); err != nil {
			return fmt.Errorf("update discovered_addresses: %w", err)
		}

	case "skip":
		if _, err := s.q.Exec(ctx,
			`UPDATE discovered_addresses SET type = 'unclaimed'
			 WHERE domain_id = $1 AND address = $2`,
			domainID, address,
		); err != nil {
			return fmt.Errorf("mark unclaimed: %w", err)
		}
	}

	return nil
}

func (s *PgStore) CompleteOnboarding(ctx context.Context, orgID string) error {
	_, err := s.q.Exec(ctx,
		"UPDATE orgs SET onboarding_completed = true, updated_at = now() WHERE id = $1",
		orgID,
	)
	return err
}

func (s *PgStore) GetFirstDomainID(ctx context.Context, orgID string) (string, error) {
	var domainID string
	err := s.q.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		orgID,
	).Scan(&domainID)
	return domainID, err
}
