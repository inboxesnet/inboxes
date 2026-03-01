-- +goose Up
ALTER TABLE orgs ADD COLUMN resend_rps INTEGER NOT NULL DEFAULT 2;

-- +goose Down
ALTER TABLE orgs DROP COLUMN IF EXISTS resend_rps;
