package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/redis/go-redis/v9"
)

const emailJobsQueue = "email:jobs"

type EmailWorker struct {
	store     store.Store
	rdb       *redis.Client
	resendSvc *service.ResendService
	bus       *event.Bus
	limiter   *OrgLimiterMap
	stripeKey string
}

func NewEmailWorker(store store.Store, rdb *redis.Client, resendSvc *service.ResendService, bus *event.Bus, limiter *OrgLimiterMap, stripeKey string) *EmailWorker {
	return &EmailWorker{store: store, rdb: rdb, resendSvc: resendSvc, bus: bus, limiter: limiter, stripeKey: stripeKey}
}

// Run is the main BRPOP loop that processes email jobs from Redis.
func (w *EmailWorker) Run(ctx context.Context) {
	slog.Info("email worker: started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("email worker: stopped")
			return
		default:
		}

		w.runOnce(ctx)
	}
}

func (w *EmailWorker) runOnce(ctx context.Context) {
	defer util.RecoverWorker("email-worker")

	result, err := w.rdb.BRPop(ctx, 5*time.Second, emailJobsQueue).Result()
	if err != nil {
		if err == redis.Nil || ctx.Err() != nil {
			return
		}
		slog.Error("email worker: BRPOP error", "error", err)
		time.Sleep(time.Second)
		return
	}
	if len(result) < 2 {
		return
	}

	jobID := result[1]
	w.processJob(ctx, jobID)
}

func (w *EmailWorker) processJob(ctx context.Context, jobID string) {
	var panicErr error
	defer util.RecoverWorkerJob("email-worker-job", &panicErr)
	defer func() {
		if panicErr != nil {
			// Mark the job as failed with the panic message
			if _, err := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='failed', error_message=$1, updated_at=now() WHERE id=$2`,
				panicErr.Error(), jobID,
			); err != nil {
				slog.Error("email worker: failed to mark panicked job as failed", "job_id", jobID, "error", err)
			}
		}
	}()

	var orgID, userID, jobType, status string
	var retryCount, maxRetries int
	err := w.store.Q().QueryRow(ctx,
		`SELECT org_id, user_id, job_type, status, retry_count, max_retries
		 FROM email_jobs WHERE id = $1`, jobID,
	).Scan(&orgID, &userID, &jobType, &status, &retryCount, &maxRetries)
	if err != nil {
		slog.Error("email worker: failed to load job", "job_id", jobID, "error", err)
		return
	}

	if shouldSkipJob(status, retryCount, maxRetries) {
		slog.Info("email worker: skipping job", "job_id", jobID, "status", status)
		return
	}

	// Check org is not deleted
	var orgDeletedAt *time.Time
	if err := w.store.Q().QueryRow(ctx,
		"SELECT deleted_at FROM orgs WHERE id = $1", orgID,
	).Scan(&orgDeletedAt); err != nil || orgDeletedAt != nil {
		slog.Warn("email worker: skipping job for deleted org", "job_id", jobID, "org_id", orgID)
		if _, err := w.store.Q().Exec(ctx,
			`UPDATE email_jobs SET status='cancelled', error_message='org deleted', updated_at=now() WHERE id=$1`,
			jobID,
		); err != nil {
			slog.Error("email worker: failed to cancel job for deleted org", "job_id", jobID, "error", err)
		}
		return
	}

	// Plan check at send time — only for send jobs when Stripe is configured
	if jobType == "send" && w.stripeKey != "" {
		if !w.isPlanActive(ctx, orgID) {
			slog.Warn("email worker: subscription inactive, failing send job", "job_id", jobID, "org_id", orgID)
			if _, err := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='failed', error_message='subscription inactive', updated_at=now() WHERE id=$1`,
				jobID,
			); err != nil {
				slog.Error("email worker: failed to mark job failed for inactive plan", "job_id", jobID, "error", err)
			}
			// Mark the email as failed too
			var emailID *string
			if err := w.store.Q().QueryRow(ctx, `SELECT email_id FROM email_jobs WHERE id=$1`, jobID).Scan(&emailID); err == nil && emailID != nil {
				w.store.Q().Exec(ctx, `UPDATE emails SET status='failed', updated_at=now() WHERE id=$1`, *emailID)
			}
			return
		}
	}

	// Mark running
	_, err = w.store.Q().Exec(ctx,
		`UPDATE email_jobs SET status='running', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
		jobID,
	)
	if err != nil {
		slog.Error("email worker: failed to mark running", "job_id", jobID, "error", err)
		return
	}

	slog.Info("email worker: processing job", "job_id", jobID, "type", jobType, "org_id", orgID)

	heartCtx, heartCancel := context.WithCancel(ctx)
	defer heartCancel()
	util.SafeGo("email-heartbeat", func() {
		w.heartbeat(heartCtx, jobID)
	})

	var processErr error
	switch jobType {
	case "send":
		processErr = w.processSend(ctx, jobID, orgID, userID)
	case "fetch":
		processErr = w.processFetch(ctx, jobID, orgID, userID)
	case "fetch_sent":
		processErr = w.processFetchSent(ctx, jobID, orgID, userID)
	default:
		processErr = errors.New("unknown job type: " + jobType)
	}

	heartCancel()

	if processErr != nil {
		slog.Error("email worker: job failed", "job_id", jobID, "type", jobType, "error", processErr)
		w.failJob(ctx, jobID, retryCount, maxRetries, processErr)
		return
	}

	if _, err := w.store.Q().Exec(ctx,
		`UPDATE email_jobs SET status='completed', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
		jobID,
	); err != nil {
		slog.Error("email worker: failed to mark completed", "job_id", jobID, "error", err)
	}
	slog.Info("email worker: job completed", "job_id", jobID, "type", jobType)
}

func (w *EmailWorker) heartbeat(ctx context.Context, jobID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET heartbeat_at=now() WHERE id=$1`, jobID,
			); err != nil {
				slog.Error("email worker: heartbeat failed", "job_id", jobID, "error", err)
			}
		}
	}
}

func (w *EmailWorker) failJob(ctx context.Context, jobID string, retryCount, maxRetries int, err error) {
	retryable := isRetryableFailure(err)
	domainErr := isDomainFailure(err)
	newRetry := retryCount + 1

	// If this is a domain-level error, mark the domain as disconnected
	if domainErr {
		w.markDomainDisconnected(ctx, jobID, err)
	}

	if shouldPermanentlyFail(retryable, newRetry, maxRetries) || domainErr {
		if _, execErr := w.store.Q().Exec(ctx,
			`UPDATE email_jobs SET status='failed', error_message=$1, retry_count=$2,
			 heartbeat_at=now(), updated_at=now() WHERE id=$3`,
			err.Error(), newRetry, jobID,
		); execErr != nil {
			slog.Error("email worker: failed to mark job failed", "job_id", jobID, "error", execErr)
		}

		// If this was a send job, mark the email as failed
		var emailID *string
		var draftID *string
		warnIfErr(w.store.Q().QueryRow(ctx,
			`SELECT email_id, draft_id FROM email_jobs WHERE id=$1`, jobID,
		).Scan(&emailID, &draftID), "email worker: email_id lookup for failed job", "job_id", jobID)
		if emailID != nil {
			if _, execErr := w.store.Q().Exec(ctx,
				`UPDATE emails SET status='failed', updated_at=now() WHERE id=$1`, *emailID,
			); execErr != nil {
				slog.Error("email worker: failed to mark email failed", "email_id", *emailID, "error", execErr)
			}

			// Create recovery draft if this was a direct send (no existing draft)
			if draftID == nil || *draftID == "" {
				recoveryDraftID := w.createRecoveryDraft(ctx, *emailID)
				if recoveryDraftID != "" {
					draftID = &recoveryDraftID
				}
			}

			// Publish status update event
			var orgID, threadID, domainID, emailSubject string
			warnIfErr(w.store.Q().QueryRow(ctx,
				`SELECT org_id, thread_id, domain_id, subject FROM emails WHERE id=$1`, *emailID,
			).Scan(&orgID, &threadID, &domainID, &emailSubject), "email worker: event data lookup for failed email", "email_id", *emailID)
			payload := map[string]interface{}{
				"email_id": *emailID,
				"status":   "failed",
				"subject":  emailSubject,
			}
			if draftID != nil && *draftID != "" {
				payload["draft_id"] = *draftID
			}
			w.bus.Publish(ctx, event.Event{
				EventType: event.EmailStatusUpdated,
				OrgID:     orgID,
				DomainID:  domainID,
				ThreadID:  threadID,
				Payload:   payload,
			})
		}

		slog.Error("email worker: job permanently failed", "job_id", jobID, "retries", newRetry, "domain_error", domainErr)
		return
	}

	backoff := calcBackoff(retryCount)

	if _, execErr := w.store.Q().Exec(ctx,
		`UPDATE email_jobs SET status='pending', error_message=$1, retry_count=$2,
		 heartbeat_at=now(), updated_at=now() WHERE id=$3`,
		err.Error(), newRetry, jobID,
	); execErr != nil {
		slog.Error("email worker: failed to mark job pending for retry", "job_id", jobID, "error", execErr)
	}

	// Re-enqueue after backoff
	util.SafeGo("email-retry-enqueue", func() {
		time.Sleep(backoff)
		retryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := w.rdb.LPush(retryCtx, emailJobsQueue, jobID).Err(); err != nil {
			slog.Error("email worker: redis lpush retry failed", "job_id", jobID, "error", err)
		}
		slog.Warn("email worker: job re-enqueued after backoff", "job_id", jobID, "retry", newRetry, "backoff", backoff)
	})
}

// createRecoveryDraft creates a draft from the failed email so the user can retry.
// Returns the new draft ID, or empty string on failure.
func (w *EmailWorker) createRecoveryDraft(ctx context.Context, emailID string) string {
	var orgID, userID, domainID, fromAddr, subject, bodyHTML, bodyPlain string
	var threadID, inReplyTo *string
	var toJSON, ccJSON, bccJSON json.RawMessage
	var attachmentIDs json.RawMessage

	err := w.store.Q().QueryRow(ctx,
		`SELECT org_id, user_id, domain_id, from_address, to_addresses, cc_addresses, bcc_addresses,
		 subject, body_html, body_plain, thread_id, in_reply_to, COALESCE(attachment_ids, '[]')
		 FROM emails WHERE id = $1`,
		emailID,
	).Scan(&orgID, &userID, &domainID, &fromAddr, &toJSON, &ccJSON, &bccJSON,
		&subject, &bodyHTML, &bodyPlain, &threadID, &inReplyTo, &attachmentIDs)
	if err != nil {
		slog.Error("email worker: createRecoveryDraft query failed", "email_id", emailID, "error", err)
		return ""
	}

	kind := "compose"
	if inReplyTo != nil && *inReplyTo != "" {
		kind = "reply"
	}

	var draftID string
	err = w.store.Q().QueryRow(ctx,
		`INSERT INTO drafts (org_id, user_id, domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain, attachment_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id`,
		orgID, userID, domainID, threadID, kind, subject, fromAddr,
		toJSON, ccJSON, bccJSON, bodyHTML, bodyPlain, attachmentIDs,
	).Scan(&draftID)
	if err != nil {
		slog.Error("email worker: createRecoveryDraft insert failed", "email_id", emailID, "error", err)
		return ""
	}

	slog.Info("email worker: created recovery draft", "email_id", emailID, "draft_id", draftID)
	return draftID
}

// RunStaleRecovery periodically checks for running jobs with stale heartbeats.
func (w *EmailWorker) RunStaleRecovery(ctx context.Context) {
	slog.Info("email worker: stale recovery started")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("email worker: stale recovery stopped")
			return
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("email-stale-recovery")
				w.recoverStaleJobs(ctx)
				w.recoverOrphanedJobs(ctx)
			}()
		}
	}
}

func (w *EmailWorker) recoverStaleJobs(ctx context.Context) {
	rows, err := w.store.Q().Query(ctx,
		`SELECT id, retry_count, max_retries FROM email_jobs
		 WHERE status = 'running' AND heartbeat_at < now() - interval '90 seconds'`,
	)
	if err != nil {
		slog.Error("email worker: stale recovery query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var retryCount, maxRetries int
		rows.Scan(&id, &retryCount, &maxRetries)

		if retryCount >= maxRetries {
			if _, err := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='failed', error_message='stale: max retries exceeded',
				 updated_at=now() WHERE id=$1`, id,
			); err != nil {
				slog.Error("email worker: stale recovery mark failed", "job_id", id, "error", err)
			}
			slog.Warn("email worker: stale job permanently failed", "job_id", id)
			continue
		}

		_, err := w.store.Q().Exec(ctx,
			`UPDATE email_jobs SET status='pending', retry_count=retry_count+1,
			 error_message='recovered from stale heartbeat', updated_at=now() WHERE id=$1`, id,
		)
		if err == nil {
			if lpushErr := w.rdb.LPush(ctx, emailJobsQueue, id).Err(); lpushErr != nil {
				slog.Error("email worker: stale recovery lpush failed", "job_id", id, "error", lpushErr)
			}
			slog.Warn("email worker: recovered stale job", "job_id", id)
		}
	}
}

// recoverOrphanedJobs re-enqueues pending jobs that were never pushed to Redis
// (or whose Redis push failed). Uses updated_at to avoid interfering with retry backoffs.
func (w *EmailWorker) recoverOrphanedJobs(ctx context.Context) {
	rows, err := w.store.Q().Query(ctx,
		`SELECT id FROM email_jobs
		 WHERE status = 'pending' AND updated_at < now() - interval '5 minutes'`,
	)
	if err != nil {
		slog.Error("email worker: orphan recovery query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		rows.Scan(&id)

		if err := w.rdb.LPush(ctx, emailJobsQueue, id).Err(); err != nil {
			slog.Error("email worker: orphan recovery lpush failed", "job_id", id, "error", err)
			continue
		}
		slog.Warn("email worker: recovered orphaned pending job", "job_id", id)
	}
}

// --- Extracted pure functions for testability ---

// shouldSkipJob returns true when a job should not be processed.
func shouldSkipJob(status string, retryCount, maxRetries int) bool {
	return status == "completed" || (status == "failed" && retryCount >= maxRetries)
}

// isRetryableFailure determines whether the error allows retrying.
func isRetryableFailure(err error) bool {
	if err == nil {
		return true
	}
	var nre *nonRetryableError
	if errors.As(err, &nre) {
		return false
	}
	var resendErr *service.ResendError
	if errors.As(err, &resendErr) {
		return resendErr.IsRetryable()
	}
	return true // assume generic errors are retryable
}

// shouldPermanentlyFail returns true when a job should be marked as permanently failed.
func shouldPermanentlyFail(retryable bool, newRetryCount, maxRetries int) bool {
	return !retryable || newRetryCount >= maxRetries
}

// nonRetryableError wraps errors that should never be retried (e.g. missing domain).
type nonRetryableError struct{ err error }

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// isDomainFailure returns true when the error indicates a domain/account problem
// that should mark the domain as disconnected (not a transient or payload error).
func isDomainFailure(err error) bool {
	if err == nil {
		return false
	}
	var resendErr *service.ResendError
	if errors.As(err, &resendErr) {
		return resendErr.IsDomainError()
	}
	return false
}

// markDomainDisconnected marks the domain associated with a job as disconnected.
func (w *EmailWorker) markDomainDisconnected(ctx context.Context, jobID string, sendErr error) {
	var domainID, orgID string
	err := w.store.Q().QueryRow(ctx,
		`SELECT ej.domain_id, ej.org_id FROM email_jobs ej WHERE ej.id = $1`,
		jobID,
	).Scan(&domainID, &orgID)
	if err != nil || domainID == "" {
		return
	}

	tag, err := w.store.Q().Exec(ctx,
		`UPDATE domains SET status = 'disconnected', updated_at = now()
		 WHERE id = $1 AND status != 'disconnected'`,
		domainID,
	)
	if err != nil {
		slog.Error("email worker: failed to mark domain disconnected", "domain_id", domainID, "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		slog.Warn("email worker: domain marked disconnected due to send failure",
			"domain_id", domainID, "org_id", orgID, "reason", sendErr.Error())
	}
}

// isPlanActive checks whether an org has an active subscription.
// Mirrors the RequirePlan middleware logic.
func (w *EmailWorker) isPlanActive(ctx context.Context, orgID string) bool {
	var plan string
	var planExpiresAt *time.Time
	if err := w.store.Q().QueryRow(ctx,
		"SELECT plan, plan_expires_at FROM orgs WHERE id = $1 AND deleted_at IS NULL", orgID,
	).Scan(&plan, &planExpiresAt); err != nil {
		return false
	}
	if plan == "pro" || plan == "past_due" {
		return true
	}
	if plan == "cancelled" && planExpiresAt != nil && planExpiresAt.After(time.Now()) {
		return true
	}
	return false
}

// calcBackoff computes exponential backoff: 2^retry × 5s, capped at 5 min.
func calcBackoff(retryCount int) time.Duration {
	backoffSecs := float64(int(1)<<uint(retryCount)) * 5
	if backoffSecs > 300 {
		backoffSecs = 300
	}
	return time.Duration(backoffSecs) * time.Second
}
