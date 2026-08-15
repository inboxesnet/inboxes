package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/util"
)

// stuckEmailAge is how long an email may sit in 'queued' before the
// reconciler treats it as lost.
const stuckEmailAge = 15 * time.Minute

// RunReconciler periodically fails outbound emails that are stuck in
// 'queued' with no live send job. Without it, a lost job leaves the email
// showing "Queued" forever with no failure surfaced to the user.
func (w *EmailWorker) RunReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	slog.Info("send reconciler: starting", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("send-reconciler")
				w.reconcileStuckEmails(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (w *EmailWorker) reconcileStuckEmails(ctx context.Context) {
	rows, err := w.store.Q().Query(ctx,
		`SELECT e.id FROM emails e
		 WHERE e.status = 'queued' AND e.direction = 'outbound'
		   AND e.created_at < now() - make_interval(mins => $1)
		   AND NOT EXISTS (
		     SELECT 1 FROM email_jobs j
		     WHERE j.email_id = e.id AND j.job_type = 'send'
		       AND j.status IN ('pending', 'running')
		   )
		 LIMIT 100`,
		int(stuckEmailAge.Minutes()),
	)
	if err != nil {
		slog.Error("send reconciler: query failed", "error", err)
		return
	}
	defer rows.Close()

	var emailIDs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			emailIDs = append(emailIDs, id)
		}
	}

	for _, emailID := range emailIDs {
		w.failStuckEmail(ctx, emailID)
	}
	if len(emailIDs) > 0 {
		slog.Warn("send reconciler: failed stuck emails", "count", len(emailIDs))
	}
}

// failStuckEmail marks one stuck email as failed, creates a recovery draft
// when the send did not come from a draft, and tells the frontend.
func (w *EmailWorker) failStuckEmail(ctx context.Context, emailID string) {
	if _, err := w.store.Q().Exec(ctx,
		`UPDATE emails SET status = 'failed', updated_at = now() WHERE id = $1 AND status = 'queued'`,
		emailID,
	); err != nil {
		slog.Error("send reconciler: failed to mark email failed", "email_id", emailID, "error", err)
		return
	}

	// Reuse the draft from the original job when there was one.
	var draftID *string
	_ = w.store.Q().QueryRow(ctx,
		`SELECT draft_id FROM email_jobs WHERE email_id = $1 AND job_type = 'send'
		 ORDER BY created_at DESC LIMIT 1`,
		emailID,
	).Scan(&draftID)
	if draftID == nil || *draftID == "" {
		if recoveryID := w.createRecoveryDraft(ctx, emailID); recoveryID != "" {
			draftID = &recoveryID
		}
	}

	var orgID, threadID, domainID, subject string
	warnIfErr(w.store.Q().QueryRow(ctx,
		`SELECT org_id, thread_id, domain_id, subject FROM emails WHERE id = $1`, emailID,
	).Scan(&orgID, &threadID, &domainID, &subject), "send reconciler: event data lookup", "email_id", emailID)

	payload := map[string]interface{}{
		"email_id": emailID,
		"status":   "failed",
		"subject":  subject,
	}
	if draftID != nil && *draftID != "" {
		payload["draft_id"] = *draftID
	}
	if w.bus != nil {
		w.bus.Publish(ctx, event.Event{
			EventType: event.EmailStatusUpdated,
			OrgID:     orgID,
			DomainID:  domainID,
			ThreadID:  threadID,
			Payload:   payload,
		})
	}
	slog.Warn("send reconciler: email had no live send job, marked failed", "email_id", emailID)
}
