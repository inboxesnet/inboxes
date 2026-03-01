-- +goose Up
CREATE INDEX idx_emails_delivered_via_alias ON emails(delivered_via_alias) WHERE delivered_via_alias IS NOT NULL;
CREATE INDEX idx_emails_sent_as_alias ON emails(sent_as_alias) WHERE sent_as_alias IS NOT NULL;
CREATE INDEX idx_emails_thread_alias ON emails(thread_id, delivered_via_alias, sent_as_alias);

-- Backfill delivered_via_alias for inbound emails
UPDATE emails e SET delivered_via_alias = a.address
FROM aliases a
WHERE e.direction = 'inbound' AND e.delivered_via_alias IS NULL
  AND (e.to_addresses::jsonb @> to_jsonb(a.address) OR e.cc_addresses::jsonb @> to_jsonb(a.address))
  AND a.org_id = e.org_id;

-- Backfill sent_as_alias for outbound emails
UPDATE emails e SET sent_as_alias = a.address
FROM aliases a
WHERE e.direction = 'outbound' AND e.sent_as_alias IS NULL
  AND (lower(trim(e.from_address)) = a.address
       OR lower(trim(e.from_address)) LIKE '%<' || a.address || '>')
  AND a.org_id = e.org_id;

-- +goose Down
DROP INDEX IF EXISTS idx_emails_thread_alias;
DROP INDEX IF EXISTS idx_emails_sent_as_alias;
DROP INDEX IF EXISTS idx_emails_delivered_via_alias;
