package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatusRecovery periodically polls Resend for the true status of outbound
// emails that are still marked as "received" after 10 minutes.
type StatusRecovery struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Limiter   *queue.OrgLimiterMap
	Interval  time.Duration
	// Bus is optional. When set, each status change goes to clients as an
	// email.status_updated event.
	Bus *event.Bus
}

func NewStatusRecovery(db *pgxpool.Pool, resendSvc *service.ResendService, limiter *queue.OrgLimiterMap, interval time.Duration) *StatusRecovery {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &StatusRecovery{DB: db, ResendSvc: resendSvc, Limiter: limiter, Interval: interval}
}

func (sr *StatusRecovery) Run(ctx context.Context) {
	slog.Info("status recovery: starting", "interval", sr.Interval)

	// Initial delay to let other workers start first
	select {
	case <-time.After(2 * time.Minute):
	case <-ctx.Done():
		return
	}

	func() {
		defer util.RecoverWorker("status-recovery")
		sr.recover(ctx)
	}()

	ticker := time.NewTicker(sr.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("status-recovery")
				sr.recover(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

type staleEmail struct {
	ID            string
	ResendEmailID string
	OrgID         string
	Status        string
}

func (sr *StatusRecovery) recover(ctx context.Context) {
	// 'received' catches outbound emails whose status was never advanced.
	// 'sent' catches emails whose delivery webhook never arrived — common
	// for self-hosters without a reachable webhook URL.
	rows, err := sr.DB.Query(ctx,
		`SELECT id, resend_email_id, org_id, status FROM emails
		 WHERE resend_email_id IS NOT NULL
		   AND status IN ('received', 'sent')
		   AND direction = 'outbound'
		   AND created_at < now() - interval '10 minutes'
		   AND created_at > now() - interval '24 hours'
		   AND updated_at < now() - interval '30 minutes'
		 LIMIT 50`,
	)
	if err != nil {
		slog.Error("status recovery: query failed", "error", err)
		return
	}
	defer rows.Close()

	var emails []staleEmail
	for rows.Next() {
		var e staleEmail
		if err := rows.Scan(&e.ID, &e.ResendEmailID, &e.OrgID, &e.Status); err != nil {
			slog.Error("status recovery: scan failed", "error", err)
			continue
		}
		emails = append(emails, e)
	}

	if len(emails) == 0 {
		return
	}

	slog.Info("status recovery: found stale emails", "count", len(emails))

	for _, e := range emails {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := sr.Limiter.WaitForOrg(ctx, e.OrgID); err != nil {
			return // context cancelled
		}

		data, err := sr.ResendSvc.Fetch(ctx, e.OrgID, "GET", "/emails/"+e.ResendEmailID, nil)
		if err != nil {
			slog.Error("status recovery: fetch from Resend failed",
				"email_id", e.ID, "resend_email_id", e.ResendEmailID, "error", err)
			continue
		}

		var resendEmail struct {
			LastEvent string `json:"last_event"`
		}
		if err := json.Unmarshal(data, &resendEmail); err != nil {
			slog.Error("status recovery: parse response failed", "email_id", e.ID, "error", err)
			continue
		}

		newStatus := mapResendEvent(resendEmail.LastEvent)
		if newStatus == "" || newStatus == "received" || newStatus == e.Status {
			// No new information. Bump updated_at so the 30-minute throttle
			// in the query above delays the next poll of this email.
			if _, err := sr.DB.Exec(ctx,
				"UPDATE emails SET updated_at = now() WHERE id = $1", e.ID,
			); err != nil {
				slog.Error("status recovery: throttle bump failed", "email_id", e.ID, "error", err)
			}
			continue
		}

		// Status-order guard: change only a row that is still 'received'
		// or 'sent'. A webhook can set a final status (delivered, bounced,
		// complained) while this poll runs; the poll must not move it back.
		var threadID, domainID, subject string
		err = sr.DB.QueryRow(ctx,
			`UPDATE emails SET status = $1, updated_at = now()
			 WHERE id = $2 AND status IN ('received', 'sent')
			 RETURNING thread_id, domain_id, subject`,
			newStatus, e.ID,
		).Scan(&threadID, &domainID, &subject)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			slog.Error("status recovery: update failed", "email_id", e.ID, "error", err)
			continue
		}

		if sr.Bus != nil {
			sr.Bus.Publish(ctx, event.Event{
				EventType: event.EmailStatusUpdated,
				OrgID:     e.OrgID,
				DomainID:  domainID,
				ThreadID:  threadID,
				Payload: map[string]interface{}{
					"email_id": e.ID,
					"status":   newStatus,
					"subject":  subject,
				},
			})
		}

		slog.Info("status recovery: updated email status",
			"email_id", e.ID, "resend_email_id", e.ResendEmailID, "new_status", newStatus)
	}
}

// mapResendEvent converts a Resend last_event string to our internal status.
func mapResendEvent(event string) string {
	switch event {
	case "email.sent":
		return "sent"
	case "email.delivered":
		return "delivered"
	case "email.bounced":
		return "bounced"
	case "email.complained":
		return "complained"
	default:
		return ""
	}
}
