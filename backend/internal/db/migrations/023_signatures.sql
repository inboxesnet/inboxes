-- +goose Up
ALTER TABLE users ADD COLUMN signature_html TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN signature_html;
