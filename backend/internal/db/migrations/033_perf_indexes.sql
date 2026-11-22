-- +goose Up

-- The search vector depends only on subject, body_plain and from_address.
-- The old trigger fired on every UPDATE (read flag, status, dismiss), parsed
-- the full body again, and rewrote the GIN index entry each time.
DROP TRIGGER IF EXISTS emails_search_update ON emails;
CREATE TRIGGER emails_search_update
  BEFORE INSERT OR UPDATE OF subject, body_plain, from_address ON emails
  FOR EACH ROW EXECUTE FUNCTION emails_search_trigger();

-- Job lookups by email (retry, undo, reconciler) and the foreign key check
-- on DELETE FROM emails.
CREATE INDEX idx_email_jobs_email_id ON email_jobs(email_id) WHERE email_id IS NOT NULL;

-- One live send job for each draft. This closes the double-send race of two
-- "send draft" requests at the same time.
-- Existing duplicates first: keep the newest live job attached to the draft
-- and detach the older ones. A detached job still sends its email; it only
-- loses the link to the draft. No job is cancelled and no email is lost.
UPDATE email_jobs j
SET draft_id = NULL
FROM (
  SELECT id,
         row_number() OVER (PARTITION BY draft_id ORDER BY created_at DESC, id DESC) AS rn
  FROM email_jobs
  WHERE draft_id IS NOT NULL AND job_type = 'send' AND status IN ('pending', 'running')
) d
WHERE d.id = j.id AND d.rn > 1;

CREATE UNIQUE INDEX idx_email_jobs_draft_live ON email_jobs(draft_id)
  WHERE draft_id IS NOT NULL AND job_type = 'send' AND status IN ('pending', 'running');

-- Orphan recovery reads pending jobs every 60 seconds.
CREATE INDEX idx_email_jobs_pending ON email_jobs(updated_at) WHERE status = 'pending';

-- The Failed view and its sidebar count.
CREATE INDEX idx_emails_failed_outbound ON emails(thread_id)
  WHERE direction = 'outbound' AND status IN ('failed', 'bounced') AND dismissed_at IS NULL;

-- The send reconciler (status queued) and status recovery (received, sent).
CREATE INDEX idx_emails_outbound_open ON emails(created_at)
  WHERE direction = 'outbound' AND status IN ('queued', 'received', 'sent');

-- Contact suggestions read the newest emails of one org.
CREATE INDEX idx_emails_org_created ON emails(org_id, created_at DESC);

-- Unread counters in the sidebar read only threads with unread mail.
CREATE INDEX idx_threads_org_domain_unread ON threads(org_id, domain_id)
  WHERE unread_count > 0 AND deleted_at IS NULL;

-- The trash collector and the purge endpoint.
CREATE INDEX idx_threads_trash_expiry ON threads(trash_expires_at)
  WHERE trash_expires_at IS NOT NULL AND deleted_at IS NULL;

-- The notification bell reads only warning events, newest first.
CREATE INDEX idx_events_org_warning ON events(org_id, id DESC)
  WHERE event_type IN ('domain.disconnected', 'domain.not_found', 'domain.dns_degraded',
                       'domain.reconnected', 'email.status_updated', 'plan.changed');

-- +goose Down
DROP INDEX IF EXISTS idx_events_org_warning;
DROP INDEX IF EXISTS idx_threads_trash_expiry;
DROP INDEX IF EXISTS idx_threads_org_domain_unread;
DROP INDEX IF EXISTS idx_emails_org_created;
DROP INDEX IF EXISTS idx_emails_outbound_open;
DROP INDEX IF EXISTS idx_emails_failed_outbound;
DROP INDEX IF EXISTS idx_email_jobs_pending;
DROP INDEX IF EXISTS idx_email_jobs_draft_live;
DROP INDEX IF EXISTS idx_email_jobs_email_id;

DROP TRIGGER IF EXISTS emails_search_update ON emails;
CREATE TRIGGER emails_search_update
  BEFORE INSERT OR UPDATE ON emails
  FOR EACH ROW EXECUTE FUNCTION emails_search_trigger();
