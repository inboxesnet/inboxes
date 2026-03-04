package store

import (
	"context"
	"encoding/json"
	"log/slog"
)

func (s *PgStore) ListDomains(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error) {
	var query string
	if includeHidden {
		query = `SELECT id, org_id, domain, resend_domain_id, status,
				display_order, dns_records, hidden, created_at
		 FROM domains WHERE org_id = $1 ORDER BY display_order, created_at`
	} else {
		query = `SELECT id, org_id, domain, resend_domain_id, status,
				display_order, dns_records, created_at
		 FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order, created_at`
	}
	rows, err := s.q.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) InsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, dnsRecords json.RawMessage) (string, error) {
	var id string
	err := s.q.QueryRow(ctx,
		`INSERT INTO domains (org_id, domain, resend_domain_id, status, dns_records)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, domain, resendDomainID, status, dnsRecords,
	).Scan(&id)
	return id, err
}

func (s *PgStore) GetResendDomainID(ctx context.Context, domainID, orgID string) (string, error) {
	var resendID string
	err := s.q.QueryRow(ctx,
		`SELECT resend_domain_id FROM domains WHERE id = $1 AND org_id = $2`,
		domainID, orgID).Scan(&resendID)
	return resendID, err
}

func (s *PgStore) UpdateDomainStatus(ctx context.Context, domainID, status string, dnsRecords json.RawMessage) error {
	_, err := s.q.Exec(ctx,
		`UPDATE domains SET status = $1, dns_records = $2, updated_at = now() WHERE id = $3`,
		status, dnsRecords, domainID)
	return err
}

func (s *PgStore) ReorderDomains(ctx context.Context, orgID string, order []DomainOrder) error {
	for _, item := range order {
		if _, err := s.q.Exec(ctx,
			`UPDATE domains SET display_order = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
			item.Order, item.ID, orgID); err != nil {
			slog.Error("domain: reorder update failed", "domain_id", item.ID, "error", err)
		}
	}
	return nil
}

func (s *PgStore) GetUnreadCounts(ctx context.Context, orgID, userID string) (map[string]int, error) {
	rows, err := s.q.Query(ctx,
		`SELECT t.domain_id, COALESCE(SUM(t.unread_count), 0)
		 FROM threads t
		 JOIN thread_labels tl ON tl.thread_id = t.id
		 JOIN domains d ON d.id = t.domain_id
		 WHERE d.org_id = $1 AND t.user_id = $2 AND tl.label = 'inbox' AND t.deleted_at IS NULL
		 AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))
		 GROUP BY t.domain_id`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var domainID string
		var count int
		if rows.Scan(&domainID, &count) == nil {
			counts[domainID] = count
		}
	}
	return counts, nil
}

func (s *PgStore) UpdateDomainVisibility(ctx context.Context, orgID string, visibleIDs []string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`, orgID)
	if err != nil {
		return err
	}
	if len(visibleIDs) > 0 {
		_, err = s.q.Exec(ctx,
			`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
			orgID, visibleIDs)
	}
	return err
}

func (s *PgStore) SyncDomains(ctx context.Context, orgID string, resendDomains []ResendDomainInfo) error {
	resendDomainNames := make(map[string]bool, len(resendDomains))
	for _, rd := range resendDomains {
		resendDomainNames[rd.Name] = true
		if _, err := s.q.Exec(ctx,
			`INSERT INTO domains (org_id, domain, resend_domain_id, status, hidden)
			 VALUES ($1, $2, $3, $4, true)
			 ON CONFLICT (domain) WHERE status NOT IN ('deleted') DO UPDATE SET status = EXCLUDED.status, updated_at = now()`,
			orgID, rd.Name, rd.ID, rd.Status); err != nil {
			slog.Error("domain: sync upsert failed", "domain", rd.Name, "error", err)
		}
	}

	localRows, err := s.q.Query(ctx,
		`SELECT id, domain, status FROM domains WHERE org_id = $1`, orgID)
	if err != nil {
		return err
	}
	defer localRows.Close()
	for localRows.Next() {
		var localID, localDomain, localStatus string
		if localRows.Scan(&localID, &localDomain, &localStatus) == nil {
			if !resendDomainNames[localDomain] && localStatus != "disconnected" {
				if _, err := s.q.Exec(ctx,
					`UPDATE domains SET status = 'disconnected', updated_at = now() WHERE id = $1`,
					localID); err != nil {
					slog.Error("domain: disconnect update failed", "domain_id", localID, "error", err)
				}
			}
		}
	}
	return nil
}

func (s *PgStore) SoftDeleteDomain(ctx context.Context, domainID, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE domains SET status = 'deleted', hidden = true, updated_at = now() WHERE id = $1 AND org_id = $2`,
		domainID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) CascadeDeleteDomain(ctx context.Context, domainID string) error {
	if _, err := s.q.Exec(ctx, `UPDATE aliases SET deleted_at = now() WHERE domain_id = $1 AND deleted_at IS NULL`, domainID); err != nil {
		slog.Error("domain: cascade delete aliases failed", "domain_id", domainID, "error", err)
	}
	if _, err := s.q.Exec(ctx, `DELETE FROM alias_users WHERE alias_id IN (SELECT id FROM aliases WHERE domain_id = $1)`, domainID); err != nil {
		slog.Error("domain: cascade delete alias_users failed", "domain_id", domainID, "error", err)
	}
	if _, err := s.q.Exec(ctx, `UPDATE threads SET deleted_at = now(), updated_at = now() WHERE domain_id = $1 AND deleted_at IS NULL`, domainID); err != nil {
		slog.Error("domain: cascade delete threads failed", "domain_id", domainID, "error", err)
	}
	if _, err := s.q.Exec(ctx, `DELETE FROM discovered_addresses WHERE domain_id = $1`, domainID); err != nil {
		slog.Error("domain: cascade delete discovered_addresses failed", "domain_id", domainID, "error", err)
	}
	return nil
}

func (s *PgStore) ListDiscoveredDomains(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, domain, first_seen_at
		 FROM discovered_domains
		 WHERE org_id = $1 AND dismissed = false
		 ORDER BY first_seen_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) DismissDiscoveredDomain(ctx context.Context, id, orgID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE discovered_domains SET dismissed = true WHERE id = $1 AND org_id = $2`,
		id, orgID,
	)
	return err
}

func (s *PgStore) UpdateWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1,
		 resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = $2, resend_webhook_secret_iv = $3, resend_webhook_secret_tag = $4,
		 updated_at = now() WHERE id = $5`,
		webhookID, encSecret, encIV, encTag, orgID,
	)
	return err
}
