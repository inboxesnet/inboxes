package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/redis/go-redis/v9"
)

// maxPollPages caps pagination to avoid runaway API calls on first poll.
const maxPollPages = 10

type InboxPoller struct {
	Store     store.Store
	RDB       *redis.Client
	ResendSvc *service.ResendService
	Limiter   *queue.OrgLimiterMap
}

func NewInboxPoller(st store.Store, rdb *redis.Client, resendSvc *service.ResendService, limiter *queue.OrgLimiterMap) *InboxPoller {
	return &InboxPoller{
		Store:     st,
		RDB:       rdb,
		ResendSvc: resendSvc,
		Limiter:   limiter,
	}
}

func (p *InboxPoller) Run(ctx context.Context) {
	slog.Info("inbox poller: starting")

	// Initial delay — let migrations and other workers settle
	select {
	case <-time.After(1 * time.Minute):
	case <-ctx.Done():
		return
	}

	// Lightweight check loop — ticks every 30s, checks which orgs are due
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run once immediately after initial delay
	func() {
		defer util.RecoverWorker("inbox-poller")
		p.checkAll(ctx)
	}()

	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("inbox-poller")
				p.checkAll(ctx)
			}()
		case <-ctx.Done():
			slog.Info("inbox poller: stopped")
			return
		}
	}
}

func (p *InboxPoller) checkAll(ctx context.Context) {
	// Find orgs that have auto-poll enabled, an API key, and are due for polling
	rows, err := p.Store.Q().Query(ctx,
		`SELECT id, auto_poll_interval FROM orgs
		 WHERE auto_poll_enabled = true
		   AND deleted_at IS NULL
		   AND resend_api_key_encrypted IS NOT NULL
		   AND (last_polled_at IS NULL OR last_polled_at < now() - (auto_poll_interval || ' seconds')::interval)
		   AND id NOT IN (SELECT org_id FROM sync_jobs WHERE status IN ('pending', 'running'))`)
	if err != nil {
		slog.Error("inbox poller: failed to query orgs", "error", err)
		return
	}
	defer rows.Close()

	type orgEntry struct {
		id       string
		interval int
	}
	var due []orgEntry
	for rows.Next() {
		var e orgEntry
		if rows.Scan(&e.id, &e.interval) == nil {
			due = append(due, e)
		}
	}
	rows.Close()

	if len(due) == 0 {
		return
	}

	for _, org := range due {
		p.pollOrg(ctx, org.id)
		p.pollOrgSent(ctx, org.id)
		// Update last_polled_at regardless of success (avoids tight retry loops on persistent errors)
		if _, err := p.Store.Q().Exec(ctx,
			`UPDATE orgs SET last_polled_at = now() WHERE id = $1`, org.id); err != nil {
			slog.Error("inbox poller: failed to update last_polled_at", "org_id", org.id, "error", err)
		}
	}
}

// --- Resend response types ---

type resendReceivedEmail struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	CC        []string `json:"cc"`
	BCC       []string `json:"bcc"`
	ReplyTo   []string `json:"reply_to"`
	Subject   string   `json:"subject"`
	HTML      string   `json:"html"`
	Text      string   `json:"text"`
	MessageID string   `json:"message_id"`
	CreatedAt string   `json:"created_at"`
}

type resendReceivedListResponse struct {
	Data    []resendReceivedEmail `json:"data"`
	HasMore bool                  `json:"has_more"`
}

type resendSentEmail struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	CC        []string `json:"cc"`
	BCC       []string `json:"bcc"`
	ReplyTo   []string `json:"reply_to"`
	Subject   string   `json:"subject"`
	HTML      string   `json:"html"`
	Text      string   `json:"text"`
	LastEvent string   `json:"last_event"`
	MessageID string   `json:"message_id"`
	CreatedAt string   `json:"created_at"`
}

type resendSentListResponse struct {
	Data    []resendSentEmail `json:"data"`
	HasMore bool              `json:"has_more"`
}

// --- Inbound (received) polling ---

func (p *InboxPoller) pollOrg(ctx context.Context, orgID string) {
	// Find admin user for this org (needed for job creation)
	var adminUserID string
	if err := p.Store.Q().QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&adminUserID); err != nil {
		slog.Error("inbox poller: no admin user found", "org_id", orgID, "error", err)
		return
	}

	// Load visible domain names to filter Resend results (skip hidden domains)
	visibleDomains := make(map[string]bool)
	dRows, dErr := p.Store.Q().Query(ctx,
		"SELECT domain FROM domains WHERE org_id = $1 AND hidden = false AND status IN ('active', 'verified')", orgID)
	if dErr != nil {
		slog.Error("inbox poller: failed to load domains", "org_id", orgID, "error", dErr)
		return
	}
	for dRows.Next() {
		var d string
		if dRows.Scan(&d) == nil {
			visibleDomains[d] = true
		}
	}
	dRows.Close()

	var pending []pendingJob
	cursor := ""

	for page := 0; page < maxPollPages; page++ {
		if err := p.Limiter.WaitForOrg(ctx, orgID); err != nil {
			slog.Error("inbox poller: rate limit wait failed", "org_id", orgID, "error", err)
			return
		}

		path := "/emails/receiving?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}

		respBytes, err := p.ResendSvc.Fetch(ctx, orgID, "GET", path, nil)
		if err != nil {
			slog.Warn("inbox poller: failed to fetch received emails", "org_id", orgID, "error", err)
			return
		}

		var resp resendReceivedListResponse
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			slog.Error("inbox poller: failed to parse response", "org_id", orgID, "error", err)
			return
		}

		if len(resp.Data) == 0 {
			break
		}

		hitKnown := false

		ids := make([]string, 0, len(resp.Data))
		for _, email := range resp.Data {
			if email.ID != "" {
				ids = append(ids, email.ID)
			}
		}
		stored, queued, err := p.knownEmails(ctx, ids)
		if err != nil {
			slog.Error("inbox poller: failed to check known emails", "org_id", orgID, "error", err)
			return
		}

		for _, email := range resp.Data {
			if email.ID == "" {
				continue
			}

			// Skip emails for hidden/unselected domains
			if !emailMatchesVisibleDomain(email.To, visibleDomains) {
				continue
			}

			if stored[email.ID] {
				hitKnown = true
				continue
			}
			if queued[email.ID] {
				continue
			}

			webhookData := map[string]interface{}{
				"email_id":   email.ID,
				"from":       email.From,
				"to":         email.To,
				"cc":         email.CC,
				"bcc":        email.BCC,
				"reply_to":   email.ReplyTo,
				"subject":    email.Subject,
				"html":       email.HTML,
				"text":       email.Text,
				"message_id": email.MessageID,
				"created_at": email.CreatedAt,
			}
			webhookDataJSON, err := json.Marshal(webhookData)
			if err != nil {
				slog.Error("inbox poller: failed to marshal email data", "error", err)
				continue
			}

			pending = append(pending, pendingJob{resendID: email.ID, data: webhookDataJSON})
		}

		// Stop paginating if we hit a known email — we've reached our frontier
		if hitKnown {
			break
		}

		// Stop if no more pages
		if !resp.HasMore || len(resp.Data) < 100 {
			break
		}

		cursor = resp.Data[len(resp.Data)-1].ID
	}

	if totalEnqueued := p.enqueueOldestFirst(ctx, orgID, adminUserID, "fetch", pending); totalEnqueued > 0 {
		slog.Info("inbox poller: enqueued fetch jobs", "org_id", orgID, "count", totalEnqueued)
	}
}

// --- Outbound (sent) polling ---

func (p *InboxPoller) pollOrgSent(ctx context.Context, orgID string) {
	// Find admin user for this org
	var adminUserID string
	if err := p.Store.Q().QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&adminUserID); err != nil {
		slog.Error("inbox poller: no admin user found (sent)", "org_id", orgID, "error", err)
		return
	}

	// Load visible domain names to filter Resend results (skip hidden domains)
	visibleDomains := make(map[string]bool)
	dRows, dErr := p.Store.Q().Query(ctx,
		"SELECT domain FROM domains WHERE org_id = $1 AND hidden = false AND status IN ('active', 'verified')", orgID)
	if dErr != nil {
		slog.Error("inbox poller: failed to load domains (sent)", "org_id", orgID, "error", dErr)
		return
	}
	for dRows.Next() {
		var d string
		if dRows.Scan(&d) == nil {
			visibleDomains[d] = true
		}
	}
	dRows.Close()

	var pending []pendingJob
	cursor := ""

	for page := 0; page < maxPollPages; page++ {
		if err := p.Limiter.WaitForOrg(ctx, orgID); err != nil {
			slog.Error("inbox poller: rate limit wait failed (sent)", "org_id", orgID, "error", err)
			return
		}

		path := "/emails?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}

		respBytes, err := p.ResendSvc.Fetch(ctx, orgID, "GET", path, nil)
		if err != nil {
			slog.Warn("inbox poller: failed to fetch sent emails", "org_id", orgID, "error", err)
			return
		}

		var resp resendSentListResponse
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			slog.Error("inbox poller: failed to parse sent response", "org_id", orgID, "error", err)
			return
		}

		if len(resp.Data) == 0 {
			break
		}

		hitKnown := false

		ids := make([]string, 0, len(resp.Data))
		for _, email := range resp.Data {
			if email.ID != "" {
				ids = append(ids, email.ID)
			}
		}
		stored, queued, err := p.knownEmails(ctx, ids)
		if err != nil {
			slog.Error("inbox poller: failed to check known emails (sent)", "org_id", orgID, "error", err)
			return
		}

		for _, email := range resp.Data {
			if email.ID == "" {
				continue
			}

			// Skip emails from hidden/unselected domains (sent emails: check From address)
			fromParts := strings.Split(service.ExtractEmail(email.From), "@")
			if len(fromParts) == 2 && !visibleDomains[strings.ToLower(fromParts[1])] {
				continue
			}

			if stored[email.ID] {
				hitKnown = true
				continue
			}
			if queued[email.ID] {
				continue
			}

			sentData := map[string]interface{}{
				"email_id":   email.ID,
				"from":       email.From,
				"to":         email.To,
				"cc":         email.CC,
				"bcc":        email.BCC,
				"reply_to":   email.ReplyTo,
				"subject":    email.Subject,
				"last_event": email.LastEvent,
				"message_id": email.MessageID,
				"created_at": email.CreatedAt,
			}
			sentDataJSON, err := json.Marshal(sentData)
			if err != nil {
				slog.Error("inbox poller: failed to marshal sent email data", "error", err)
				continue
			}

			pending = append(pending, pendingJob{resendID: email.ID, data: sentDataJSON})
		}

		// Stop paginating if we hit a known email — we've reached our frontier
		if hitKnown {
			break
		}

		// Stop if no more pages
		if !resp.HasMore || len(resp.Data) < 100 {
			break
		}

		cursor = resp.Data[len(resp.Data)-1].ID
	}

	if totalEnqueued := p.enqueueOldestFirst(ctx, orgID, adminUserID, "fetch_sent", pending); totalEnqueued > 0 {
		slog.Info("inbox poller: enqueued fetch_sent jobs", "org_id", orgID, "count", totalEnqueued)
	}
}

// pendingJob is one listed email that has no row and no open job.
type pendingJob struct {
	resendID string
	data     []byte
}

// knownEmails returns the listed ids that already have an email row
// (stored) and the ids that have an open job (queued). It uses two queries
// for one page.
func (p *InboxPoller) knownEmails(ctx context.Context, ids []string) (stored, queued map[string]bool, err error) {
	stored = make(map[string]bool)
	queued = make(map[string]bool)
	if len(ids) == 0 {
		return stored, queued, nil
	}
	collect := func(dst map[string]bool, query string) error {
		rows, err := p.Store.Q().Query(ctx, query, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			dst[id] = true
		}
		return rows.Err()
	}
	if err := collect(stored, "SELECT resend_email_id FROM emails WHERE resend_email_id = ANY($1)"); err != nil {
		return nil, nil, err
	}
	if err := collect(queued, "SELECT resend_email_id FROM email_jobs WHERE resend_email_id = ANY($1) AND status IN ('pending', 'running')"); err != nil {
		return nil, nil, err
	}
	return stored, queued, nil
}

// enqueueOldestFirst creates the jobs in reverse list order. Resend lists
// the newest email first; the worker takes jobs in push order. A reply must
// not be processed before its parent.
func (p *InboxPoller) enqueueOldestFirst(ctx context.Context, orgID, adminUserID, jobType string, pending []pendingJob) int {
	enqueued := 0
	for i := len(pending) - 1; i >= 0; i-- {
		job := pending[i]
		var jobID string
		if err := p.Store.Q().QueryRow(ctx,
			`INSERT INTO email_jobs (org_id, user_id, job_type, resend_email_id, webhook_data)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (resend_email_id) WHERE status IN ('pending', 'running') DO NOTHING
			 RETURNING id`,
			orgID, adminUserID, jobType, job.resendID, job.data,
		).Scan(&jobID); err != nil {
			continue
		}
		if err := p.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
			slog.Error("inbox poller: redis lpush failed", "job_id", jobID, "type", jobType, "error", err)
			continue
		}
		enqueued++
	}
	return enqueued
}

// emailMatchesVisibleDomain checks if any recipient address belongs to a visible domain.
func emailMatchesVisibleDomain(recipients []string, visibleDomains map[string]bool) bool {
	for _, addr := range recipients {
		parts := strings.Split(addr, "@")
		if len(parts) == 2 && visibleDomains[strings.ToLower(parts[1])] {
			return true
		}
	}
	return false
}
