package store

import (
	"context"
	"log/slog"
)

func (s *PgStore) GetOrgSettings(ctx context.Context, orgID string) (map[string]any, error) {
	var name string
	var onboardingCompleted bool
	var hasAPIKey bool
	var resendRPS int
	var autoPollEnabled bool
	var autoPollInterval int

	err := s.q.QueryRow(ctx,
		`SELECT name, onboarding_completed, (resend_api_key_encrypted IS NOT NULL) as has_key, resend_rps,
		        auto_poll_enabled, auto_poll_interval
		 FROM orgs WHERE id = $1`, orgID,
	).Scan(&name, &onboardingCompleted, &hasAPIKey, &resendRPS, &autoPollEnabled, &autoPollInterval)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"name":                 name,
		"onboarding_completed": onboardingCompleted,
		"has_api_key":          hasAPIKey,
		"resend_rps":           resendRPS,
		"auto_poll_enabled":    autoPollEnabled,
		"auto_poll_interval":   autoPollInterval,
	}, nil
}

func (s *PgStore) UpdateOrgName(ctx context.Context, orgID, name string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET name = $1, updated_at = now() WHERE id = $2`,
		name, orgID)
	return err
}

func (s *PgStore) UpdateOrgAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2, resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
		ciphertext, iv, tag, orgID)
	return err
}

func (s *PgStore) UpdateOrgRPS(ctx context.Context, orgID string, rps int) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET resend_rps = $1, updated_at = now() WHERE id = $2`,
		rps, orgID)
	return err
}

func (s *PgStore) UpdateOrgAutoPoll(ctx context.Context, orgID string, enabled bool) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET auto_poll_enabled = $1, updated_at = now() WHERE id = $2`,
		enabled, orgID)
	return err
}

func (s *PgStore) UpdateOrgAutoPollInterval(ctx context.Context, orgID string, interval int) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET auto_poll_interval = $1, updated_at = now() WHERE id = $2`,
		interval, orgID)
	return err
}

func (s *PgStore) IsOrgOwner(ctx context.Context, userID, orgID string) (bool, error) {
	var isOwner bool
	err := s.q.QueryRow(ctx,
		`SELECT is_owner FROM users WHERE id = $1 AND org_id = $2`,
		userID, orgID).Scan(&isOwner)
	if err != nil {
		return false, err
	}
	return isOwner, nil
}

func (s *PgStore) GetStripeSubscriptionID(ctx context.Context, orgID string) (*string, error) {
	var subID *string
	err := s.q.QueryRow(ctx,
		`SELECT stripe_subscription_id FROM orgs WHERE id = $1`, orgID,
	).Scan(&subID)
	if err != nil {
		return nil, err
	}
	return subID, nil
}

func (s *PgStore) GetWebhookID(ctx context.Context, orgID string) (*string, error) {
	var webhookID *string
	err := s.q.QueryRow(ctx,
		`SELECT resend_webhook_id FROM orgs WHERE id = $1`, orgID,
	).Scan(&webhookID)
	if err != nil {
		return nil, err
	}
	return webhookID, nil
}

func (s *PgStore) SoftDeleteOrg(ctx context.Context, orgID string) error {
	// Cascade soft-delete to all child entities
	if _, err := s.q.Exec(ctx,
		`UPDATE orgs SET deleted_at = now(), stripe_subscription_id = NULL,
		 resend_webhook_id = NULL, resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = NULL, resend_webhook_secret_iv = NULL, resend_webhook_secret_tag = NULL,
		 updated_at = now() WHERE id = $1`,
		orgID); err != nil {
		return err
	}

	if _, err := s.q.Exec(ctx,
		`UPDATE users SET status = 'disabled', updated_at = now() WHERE org_id = $1`,
		orgID); err != nil {
		slog.Error("orgs: failed to disable users on delete", "error", err, "org_id", orgID)
	}

	if _, err := s.q.Exec(ctx,
		`UPDATE domains SET status = 'deleted', hidden = true, updated_at = now() WHERE org_id = $1`,
		orgID); err != nil {
		slog.Error("orgs: failed to mark domains deleted", "error", err, "org_id", orgID)
	}

	if _, err := s.q.Exec(ctx,
		`UPDATE aliases SET deleted_at = now() WHERE org_id = $1 AND deleted_at IS NULL`,
		orgID); err != nil {
		slog.Error("orgs: failed to soft-delete aliases", "error", err, "org_id", orgID)
	}

	if _, err := s.q.Exec(ctx,
		`UPDATE threads SET deleted_at = now(), updated_at = now() WHERE org_id = $1 AND deleted_at IS NULL`,
		orgID); err != nil {
		slog.Error("orgs: failed to soft-delete threads", "error", err, "org_id", orgID)
	}

	if _, err := s.q.Exec(ctx,
		`DELETE FROM discovered_addresses WHERE domain_id IN (SELECT id FROM domains WHERE org_id = $1)`,
		orgID); err != nil {
		slog.Error("orgs: failed to delete discovered_addresses", "error", err, "org_id", orgID)
	}

	if _, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE org_id = $1`,
		orgID); err != nil {
		slog.Error("orgs: failed to delete thread_labels", "error", err, "org_id", orgID)
	}

	return nil
}

func (s *PgStore) ListOrgUserIDs(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.q.Query(ctx, `SELECT id FROM users WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (s *PgStore) CancelOrgJobs(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`UPDATE email_jobs SET status='cancelled', error_message='org deleted', updated_at=now()
		 WHERE org_id = $1 AND status IN ('pending', 'running')
		 RETURNING id`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (s *PgStore) GetOrgNameByID(ctx context.Context, orgID string) (string, error) {
	var name string
	err := s.q.QueryRow(ctx,
		`SELECT name FROM orgs WHERE id = $1`, orgID,
	).Scan(&name)
	return name, err
}

func (s *PgStore) HardDeleteOrg(ctx context.Context, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE orgs SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
		orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
