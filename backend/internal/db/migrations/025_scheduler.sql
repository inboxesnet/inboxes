-- +goose Up
ALTER TYPE email_status ADD VALUE IF NOT EXISTS 'scheduled';
ALTER TABLE email_jobs ADD COLUMN run_after TIMESTAMPTZ;
ALTER TABLE drafts ADD COLUMN scheduled_send_at TIMESTAMPTZ;
ALTER TABLE threads ADD COLUMN snoozed_until TIMESTAMPTZ;

CREATE INDEX idx_email_jobs_run_after ON email_jobs(run_after) WHERE run_after IS NOT NULL;
CREATE INDEX idx_threads_snoozed ON threads(snoozed_until) WHERE snoozed_until IS NOT NULL;

-- +goose Down
DROP INDEX idx_threads_snoozed;
DROP INDEX idx_email_jobs_run_after;
ALTER TABLE threads DROP COLUMN snoozed_until;
ALTER TABLE drafts DROP COLUMN scheduled_send_at;
ALTER TABLE email_jobs DROP COLUMN run_after;
