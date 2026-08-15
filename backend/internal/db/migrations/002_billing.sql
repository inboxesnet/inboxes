-- +goose Up

-- Billing lifecycle bookkeeping.
-- expiry_notice_sent_at: when the pre-expiry warning email was sent for the
-- current grace period. Cleared when the org returns to "pro".
-- lapsed_at: when the grace period worker downgraded the org to "free".
-- A non-null value marks the org as "lapsed" (had a plan before), which
-- enables read-only access. Cleared when the org returns to "pro".
ALTER TABLE orgs ADD COLUMN expiry_notice_sent_at TIMESTAMPTZ;
ALTER TABLE orgs ADD COLUMN lapsed_at TIMESTAMPTZ;
