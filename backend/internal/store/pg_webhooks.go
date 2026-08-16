package store

import (
	"context"
	"time"
)

func (s *PgStore) GetOrgWebhookSecret(ctx context.Context, orgID string) (encSecret, encIV, encTag string, plainSecret *string, err error) {
	var encS, encI, encT *string
	err = s.q.QueryRow(ctx,
		`SELECT resend_webhook_secret, resend_webhook_secret_encrypted, resend_webhook_secret_iv, resend_webhook_secret_tag
		 FROM orgs WHERE id = $1`, orgID,
	).Scan(&plainSecret, &encS, &encI, &encT)
	if err != nil {
		return "", "", "", nil, err
	}
	if encS != nil {
		encSecret = *encS
	}
	if encI != nil {
		encIV = *encI
	}
	if encT != nil {
		encTag = *encT
	}
	return
}

func (s *PgStore) CheckWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) (bool, error) {
	var exists bool
	err := s.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM emails WHERE resend_email_id = $1 AND org_id = $2)`,
		resendEmailID, orgID,
	).Scan(&exists)
	return exists, err
}

func (s *PgStore) InsertWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) error {
	// This is handled by the idempotent INSERT in email_jobs with ON CONFLICT
	// The handler uses this as a check — the actual dedup is via the unique index on resend_email_id
	return nil
}

// UpdateEmailStatus sets a new status and reports affected rows. The
// IS DISTINCT FROM guard makes a replayed webhook a no-op: zero rows means
// "no such email" or "status unchanged" — callers must tell the two apart
// before they retry or publish an event.
func (s *PgStore) UpdateEmailStatus(ctx context.Context, orgID, resendEmailID, status string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		"UPDATE emails SET status = $1, updated_at = now() WHERE resend_email_id = $2 AND org_id = $3 AND status IS DISTINCT FROM $1",
		status, resendEmailID, orgID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) GetEmailThreadByResendID(ctx context.Context, orgID, resendEmailID string) (emailID, threadID, domainID string, err error) {
	var subject string
	err = s.q.QueryRow(ctx,
		"SELECT id, thread_id, domain_id, subject FROM emails WHERE resend_email_id = $1 AND org_id = $2",
		resendEmailID, orgID,
	).Scan(&emailID, &threadID, &domainID, &subject)
	return
}

func (s *PgStore) InsertBounce(ctx context.Context, orgID, address, bounceType string) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO email_bounces (org_id, address, reason)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, lower(address)) DO UPDATE SET reason = $3, created_at = now()`,
		orgID, address, bounceType,
	)
	return err
}

func (s *PgStore) ClearBounce(ctx context.Context, orgID, fromAddress string) error {
	_, err := s.q.Exec(ctx,
		`DELETE FROM email_bounces WHERE org_id = $1 AND lower(address) = lower($2)`,
		orgID, fromAddress,
	)
	return err
}

// ListBounces returns the bounce block list for an org, newest first.
// Each row includes expires_at so the UI can show when the block ends.
func (s *PgStore) ListBounces(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, address, reason, created_at,
		        created_at + make_interval(days => $2) AS expires_at
		 FROM email_bounces WHERE org_id = $1
		 ORDER BY created_at DESC LIMIT 500`,
		orgID, BounceExpiryDays,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bounces := []map[string]any{}
	for rows.Next() {
		var id, address, reason string
		var createdAt, expiresAt time.Time
		if rows.Scan(&id, &address, &reason, &createdAt, &expiresAt) == nil {
			bounces = append(bounces, map[string]any{
				"id":         id,
				"address":    address,
				"reason":     reason,
				"created_at": createdAt,
				"expires_at": expiresAt,
			})
		}
	}
	return bounces, rows.Err()
}

// DeleteBounce removes one bounce row for an org. Returns affected rows.
func (s *PgStore) DeleteBounce(ctx context.Context, orgID, id string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`DELETE FROM email_bounces WHERE org_id = $1 AND id = $2`,
		orgID, id,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
