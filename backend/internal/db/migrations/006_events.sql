-- +goose Up
CREATE TABLE events (
  id         BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  org_id     UUID NOT NULL,
  user_id    UUID,
  domain_id  UUID,
  thread_id  UUID,
  payload    JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_org_created ON events(org_id, created_at);
CREATE INDEX idx_events_created ON events(created_at);

-- +goose Down
DROP TABLE IF EXISTS events;
