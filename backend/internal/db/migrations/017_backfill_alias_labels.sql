-- +goose Up

-- Backfill alias labels from existing delivered_via_alias data (inbound emails)
INSERT INTO thread_labels (thread_id, org_id, label)
SELECT DISTINCT e.thread_id, e.org_id, 'alias:' || e.delivered_via_alias
FROM emails e
WHERE e.delivered_via_alias IS NOT NULL
ON CONFLICT DO NOTHING;

-- Backfill alias labels from existing sent_as_alias data (outbound emails)
INSERT INTO thread_labels (thread_id, org_id, label)
SELECT DISTINCT e.thread_id, e.org_id, 'alias:' || e.sent_as_alias
FROM emails e
WHERE e.sent_as_alias IS NOT NULL
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM thread_labels WHERE label LIKE 'alias:%';
