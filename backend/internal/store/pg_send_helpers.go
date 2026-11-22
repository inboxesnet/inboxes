package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// LatestThreadMessageRef returns the Message-ID and the References list of
// the newest email in a thread that has a Message-ID. A send into a thread
// without threading headers uses it to build In-Reply-To and References.
// It returns an empty messageID when the thread has no such email.
func (s *PgStore) LatestThreadMessageRef(ctx context.Context, threadID, orgID string) (messageID string, references []string, err error) {
	var refsRaw []byte
	err = s.q.QueryRow(ctx,
		`SELECT message_id, COALESCE(references_header, '[]'::jsonb)
		 FROM emails
		 WHERE thread_id = $1 AND org_id = $2 AND COALESCE(message_id, '') <> ''
		 ORDER BY created_at DESC
		 LIMIT 1`,
		threadID, orgID,
	).Scan(&messageID, &refsRaw)
	if err != nil {
		return "", nil, err
	}
	// A value that is not a string array gives an empty list.
	_ = json.Unmarshal(refsRaw, &references)
	return messageID, references, nil
}

// RecomputeThreadStats sets message_count, last_message_at, snippet and
// last_sender of a thread from the emails that remain in it. Call it after
// an email row is deleted. It returns the number of remaining emails. With
// zero emails it changes nothing; the caller decides what to do with the
// empty thread.
func (s *PgStore) RecomputeThreadStats(ctx context.Context, threadID, orgID string) (int, error) {
	var remaining int
	if err := s.q.QueryRow(ctx,
		`SELECT count(*) FROM emails WHERE thread_id = $1`, threadID,
	).Scan(&remaining); err != nil {
		return 0, err
	}
	if remaining == 0 {
		return 0, nil
	}
	_, err := s.q.Exec(ctx,
		`UPDATE threads t SET
		   message_count = $3,
		   last_message_at = last.created_at,
		   snippet = left(regexp_replace(COALESCE(last.body_plain, ''), '\s+', ' ', 'g'), 200),
		   last_sender = last.from_address,
		   updated_at = now()
		 FROM (
		   SELECT created_at, body_plain, from_address FROM emails
		   WHERE thread_id = $1
		   ORDER BY created_at DESC
		   LIMIT 1
		 ) last
		 WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID, remaining,
	)
	return remaining, err
}

// CancelScheduledSendJob cancels the pending scheduled send job of a draft.
// It returns the id of the parked email, and found = false when the draft
// has no pending scheduled job (or the send already started).
func (s *PgStore) CancelScheduledSendJob(ctx context.Context, draftID, reason string) (emailID *string, found bool, err error) {
	err = s.q.QueryRow(ctx,
		`UPDATE email_jobs SET status = 'cancelled', error_message = $2, updated_at = now()
		 WHERE id = (
		   SELECT id FROM email_jobs
		   WHERE draft_id = $1 AND job_type = 'send' AND status = 'pending' AND run_after IS NOT NULL
		   ORDER BY created_at DESC LIMIT 1
		 ) AND status = 'pending'
		 RETURNING email_id`,
		draftID, reason,
	).Scan(&emailID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return emailID, true, nil
}
