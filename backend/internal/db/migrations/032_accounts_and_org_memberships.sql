-- +goose Up
-- Separate a human's sign-in identity from their organization-scoped user
-- records. Existing user IDs remain stable because they are referenced by
-- mailbox data, aliases, events, and agent tokens.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM users GROUP BY lower(email) HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'cannot migrate accounts: existing user emails collide when compared case-insensitively';
  END IF;
END;
$$;
-- +goose StatementEnd

CREATE TABLE accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  password_hash TEXT,
  email_verified BOOLEAN NOT NULL DEFAULT true,
  verification_code TEXT,
  verification_expires_at TIMESTAMPTZ,
  reset_token TEXT,
  reset_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_accounts_email ON accounts (lower(email));

INSERT INTO accounts (email, name, password_hash, email_verified, verification_code,
                      verification_expires_at, reset_token, reset_expires_at,
                      created_at, updated_at)
SELECT DISTINCT ON (lower(email)) email, name, password_hash, email_verified,
       verification_code, verification_expires_at, reset_token, reset_expires_at,
       created_at, updated_at
FROM users
WHERE status <> 'placeholder'
ORDER BY lower(email), created_at, id;

ALTER TABLE users ADD COLUMN account_id UUID REFERENCES accounts(id) ON DELETE SET NULL;

UPDATE users AS u
SET account_id = a.id
FROM accounts AS a
WHERE u.status <> 'placeholder'
  AND lower(u.email) = lower(a.email);

CREATE INDEX idx_users_account ON users(account_id) WHERE account_id IS NOT NULL;

ALTER TABLE users DROP CONSTRAINT users_email_key;
CREATE UNIQUE INDEX idx_users_org_email ON users(org_id, lower(email));

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'migration 032 cannot be rolled back safely after account/workspace creation';
END;
$$;
-- +goose StatementEnd
