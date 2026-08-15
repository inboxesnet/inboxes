-- +goose Up
ALTER TABLE users ADD COLUMN undo_send_seconds INT NOT NULL DEFAULT 10;

-- +goose Down
ALTER TABLE users DROP COLUMN undo_send_seconds;
