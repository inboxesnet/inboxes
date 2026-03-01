-- +goose Up
ALTER TABLE users ADD COLUMN is_owner BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS is_owner;
