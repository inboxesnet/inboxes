-- +goose Up
ALTER TABLE emails ADD COLUMN is_read BOOLEAN DEFAULT false;

-- +goose Down
ALTER TABLE emails DROP COLUMN is_read;
