-- +goose Up
ALTER TABLE orgs ADD COLUMN deleted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE orgs DROP COLUMN deleted_at;
