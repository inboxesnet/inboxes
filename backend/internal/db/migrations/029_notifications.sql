-- +goose Up
-- Bell notification center: per-user read cursor over the events table.
-- Events with id <= notifications_read_id count as read.
ALTER TABLE users ADD COLUMN notifications_read_id BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN notifications_read_id;
