package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TrashCollector struct {
	DB      *pgxpool.Pool
	Enabled bool
}

func NewTrashCollector(db *pgxpool.Pool, enabled bool) *TrashCollector {
	return &TrashCollector{DB: db, Enabled: enabled}
}

func (tc *TrashCollector) Run(ctx context.Context) {
	if !tc.Enabled {
		return
	}
	slog.Info("trash collector: starting", "interval", "1h")
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tc.collect(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (tc *TrashCollector) collect(ctx context.Context) {
	tag, err := tc.DB.Exec(ctx,
		`UPDATE threads SET folder = 'deleted_forever', updated_at = now()
		 WHERE folder = 'trash' AND trash_expires_at < now()`,
	)
	if err != nil {
		slog.Error("trash collector: failed to collect", "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		slog.Info("trash collector: collected expired threads", "count", tag.RowsAffected())
	}
}
