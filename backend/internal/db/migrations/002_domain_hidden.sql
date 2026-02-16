-- +goose Up
ALTER TABLE domains ADD COLUMN hidden BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE domains DROP COLUMN hidden;
