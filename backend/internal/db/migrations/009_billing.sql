-- +goose Up
ALTER TABLE orgs
  ADD COLUMN stripe_customer_id TEXT,
  ADD COLUMN stripe_subscription_id TEXT,
  ADD COLUMN plan TEXT NOT NULL DEFAULT 'free',
  ADD COLUMN plan_expires_at TIMESTAMPTZ;

CREATE INDEX idx_orgs_stripe_customer ON orgs(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_orgs_stripe_customer;
ALTER TABLE orgs
  DROP COLUMN IF EXISTS stripe_customer_id,
  DROP COLUMN IF EXISTS stripe_subscription_id,
  DROP COLUMN IF EXISTS plan,
  DROP COLUMN IF EXISTS plan_expires_at;
