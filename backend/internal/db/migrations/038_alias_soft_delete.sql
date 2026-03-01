-- +goose Up
ALTER TABLE aliases ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE aliases DROP COLUMN IF EXISTS deleted_at;
