package store

import (
	"context"
	"time"
)

func (s *PgStore) GetEmailJob(ctx context.Context, jobID string) (map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, org_id, user_id, domain_id, job_type, status, email_id, thread_id,
		 resend_payload, draft_id, resend_email_id, webhook_data,
		 retry_count, max_retries, error_message, heartbeat_at, created_at, updated_at
		 FROM email_jobs WHERE id = $1`, jobID,
	)
	if err != nil {
		return nil, err
	}
	results, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

func (s *PgStore) UpdateEmailJobStatus(ctx context.Context, jobID, status, errorMsg string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE email_jobs SET status = $1, error_message = $2, heartbeat_at = now(), updated_at = now() WHERE id = $3`,
		status, errorMsg, jobID,
	)
	return err
}

func (s *PgStore) UpdateEmailJobHeartbeat(ctx context.Context, jobID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE email_jobs SET heartbeat_at = now() WHERE id = $1`, jobID,
	)
	return err
}

func (s *PgStore) IncrementJobAttempts(ctx context.Context, jobID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE email_jobs SET retry_count = retry_count + 1, updated_at = now() WHERE id = $1`, jobID,
	)
	return err
}

func (s *PgStore) GetStaleJobs(ctx context.Context, timeout time.Duration) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id FROM email_jobs
		 WHERE status = 'running' AND heartbeat_at < now() - $1::interval`,
		timeout.String(),
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

func (s *PgStore) GetOrphanedJobs(ctx context.Context, age time.Duration) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id FROM email_jobs
		 WHERE status = 'pending' AND updated_at < now() - $1::interval`,
		age.String(),
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
