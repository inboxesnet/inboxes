-- +goose Up
-- +goose NO TRANSACTION

-- PRD-074: Change global UNIQUE to partial unique index excluding deleted domains.
-- This allows re-adding a domain that was previously soft-deleted.
-- Must drop the existing constraint first (outside transaction for CONCURRENTLY).

-- Step 1: Drop the global unique constraint
ALTER TABLE domains DROP CONSTRAINT IF EXISTS domains_domain_key;

-- Step 2: Create partial unique index (only active/pending/verified domains hold uniqueness)
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_domains_unique_active
  ON domains(domain)
  WHERE status NOT IN ('deleted');

-- +goose Down
DROP INDEX IF EXISTS idx_domains_unique_active;
ALTER TABLE domains ADD CONSTRAINT domains_domain_key UNIQUE (domain);
