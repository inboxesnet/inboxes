-- +goose Up
ALTER TYPE email_status ADD VALUE IF NOT EXISTS 'queued' BEFORE 'sent';

CREATE TABLE email_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID NOT NULL REFERENCES orgs(id),
  user_id         UUID NOT NULL REFERENCES users(id),
  domain_id       UUID REFERENCES domains(id),
  job_type        TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',

  -- Send jobs
  email_id        UUID REFERENCES emails(id),
  thread_id       UUID REFERENCES threads(id),
  resend_payload  JSONB,
  draft_id        UUID,

  -- Fetch jobs
  resend_email_id TEXT,
  webhook_data    JSONB,

  -- Retry
  retry_count     INT NOT NULL DEFAULT 0,
  max_retries     INT NOT NULL DEFAULT 5,
  error_message   TEXT,
  heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_jobs_org_status ON email_jobs(org_id, status);
CREATE INDEX idx_email_jobs_status_heartbeat ON email_jobs(status, heartbeat_at) WHERE status = 'running';
CREATE INDEX idx_email_jobs_pending_send ON email_jobs(org_id, status, created_at) WHERE status = 'pending' AND job_type = 'send';

-- +goose Down
DROP TABLE IF EXISTS email_jobs;
