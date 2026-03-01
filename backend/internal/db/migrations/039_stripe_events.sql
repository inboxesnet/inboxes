-- +goose Up
CREATE TABLE IF NOT EXISTS stripe_events (
    event_id   TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stripe_events_processed_at ON stripe_events (processed_at);

-- +goose Down
DROP TABLE IF EXISTS stripe_events;
