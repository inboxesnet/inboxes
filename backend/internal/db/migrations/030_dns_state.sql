-- +goose Up
-- DNS alert transition state: the heartbeat publishes domain.dns_degraded
-- only when dns_ok flips from true to false, never on every run.
ALTER TABLE domains ADD COLUMN dns_ok BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE domains DROP COLUMN dns_ok;
