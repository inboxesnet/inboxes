-- +goose Up
CREATE TABLE thread_labels (
  thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  org_id    UUID NOT NULL REFERENCES orgs(id),
  label     TEXT NOT NULL,
  added_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (thread_id, label)
);
CREATE INDEX idx_thread_labels_org_label ON thread_labels(org_id, label, thread_id);
CREATE INDEX idx_thread_labels_thread ON thread_labels(thread_id, label);

-- Backfill from folder column
INSERT INTO thread_labels (thread_id, org_id, label)
  SELECT id, org_id, folder::text FROM threads
  WHERE folder IN ('inbox', 'sent', 'trash', 'spam') AND deleted_at IS NULL;

-- Backfill starred
INSERT INTO thread_labels (thread_id, org_id, label)
  SELECT id, org_id, 'starred' FROM threads WHERE starred = true AND deleted_at IS NULL;

-- Backfill sent label for threads with outbound emails (data we were losing)
INSERT INTO thread_labels (thread_id, org_id, label)
  SELECT DISTINCT t.id, t.org_id, 'sent' FROM threads t
  JOIN emails e ON e.thread_id = t.id AND e.direction = 'outbound'
  WHERE t.deleted_at IS NULL
  ON CONFLICT DO NOTHING;

-- Mark deleted_forever threads properly (they had no deleted_at set)
UPDATE threads SET deleted_at = now() WHERE folder = 'deleted_forever' AND deleted_at IS NULL;

-- Drop old columns
ALTER TABLE threads DROP COLUMN folder;
ALTER TABLE threads DROP COLUMN starred;

-- +goose Down
ALTER TABLE threads ADD COLUMN folder text NOT NULL DEFAULT 'inbox';
ALTER TABLE threads ADD COLUMN starred boolean NOT NULL DEFAULT false;
DROP TABLE IF EXISTS thread_labels;
