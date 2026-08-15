-- +goose Up

-- Resend API key health, updated by the domain heartbeat and by key rotation.
-- Values: 'unknown', 'valid', 'invalid'.
ALTER TABLE orgs ADD COLUMN api_key_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE orgs ADD COLUMN api_key_checked_at TIMESTAMPTZ;
