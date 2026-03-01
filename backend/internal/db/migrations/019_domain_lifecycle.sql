-- +goose Up
ALTER TYPE domain_status ADD VALUE IF NOT EXISTS 'disconnected';

-- +goose Down
-- Cannot remove enum values in PostgreSQL; no-op
