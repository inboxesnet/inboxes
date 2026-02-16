-- +goose Up
ALTER TABLE threads ADD COLUMN snippet TEXT NOT NULL DEFAULT '';

-- Backfill snippets from latest email body_plain per thread
UPDATE threads t SET snippet = sub.snippet FROM (
  SELECT DISTINCT ON (thread_id) thread_id,
    LEFT(COALESCE(body_plain, ''), 200) AS snippet
  FROM emails ORDER BY thread_id, created_at DESC
) sub WHERE t.id = sub.thread_id;

-- +goose Down
ALTER TABLE threads DROP COLUMN snippet;
