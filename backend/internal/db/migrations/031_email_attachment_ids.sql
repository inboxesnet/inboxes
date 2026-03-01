-- +goose Up
ALTER TABLE emails ADD COLUMN attachment_ids JSONB DEFAULT '[]';

-- +goose Down
ALTER TABLE emails DROP COLUMN attachment_ids;
