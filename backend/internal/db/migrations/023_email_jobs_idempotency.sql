-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_jobs_resend_id_pending
  ON email_jobs (resend_email_id)
  WHERE status IN ('pending', 'running');

-- +goose Down
DROP INDEX IF EXISTS idx_email_jobs_resend_id_pending;
