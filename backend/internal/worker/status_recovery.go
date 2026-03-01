package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatusRecovery periodically polls Resend for the true status of outbound
// emails that are still marked as "received" after 10 minutes.
type StatusRecovery struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Limiter   *queue.OrgLimiterMap
	Interval  time.Duration
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
}

func (sr *StatusRecovery) recover(ctx context.Context) {
	rows, err := sr.DB.Query(ctx,
		`SELECT id, resend_email_id, org_id FROM emails
		 WHERE resend_email_id IS NOT NULL
		   AND status = 'received'
		   AND direction = 'outbound'
		   AND created_at < now() - interval '10 minutes'
		   AND created_at > now() - interval '24 hours'
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
		if err := rows.Scan(&e.ID, &e.ResendEmailID, &e.OrgID); err != nil {
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
		if newStatus == "" || newStatus == "received" {
			continue
		}

		_, err = sr.DB.Exec(ctx,
			"UPDATE emails SET status = $1, updated_at = now() WHERE id = $2",
			newStatus, e.ID,
		)
		if err != nil {
			slog.Error("status recovery: update failed", "email_id", e.ID, "error", err)
			continue
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
