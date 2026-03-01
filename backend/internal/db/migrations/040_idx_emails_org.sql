-- +goose Up
-- +goose NO TRANSACTION
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_org ON emails(org_id);

-- +goose Down
DROP INDEX IF EXISTS idx_emails_org;
