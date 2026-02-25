-- +goose Up
ALTER TABLE alias_users ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE alias_users DROP COLUMN is_default;
