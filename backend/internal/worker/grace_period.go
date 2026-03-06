package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GracePeriodWorker transitions orgs from "cancelled" to "free" after their
// grace period (plan_expires_at) has elapsed. Also handles past_due orgs whose
// grace period expired.
type GracePeriodWorker struct {
	DB       *pgxpool.Pool
	Bus      *event.Bus
	Interval time.Duration
}

func NewGracePeriodWorker(db *pgxpool.Pool, bus *event.Bus, interval time.Duration) *GracePeriodWorker {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &GracePeriodWorker{DB: db, Bus: bus, Interval: interval}
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
	rows, err := w.DB.Query(ctx,
		`UPDATE orgs SET plan = 'free', plan_expires_at = NULL, updated_at = now()
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
