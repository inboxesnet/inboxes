-- +goose Up
-- Re-backfill any threads that still have empty snippets (e.g. from initial sync)
UPDATE threads t SET snippet = sub.snippet FROM (
  SELECT DISTINCT ON (thread_id) thread_id,
    LEFT(COALESCE(body_plain, ''), 200) AS snippet
  FROM emails ORDER BY thread_id, created_at DESC
) sub WHERE t.id = sub.thread_id AND t.snippet = '';

-- +goose Down
-- No rollback needed for data backfill
