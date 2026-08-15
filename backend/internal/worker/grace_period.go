package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

// expiryNoticeLead is how long before the downgrade the warning email goes out.
const expiryNoticeLead = 3 * 24 * time.Hour

// GracePeriodWorker transitions orgs from "cancelled" to "free" after their
// grace period (plan_expires_at) has elapsed. Also handles past_due orgs whose
// grace period expired. Before the downgrade it emails the org admins a
// warning, once per grace period.
type GracePeriodWorker struct {
	DB       *pgxpool.Pool
	Bus      *event.Bus
	Resend   *service.ResendService
	AppURL   string
	Interval time.Duration
}

func NewGracePeriodWorker(db *pgxpool.Pool, bus *event.Bus, resendSvc *service.ResendService, appURL string, interval time.Duration) *GracePeriodWorker {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &GracePeriodWorker{DB: db, Bus: bus, Resend: resendSvc, AppURL: appURL, Interval: interval}
}

func (w *GracePeriodWorker) Run(ctx context.Context) {
	slog.Info("grace period worker: starting", "interval", w.Interval)

	// Run once after a short delay
	select {
	case <-time.After(1 * time.Minute):
		func() {
			defer util.RecoverWorker("grace-period")
			w.check(ctx)
		}()
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("grace-period")
				w.check(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (w *GracePeriodWorker) check(ctx context.Context) {
	w.sendExpiryNotices(ctx)

	rows, err := w.DB.Query(ctx,
		`UPDATE orgs SET plan = 'free', plan_expires_at = NULL, lapsed_at = now(), updated_at = now()
		 WHERE plan IN ('cancelled', 'past_due')
		   AND plan_expires_at IS NOT NULL
		   AND plan_expires_at < now()
		   AND deleted_at IS NULL
		 RETURNING id`,
	)
	if err != nil {
		slog.Error("grace period worker: query failed", "error", err)
		return
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var orgID string
		if err := rows.Scan(&orgID); err != nil {
			continue
		}
		count++
		slog.Info("grace period worker: transitioned org to free", "org_id", orgID)
		if w.Bus != nil {
			w.Bus.Publish(ctx, event.Event{
				EventType: event.PlanChanged,
				OrgID:     orgID,
				Payload:   map[string]interface{}{"plan": "free"},
			})
		}
	}

	if count > 0 {
		slog.Info("grace period worker: transitioned orgs", "count", count)
	}
}

// sendExpiryNotices emails the org admins before the grace period ends. It
// sends one email per grace period, tracked by expiry_notice_sent_at.
func (w *GracePeriodWorker) sendExpiryNotices(ctx context.Context) {
	if w.Resend == nil {
		return
	}
	rows, err := w.DB.Query(ctx,
		`SELECT id, plan, plan_expires_at FROM orgs
		 WHERE plan IN ('cancelled', 'past_due')
		   AND plan_expires_at IS NOT NULL
		   AND plan_expires_at > now()
		   AND plan_expires_at < now() + make_interval(hours => $1)
		   AND expiry_notice_sent_at IS NULL
		   AND deleted_at IS NULL`,
		int(expiryNoticeLead.Hours()),
	)
	if err != nil {
		slog.Error("grace period worker: notice query failed", "error", err)
		return
	}
	defer rows.Close()

	type noticeOrg struct {
		id        string
		plan      string
		expiresAt time.Time
	}
	var orgs []noticeOrg
	for rows.Next() {
		var o noticeOrg
		if err := rows.Scan(&o.id, &o.plan, &o.expiresAt); err != nil {
			continue
		}
		orgs = append(orgs, o)
	}

	for _, o := range orgs {
		if w.notifyAdmins(ctx, o.id, o.plan, o.expiresAt) {
			if _, err := w.DB.Exec(ctx,
				`UPDATE orgs SET expiry_notice_sent_at = now() WHERE id = $1`, o.id,
			); err != nil {
				slog.Error("grace period worker: mark notice sent failed", "org_id", o.id, "error", err)
			}
		}
	}
}

// notifyAdmins sends the pre-expiry email to the org admins. It reports true
// when the send call did not return an error.
func (w *GracePeriodWorker) notifyAdmins(ctx context.Context, orgID, plan string, expiresAt time.Time) bool {
	rows, err := w.DB.Query(ctx,
		`SELECT email FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active'`, orgID,
	)
	if err != nil {
		slog.Error("grace period worker: admin lookup failed", "org_id", orgID, "error", err)
		return false
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err == nil && email != "" {
			emails = append(emails, email)
		}
	}
	if len(emails) == 0 {
		// No one to warn — do not retry every pass.
		return true
	}

	reason := "Your subscription was cancelled."
	if plan == "past_due" {
		reason = "The last payment for your subscription failed."
	}
	from := w.Resend.GetSystemFrom(ctx)
	if from == "" {
		from = "noreply@inboxes.net"
	}
	dateStr := expiresAt.UTC().Format("January 2, 2006")
	if _, err := w.Resend.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      emails,
		"subject": "Your Inboxes workspace becomes read-only on " + dateStr,
		"html": "<p>" + reason + "</p>" +
			"<p>Your workspace changes to the free plan on <b>" + dateStr + "</b>. " +
			"After that date your team can read mail, but it cannot send mail.</p>" +
			"<p><a href=\"" + w.AppURL + "/billing\">Reactivate your subscription</a> to keep full access.</p>",
	}); err != nil {
		slog.Error("grace period worker: notice email failed", "org_id", orgID, "error", err)
		return false
	}
	slog.Info("grace period worker: expiry notice sent", "org_id", orgID, "admins", len(emails))
	return true
}
