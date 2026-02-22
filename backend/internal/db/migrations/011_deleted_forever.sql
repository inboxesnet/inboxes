-- +goose Up
ALTER TYPE folder ADD VALUE IF NOT EXISTS 'deleted_forever';

-- +goose Down
-- Cannot remove enum values in PostgreSQL; no-op
