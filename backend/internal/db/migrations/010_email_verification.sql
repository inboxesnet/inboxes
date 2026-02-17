-- +goose Up
ALTER TABLE users
  ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN verification_code TEXT,
  ADD COLUMN verification_expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users
  DROP COLUMN IF EXISTS email_verified,
  DROP COLUMN IF EXISTS verification_code,
  DROP COLUMN IF EXISTS verification_expires_at;
