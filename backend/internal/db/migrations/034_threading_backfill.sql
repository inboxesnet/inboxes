-- +goose Up

-- Threading data repair. Each statement changes only rows that are not yet
-- in the normal form, so a second run changes nothing. No thread is merged
-- and no email moves to a different thread.

-- 1. Participants become bare lowercase addresses without duplicates.
--    "Alice <Alice@X.com>" becomes "alice@x.com". The subject fallback
--    compares bare lowercase addresses.
UPDATE threads t
SET participant_emails = n.val
FROM (
  SELECT t2.id,
         COALESCE(jsonb_agg(DISTINCT a.addr) FILTER (WHERE a.addr <> ''), '[]'::jsonb) AS val
  FROM threads t2
  LEFT JOIN LATERAL (
    SELECT lower(btrim(
             CASE WHEN p ~ '<[^<>]*>[^<]*$'
                  THEN substring(p from '<([^<>]*)>[^<]*$')
                  ELSE p
             END)) AS addr
    FROM jsonb_array_elements_text(t2.participant_emails) AS p
  ) a ON true
  WHERE jsonb_typeof(t2.participant_emails) = 'array'
  GROUP BY t2.id
) n
WHERE n.id = t.id
  AND t.participant_emails IS DISTINCT FROM n.val;

-- 2. Thread subjects lose reply and forward prefixes and extra whitespace.
--    The rule is the same as util.CleanSubjectLine in Go.
UPDATE threads t
SET subject = c.clean
FROM (
  SELECT id,
         btrim(regexp_replace(
           regexp_replace(
             subject,
             '^(\s*(re|aw|sv|vs|antw|odp|res|fwd?|wg|tr|rv|enc)\s*(\[\d+\]|\(\d+\))?\s*[:：]\s*)+',
             '', 'i'),
           '\s+', ' ', 'g')) AS clean
  FROM threads
) c
WHERE c.id = t.id
  AND t.subject IS DISTINCT FROM c.clean;

-- 3. Stored Message-IDs get angle brackets. Lookups use the form "<id>".
UPDATE emails
SET message_id = '<' || btrim(message_id, E' \t\r\n<>') || '>'
WHERE message_id IS NOT NULL
  AND btrim(message_id, E' \t\r\n<>') <> ''
  AND message_id !~ '^<[^<>\s]+>$';

-- 4. in_reply_to comes from the stored headers where the column is empty.
--    The sync import read the header with the wrong letter case.
UPDATE emails e
SET in_reply_to = h.msg_id
FROM (
  SELECT id AS email_id,
         substring(regexp_replace(
           COALESCE(headers->>'in-reply-to', headers->>'In-Reply-To'),
           '\s+', '', 'g') from '<[^<>]+>') AS msg_id
  FROM emails
  WHERE jsonb_typeof(headers) = 'object'
    AND (in_reply_to IS NULL OR in_reply_to = '')
) h
WHERE h.email_id = e.id
  AND h.msg_id IS NOT NULL;

-- 5. references_header comes from the stored headers where the column is
--    empty (NULL, JSON null, or []).
UPDATE emails e
SET references_header = r.refs
FROM (
  SELECT e2.id,
         (SELECT jsonb_agg(x.m[1] ORDER BY x.ord)
          FROM regexp_matches(
                 COALESCE(e2.headers->>'references', e2.headers->>'References'),
                 '<[^<>\s]+>', 'g') WITH ORDINALITY AS x(m, ord)) AS refs
  FROM emails e2
  WHERE jsonb_typeof(e2.headers) = 'object'
    AND (e2.references_header IS NULL
         OR e2.references_header = 'null'::jsonb
         OR e2.references_header = '[]'::jsonb)
) r
WHERE r.id = e.id
  AND r.refs IS NOT NULL;

-- 6. Index for the lookup "is a reply to this email already stored".
CREATE INDEX IF NOT EXISTS idx_emails_in_reply_to ON emails(in_reply_to) WHERE in_reply_to <> '';

-- +goose Down
-- The data changes are a one-way repair. Only the index is removed.
DROP INDEX IF EXISTS idx_emails_in_reply_to;
