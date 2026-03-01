package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TrashCollector struct {
	DB       *pgxpool.Pool
	Bus      *event.Bus
	Enabled  bool
	Interval time.Duration
}

func NewTrashCollector(db *pgxpool.Pool, bus *event.Bus, enabled bool, interval time.Duration) *TrashCollector {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &TrashCollector{DB: db, Bus: bus, Enabled: enabled, Interval: interval}
}

func (tc *TrashCollector) Run(ctx context.Context) {
	if !tc.Enabled {
		return
	}
	slog.Info("trash collector: starting", "interval", tc.Interval)
	ticker := time.NewTicker(tc.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("trash-collector")
				tc.collect(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (tc *TrashCollector) collect(ctx context.Context) {
	// Fetch expired thread IDs + org/domain info before deleting, so we can publish events
	rows, err := tc.DB.Query(ctx,
		`SELECT t.id, t.org_id, t.domain_id FROM threads t
		 JOIN thread_labels tl ON tl.thread_id = t.id AND tl.label = 'trash'
		 WHERE t.trash_expires_at < now() AND t.deleted_at IS NULL`)
	if err != nil {
		slog.Error("trash collector: failed to query expired threads", "error", err)
		return
	}
	type expiredThread struct {
		ID, OrgID, DomainID string
	}
	var expired []expiredThread
	for rows.Next() {
		var t expiredThread
		if rows.Scan(&t.ID, &t.OrgID, &t.DomainID) == nil {
			expired = append(expired, t)
		}
	}
	rows.Close()

	if len(expired) == 0 {
		return
	}

	ids := make([]string, len(expired))
	for i, t := range expired {
		ids[i] = t.ID
	}

	tag, err := tc.DB.Exec(ctx,
		`UPDATE threads SET deleted_at = now(), updated_at = now() WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		slog.Error("trash collector: failed to collect", "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		slog.Info("trash collector: collected expired threads", "count", tag.RowsAffected())
		// Clean up orphaned labels
		if _, err := tc.DB.Exec(ctx,
			`DELETE FROM thread_labels WHERE thread_id = ANY($1::uuid[])`, ids); err != nil {
			slog.Error("trash collector: failed to clean up labels", "error", err)
		}
		// Publish thread.deleted events
		if tc.Bus != nil {
			for _, t := range expired {
				tc.Bus.Publish(ctx, event.Event{
					EventType: event.ThreadDeleted,
					OrgID:     t.OrgID,
					DomainID:  t.DomainID,
					ThreadID:  t.ID,
				})
			}
		}
	}
}
