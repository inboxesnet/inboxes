-- +goose Up
-- Domain status is TEXT, so no schema change is needed.
-- This migration documents that 'deleted' is now a valid domain status value.
-- Valid domain statuses: 'not_started', 'pending', 'verified', 'failed', 'deleted'
SELECT 1;

-- +goose Down
SELECT 1;
