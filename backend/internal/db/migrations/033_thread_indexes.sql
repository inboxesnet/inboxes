-- +goose Up
-- +goose NO TRANSACTION

-- PRD-063 + PRD-058: Composite partial index for thread list queries
-- Covers org_id filter + deleted_at IS NULL + last_message_at sort
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_threads_org_active_date
  ON threads(org_id, last_message_at DESC)
  WHERE deleted_at IS NULL;

-- PRD-058: Partial index for trash collector and cleanup queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_threads_deleted_at
  ON threads(deleted_at)
  WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_threads_org_active_date;
DROP INDEX IF EXISTS idx_threads_deleted_at;
