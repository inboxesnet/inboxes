-- +goose Up
-- Normalize existing email addresses to lowercase.
-- Handle potential conflicts by keeping the most recently updated account.
DELETE FROM users a USING users b
  WHERE LOWER(a.email) = LOWER(b.email)
    AND a.id <> b.id
    AND a.updated_at < b.updated_at;

UPDATE users SET email = LOWER(email) WHERE email <> LOWER(email);

-- Also lowercase alias addresses for consistency.
UPDATE aliases SET address = LOWER(address) WHERE address <> LOWER(address);

-- +goose Down
-- Cannot reverse — original casing is lost.
SELECT 1;
