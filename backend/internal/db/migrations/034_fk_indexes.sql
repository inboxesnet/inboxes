-- +goose Up
-- +goose NO TRANSACTION

-- PRD-064: Missing foreign key indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_jobs_user_id
  ON email_jobs(user_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_jobs_domain_id
  ON email_jobs(domain_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alias_users_user_id
  ON alias_users(user_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_discovered_addresses_user_id
  ON discovered_addresses(user_id)
  WHERE user_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_jobs_user_id
  ON sync_jobs(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_email_jobs_user_id;
DROP INDEX IF EXISTS idx_email_jobs_domain_id;
DROP INDEX IF EXISTS idx_alias_users_user_id;
DROP INDEX IF EXISTS idx_discovered_addresses_user_id;
DROP INDEX IF EXISTS idx_sync_jobs_user_id;
