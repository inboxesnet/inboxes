-- +goose Up
ALTER TABLE orgs
    ADD COLUMN IF NOT EXISTS resend_webhook_secret_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS resend_webhook_secret_iv TEXT,
    ADD COLUMN IF NOT EXISTS resend_webhook_secret_tag TEXT;

-- +goose Down
ALTER TABLE orgs
    DROP COLUMN IF EXISTS resend_webhook_secret_encrypted,
    DROP COLUMN IF EXISTS resend_webhook_secret_iv,
    DROP COLUMN IF EXISTS resend_webhook_secret_tag;
