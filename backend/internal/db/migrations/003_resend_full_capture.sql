-- +goose Up

-- Emails: add reply_to, headers, and last_event from Resend
ALTER TABLE emails ADD COLUMN IF NOT EXISTS reply_to_addresses JSONB DEFAULT '[]';
ALTER TABLE emails ADD COLUMN IF NOT EXISTS headers JSONB;
ALTER TABLE emails ADD COLUMN IF NOT EXISTS last_event TEXT;

-- Domains: add region from Resend
ALTER TABLE domains ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE emails DROP COLUMN IF EXISTS reply_to_addresses;
ALTER TABLE emails DROP COLUMN IF EXISTS headers;
ALTER TABLE emails DROP COLUMN IF EXISTS last_event;
ALTER TABLE domains DROP COLUMN IF EXISTS region;
