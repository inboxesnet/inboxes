-- +goose Up
-- +goose NO TRANSACTION

-- PRD-062: GIN indexes for JSONB containment queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_to_addresses_gin
  ON emails USING GIN(to_addresses);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_cc_addresses_gin
  ON emails USING GIN(cc_addresses);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_threads_participant_emails_gin
  ON threads USING GIN(participant_emails);

-- +goose Down
DROP INDEX IF EXISTS idx_emails_to_addresses_gin;
DROP INDEX IF EXISTS idx_emails_cc_addresses_gin;
DROP INDEX IF EXISTS idx_threads_participant_emails_gin;
