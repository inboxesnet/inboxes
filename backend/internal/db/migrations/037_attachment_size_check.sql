-- +goose Up
ALTER TABLE attachments ADD CONSTRAINT attachments_max_size CHECK (size <= 10485760);

-- +goose Down
ALTER TABLE attachments DROP CONSTRAINT IF EXISTS attachments_max_size;
