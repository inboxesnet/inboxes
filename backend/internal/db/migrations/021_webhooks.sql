-- +goose Up

-- Last time a Resend webhook was received for this org. Lets the UI show
-- webhook health instead of silently degrading to polling.
ALTER TABLE orgs ADD COLUMN last_webhook_at TIMESTAMPTZ;
