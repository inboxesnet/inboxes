package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventPruner struct {
	DB            *pgxpool.Pool
	RetentionDays int
	Interval      time.Duration
}

func NewEventPruner(db *pgxpool.Pool, retentionDays int, interval time.Duration) *EventPruner {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &EventPruner{DB: db, RetentionDays: retentionDays, Interval: interval}
}

func (ep *EventPruner) Run(ctx context.Context) {
	if ep.RetentionDays <= 0 {
		slog.Info("event pruner: disabled (retention days <= 0)")
		return
	}
	slog.Info("event pruner: starting", "retention_days", ep.RetentionDays, "interval", ep.Interval)

	// Run once on startup after a short delay
	select {
	case <-time.After(1 * time.Minute):
		func() {
			defer util.RecoverWorker("event-pruner")
			ep.prune(ctx)
		}()
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(ep.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("event-pruner")
				ep.prune(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (ep *EventPruner) prune(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -ep.RetentionDays)

	// Delete in batches to avoid long-running transactions
	const batchSize = 5000
	var totalDeleted int64

	for {
		tag, err := ep.DB.Exec(ctx,
			`DELETE FROM events WHERE id IN (
				SELECT id FROM events WHERE created_at < $1 LIMIT $2
			)`,
			cutoff, batchSize,
		)
		if err != nil {
			slog.Error("event pruner: failed to delete", "error", err, "cutoff", cutoff)
			return
		}
		totalDeleted += tag.RowsAffected()
		if tag.RowsAffected() < batchSize {
			break
		}
		// Small pause between batches to reduce DB pressure
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}

	if totalDeleted > 0 {
		slog.Info("event pruner: pruned old events", "deleted", totalDeleted, "cutoff", cutoff.Format(time.RFC3339))
	}
}
