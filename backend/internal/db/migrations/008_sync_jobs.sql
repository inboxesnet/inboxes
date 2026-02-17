-- +goose Up
CREATE TYPE sync_job_status AS ENUM ('pending', 'running', 'completed', 'failed');

CREATE TABLE sync_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID NOT NULL REFERENCES orgs(id),
  user_id         UUID NOT NULL REFERENCES users(id),
  status          sync_job_status NOT NULL DEFAULT 'pending',
  sent_cursor     TEXT NOT NULL DEFAULT '',
  received_cursor TEXT NOT NULL DEFAULT '',
  phase           TEXT NOT NULL DEFAULT 'pending',
  imported        INT NOT NULL DEFAULT 0,
  total           INT NOT NULL DEFAULT 0,
  sent_count      INT NOT NULL DEFAULT 0,
  received_count  INT NOT NULL DEFAULT 0,
  thread_count    INT NOT NULL DEFAULT 0,
  address_count   INT NOT NULL DEFAULT 0,
  retry_count     INT NOT NULL DEFAULT 0,
  max_retries     INT NOT NULL DEFAULT 3,
  error_message   TEXT,
  heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sync_jobs_org_status ON sync_jobs(org_id, status);
CREATE INDEX idx_sync_jobs_heartbeat ON sync_jobs(status, heartbeat_at) WHERE status = 'running';

-- +goose Down
DROP TABLE IF EXISTS sync_jobs;
DROP TYPE IF EXISTS sync_job_status;
