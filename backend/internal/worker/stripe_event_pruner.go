package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StripeEventPruner removes old stripe_events rows to keep the dedup table small.
type StripeEventPruner struct {
	DB       *pgxpool.Pool
	Interval time.Duration
}

func NewStripeEventPruner(db *pgxpool.Pool, interval time.Duration) *StripeEventPruner {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &StripeEventPruner{DB: db, Interval: interval}
}

func (p *StripeEventPruner) Run(ctx context.Context) {
	slog.Info("stripe event pruner: starting", "interval", p.Interval, "retention", "7d")

	// Run once after a short delay
	select {
	case <-time.After(2 * time.Minute):
		func() {
			defer util.RecoverWorker("stripe-event-pruner")
			p.prune(ctx)
		}()
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("stripe-event-pruner")
				p.prune(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (p *StripeEventPruner) prune(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -7)
	tag, err := p.DB.Exec(ctx,
		`DELETE FROM stripe_events WHERE processed_at < $1`, cutoff,
	)
	if err != nil {
		slog.Error("stripe event pruner: delete failed", "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		slog.Info("stripe event pruner: cleaned up old events", "deleted", tag.RowsAffected())
	}
}
