-- +goose Up

-- Fix alias address uniqueness (multi-tenant safe)
ALTER TABLE aliases DROP CONSTRAINT IF EXISTS aliases_address_key;
ALTER TABLE aliases ADD CONSTRAINT aliases_org_address_unique UNIQUE (org_id, address);

-- Add unique constraint on emails.resend_email_id
DROP INDEX IF EXISTS idx_emails_resend_id;
CREATE UNIQUE INDEX idx_emails_resend_id ON emails(resend_email_id) WHERE resend_email_id IS NOT NULL;

-- Drop stale indexes
DROP INDEX IF EXISTS idx_threads_user_domain_folder;
DROP INDEX IF EXISTS idx_threads_domain_folder;

-- Performance index for thread listing
CREATE INDEX IF NOT EXISTS idx_threads_user_domain_active
  ON threads(user_id, domain_id, deleted_at, last_message_at DESC);

-- Performance index for admin user lookups
CREATE INDEX IF NOT EXISTS idx_users_org_role_status ON users(org_id, role, status);

-- Drop orphaned domain columns
ALTER TABLE domains DROP COLUMN IF EXISTS mx_verified;
ALTER TABLE domains DROP COLUMN IF EXISTS spf_verified;
ALTER TABLE domains DROP COLUMN IF EXISTS dkim_verified;
ALTER TABLE domains DROP COLUMN IF EXISTS catch_all_enabled;
ALTER TABLE domains DROP COLUMN IF EXISTS verified_at;
ALTER TABLE domains DROP COLUMN IF EXISTS region;

-- +goose Down

-- Restore domain columns
ALTER TABLE domains ADD COLUMN IF NOT EXISTS mx_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS spf_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS dkim_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS catch_all_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '';

-- Drop performance indexes
DROP INDEX IF EXISTS idx_users_org_role_status;
DROP INDEX IF EXISTS idx_threads_user_domain_active;

-- Restore original resend_email_id index (non-unique)
DROP INDEX IF EXISTS idx_emails_resend_id;
CREATE INDEX idx_emails_resend_id ON emails(resend_email_id) WHERE resend_email_id IS NOT NULL;

-- Restore original alias constraint
ALTER TABLE aliases DROP CONSTRAINT IF EXISTS aliases_org_address_unique;
ALTER TABLE aliases ADD CONSTRAINT aliases_address_key UNIQUE (address);
