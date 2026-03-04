package store

import (
	"context"
)

func (s *PgStore) GetPollableOrgs(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, auto_poll_interval FROM orgs
		 WHERE auto_poll_enabled = true
		   AND deleted_at IS NULL
		   AND resend_api_key_encrypted IS NOT NULL
		   AND (last_polled_at IS NULL OR last_polled_at < now() - (auto_poll_interval || ' seconds')::interval)
		   AND id NOT IN (SELECT org_id FROM sync_jobs WHERE status IN ('pending', 'running'))`)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) HasPendingSyncJob(ctx context.Context, orgID string) (bool, error) {
	var exists bool
	err := s.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM sync_jobs WHERE org_id = $1 AND status IN ('pending', 'running'))`,
		orgID,
	).Scan(&exists)
	return exists, err
}

func (s *PgStore) EmailExistsByResendID(ctx context.Context, orgID, resendEmailID string) (bool, error) {
	var exists bool
	err := s.q.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM emails WHERE resend_email_id = $1)",
		resendEmailID,
	).Scan(&exists)
	return exists, err
}

func (s *PgStore) CreateFetchJob(ctx context.Context, orgID, resendEmailID, jobType string) (string, error) {
	// Find admin user for this org
	var adminUserID string
	if err := s.q.QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&adminUserID); err != nil {
		return "", err
	}

	var jobID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO email_jobs (org_id, user_id, job_type, resend_email_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (resend_email_id) WHERE status IN ('pending', 'running') DO NOTHING
		 RETURNING id`,
		orgID, adminUserID, jobType, resendEmailID,
	).Scan(&jobID)
	return jobID, err
}
