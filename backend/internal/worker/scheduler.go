package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Scheduler drives time-based actions: it releases scheduled send jobs to
// the work queue when they come due, and it wakes snoozed threads.
type Scheduler struct {
	DB       *pgxpool.Pool
	RDB      *redis.Client
	Bus      *event.Bus
	Interval time.Duration
}

func NewScheduler(db *pgxpool.Pool, rdb *redis.Client, bus *event.Bus, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{DB: db, RDB: rdb, Bus: bus, Interval: interval}
}

func (s *Scheduler) Run(ctx context.Context) {
	slog.Info("scheduler: starting", "interval", s.Interval)
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("scheduler")
				s.releaseDueSends(ctx)
				s.wakeSnoozedThreads(ctx)
			}()
		case <-ctx.Done():
			slog.Info("scheduler: stopped")
			return
		}
	}
}

// ReleaseDueSendsForTest exposes one release tick for integration tests.
func (s *Scheduler) ReleaseDueSendsForTest(ctx context.Context) { s.releaseDueSends(ctx) }

// WakeSnoozedThreadsForTest exposes one wake tick for integration tests.
func (s *Scheduler) WakeSnoozedThreadsForTest(ctx context.Context) { s.wakeSnoozedThreads(ctx) }

// releaseDueSends moves due scheduled jobs onto the work queue. The state
// lives in Postgres (email_jobs.run_after), so a Redis restart cannot lose
// or early-fire a scheduled email.
func (s *Scheduler) releaseDueSends(ctx context.Context) {
	rows, err := s.DB.Query(ctx,
		`SELECT id, email_id FROM email_jobs
		 WHERE status = 'pending' AND run_after IS NOT NULL AND run_after <= now()`)
	if err != nil {
		slog.Error("scheduler: due sends query failed", "error", err)
		return
	}
	defer rows.Close()

	type due struct {
		jobID   string
		emailID *string
	}
	var dueJobs []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.jobID, &d.emailID) == nil {
			dueJobs = append(dueJobs, d)
		}
	}
	rows.Close()

	for _, d := range dueJobs {
		// The conditional update wins only once: a cancelled schedule or a
		// concurrent scheduler instance cannot release the job twice.
		res, err := s.DB.Exec(ctx,
			`UPDATE email_jobs SET run_after = NULL, updated_at = now()
			 WHERE id = $1 AND status = 'pending' AND run_after IS NOT NULL`,
			d.jobID)
		if err != nil || res.RowsAffected() == 0 {
			continue
		}
		if d.emailID != nil {
			if _, err := s.DB.Exec(ctx,
				`UPDATE emails SET status = 'queued', updated_at = now()
				 WHERE id = $1 AND status = 'scheduled'`, *d.emailID); err != nil {
				slog.Error("scheduler: email status update failed", "email_id", *d.emailID, "error", err)
			}
		}
		if err := s.RDB.LPush(ctx, "email:jobs", d.jobID).Err(); err != nil {
			// The orphan recovery sweep re-enqueues pending jobs with a null
			// run_after, so a failed push here only delays the send.
			slog.Error("scheduler: lpush failed", "job_id", d.jobID, "error", err)
			continue
		}
		slog.Info("scheduler: released scheduled send", "job_id", d.jobID)
	}
}

// wakeSnoozedThreads clears expired snoozes and tells clients to refresh.
func (s *Scheduler) wakeSnoozedThreads(ctx context.Context) {
	rows, err := s.DB.Query(ctx,
		`SELECT id, org_id, domain_id FROM threads
		 WHERE snoozed_until IS NOT NULL AND snoozed_until <= now() AND deleted_at IS NULL`)
	if err != nil {
		slog.Error("scheduler: snoozed threads query failed", "error", err)
		return
	}
	defer rows.Close()

	type woken struct{ threadID, orgID, domainID string }
	var wake []woken
	for rows.Next() {
		var w woken
		if rows.Scan(&w.threadID, &w.orgID, &w.domainID) == nil {
			wake = append(wake, w)
		}
	}
	rows.Close()

	for _, t := range wake {
		res, err := s.DB.Exec(ctx,
			`UPDATE threads SET snoozed_until = NULL, updated_at = now()
			 WHERE id = $1 AND snoozed_until IS NOT NULL AND snoozed_until <= now()`,
			t.threadID)
		if err != nil || res.RowsAffected() == 0 {
			continue
		}
		if s.Bus != nil {
			s.Bus.Publish(ctx, event.Event{
				EventType: event.ThreadUnsnoozed,
				OrgID:     t.orgID,
				DomainID:  t.domainID,
				ThreadID:  t.threadID,
				Payload:   map[string]interface{}{"thread_id": t.threadID},
			})
		}
		slog.Info("scheduler: thread woke from snooze", "thread_id", t.threadID)
	}
}
