-- +goose Up
ALTER TYPE email_status ADD VALUE IF NOT EXISTS 'complained';

-- +goose Down
-- Cannot remove enum values in PostgreSQL
