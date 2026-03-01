package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const syncJobsQueue = "sync:jobs"

type SyncWorker struct {
	pool      *pgxpool.Pool
	rdb       *redis.Client
	syncSvc   *service.SyncService
	resendSvc *service.ResendService
	bus       *event.Bus
}

func NewSyncWorker(pool *pgxpool.Pool, rdb *redis.Client, syncSvc *service.SyncService, resendSvc *service.ResendService, bus *event.Bus) *SyncWorker {
	return &SyncWorker{pool: pool, rdb: rdb, syncSvc: syncSvc, resendSvc: resendSvc, bus: bus}
}

// Run is the main BRPOP loop that processes sync jobs from Redis.
func (w *SyncWorker) Run(ctx context.Context) {
	slog.Info("sync worker: started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("sync worker: stopped")
			return
		default:
		}

		w.runOnce(ctx)
	}
}

func (w *SyncWorker) runOnce(ctx context.Context) {
	defer util.RecoverWorker("sync-worker")

	// Block for up to 5s waiting for a job ID on the queue
	result, err := w.rdb.BRPop(ctx, 5*time.Second, syncJobsQueue).Result()
	if err != nil {
		if err == redis.Nil || ctx.Err() != nil {
			return
		}
		slog.Error("sync worker: BRPOP error", "error", err)
		time.Sleep(time.Second)
		return
	}
	if len(result) < 2 {
		return
	}

	jobID := result[1]
	w.processJob(ctx, jobID)
}

func (w *SyncWorker) processJob(ctx context.Context, jobID string) {
	var panicErr error
	defer util.RecoverWorkerJob("sync-worker-job", &panicErr)
	defer func() {
		if panicErr != nil {
			w.failJob(ctx, jobID, 0, 0, "panic: "+panicErr.Error())
		}
	}()

	// Load job from Postgres
	var orgID, userID, sentCursor, receivedCursor, status string
	var retryCount, maxRetries int
	err := w.pool.QueryRow(ctx,
		`SELECT org_id, user_id, sent_cursor, received_cursor, status, retry_count, max_retries
		 FROM sync_jobs WHERE id = $1`, jobID,
	).Scan(&orgID, &userID, &sentCursor, &receivedCursor, &status, &retryCount, &maxRetries)
	if err != nil {
		slog.Error("sync worker: failed to load job", "job_id", jobID, "error", err)
		return
	}

	// Skip completed/failed jobs
	if status == "completed" || (status == "failed" && retryCount >= maxRetries) {
		slog.Info("sync worker: skipping job", "job_id", jobID, "status", status)
		return
	}

	// Mark running
	_, err = w.pool.Exec(ctx,
		`UPDATE sync_jobs SET status='running', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
		jobID,
	)
	if err != nil {
		slog.Error("sync worker: failed to mark running", "job_id", jobID, "error", err)
		return
	}

	slog.Info("sync worker: processing job", "job_id", jobID, "org_id", orgID)

	// Start heartbeat goroutine
	heartCtx, heartCancel := context.WithCancel(ctx)
	defer heartCancel()
	util.SafeGo("sync-heartbeat", func() {
		w.heartbeat(heartCtx, jobID)
	})

	// Load org's non-hidden domains
	rows, err := w.pool.Query(ctx,
		"SELECT id, domain FROM domains WHERE org_id = $1 AND hidden = false", orgID,
	)
	if err != nil {
		w.failJob(ctx, jobID, retryCount, maxRetries, "failed to fetch domains: "+err.Error())
		return
	}
	defer rows.Close()

	domains := make(map[string]string)
	for rows.Next() {
		var id, domain string
		rows.Scan(&id, &domain)
		domains[domain] = id
	}
	rows.Close()

	if len(domains) == 0 {
		w.failJob(ctx, jobID, retryCount, maxRetries, "no visible domains found")
		return
	}

	// Run the sync
	cfg := service.SyncJobConfig{
		JobID:          jobID,
		SentCursor:     sentCursor,
		ReceivedCursor: receivedCursor,
	}

	_, err = w.syncSvc.SyncEmailsWithJob(ctx, orgID, userID, domains, cfg, nil)
	heartCancel()

	if err != nil {
		slog.Error("sync worker: sync failed", "job_id", jobID, "error", err)
		w.failJob(ctx, jobID, retryCount, maxRetries, err.Error())
		return
	}

	// Mark completed
	if _, err := w.pool.Exec(ctx,
		`UPDATE sync_jobs SET status='completed', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
		jobID,
	); err != nil {
		slog.Error("sync worker: failed to mark completed", "job_id", jobID, "error", err)
	}
	slog.Info("sync worker: job completed", "job_id", jobID)
}

func (w *SyncWorker) heartbeat(ctx context.Context, jobID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.pool.Exec(ctx,
				`UPDATE sync_jobs SET heartbeat_at=now() WHERE id=$1`, jobID,
			); err != nil {
				slog.Error("sync worker: heartbeat failed", "job_id", jobID, "error", err)
			}
		}
	}
}

func (w *SyncWorker) failJob(ctx context.Context, jobID string, retryCount, maxRetries int, errMsg string) {
	newRetry := retryCount + 1
	if newRetry >= maxRetries {
		if _, err := w.pool.Exec(ctx,
			`UPDATE sync_jobs SET status='failed', error_message=$1, retry_count=$2,
			 heartbeat_at=now(), updated_at=now() WHERE id=$3`,
			errMsg, newRetry, jobID,
		); err != nil {
			slog.Error("sync worker: failed to mark job failed", "job_id", jobID, "error", err)
		}
		slog.Error("sync worker: job permanently failed", "job_id", jobID, "retries", newRetry)
	} else {
		// Reset to pending for retry
		if _, err := w.pool.Exec(ctx,
			`UPDATE sync_jobs SET status='pending', error_message=$1, retry_count=$2,
			 heartbeat_at=now(), updated_at=now() WHERE id=$3`,
			errMsg, newRetry, jobID,
		); err != nil {
			slog.Error("sync worker: failed to mark job pending", "job_id", jobID, "error", err)
		}
		// Re-enqueue
		if err := w.rdb.LPush(ctx, syncJobsQueue, jobID).Err(); err != nil {
			slog.Error("sync worker: redis lpush retry failed", "job_id", jobID, "error", err)
		}
		slog.Warn("sync worker: job failed, re-enqueued for retry", "job_id", jobID, "retry", newRetry)
	}
}

// RunStaleRecovery periodically checks for running jobs with stale heartbeats
// and re-enqueues them.
func (w *SyncWorker) RunStaleRecovery(ctx context.Context) {
	slog.Info("sync worker: stale recovery started")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("sync worker: stale recovery stopped")
			return
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("sync-stale-recovery")
				w.recoverStaleJobs(ctx)
			}()
		}
	}
}

func (w *SyncWorker) recoverStaleJobs(ctx context.Context) {
	rows, err := w.pool.Query(ctx,
		`SELECT id, retry_count, max_retries FROM sync_jobs
		 WHERE status = 'running' AND heartbeat_at < now() - interval '90 seconds'`,
	)
	if err != nil {
		slog.Error("sync worker: stale recovery query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var retryCount, maxRetries int
		rows.Scan(&id, &retryCount, &maxRetries)

		if retryCount >= maxRetries {
			if _, err := w.pool.Exec(ctx,
				`UPDATE sync_jobs SET status='failed', error_message='stale: max retries exceeded',
				 updated_at=now() WHERE id=$1`, id,
			); err != nil {
				slog.Error("sync worker: stale recovery mark failed", "job_id", id, "error", err)
			}
			slog.Warn("sync worker: stale job permanently failed", "job_id", id)
			continue
		}

		_, err := w.pool.Exec(ctx,
			`UPDATE sync_jobs SET status='pending', retry_count=retry_count+1,
			 error_message='recovered from stale heartbeat', updated_at=now() WHERE id=$1`, id,
		)
		if err == nil {
			if lpushErr := w.rdb.LPush(ctx, syncJobsQueue, id).Err(); lpushErr != nil {
				slog.Error("sync worker: stale recovery lpush failed", "job_id", id, "error", lpushErr)
			}
			slog.Warn("sync worker: recovered stale job", "job_id", id)
		}
	}
}
