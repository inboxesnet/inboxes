-- +goose Up
-- Dismissable failed/bounced sends: a dismissed email stays in the database,
-- so sync cannot resurrect it, but the Failed view hides it.
ALTER TABLE emails ADD COLUMN dismissed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE emails DROP COLUMN dismissed_at;
