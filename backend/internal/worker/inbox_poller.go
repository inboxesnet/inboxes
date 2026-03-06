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
		"SELECT domain FROM domains WHERE org_id = $1 AND hidden = false AND status = 'active'", orgID)
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

	totalEnqueued := 0
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

		enqueued := 0
		hitKnown := false

		for _, email := range resp.Data {
			if email.ID == "" {
				continue
			}

			// Skip emails for hidden/unselected domains
			if !emailMatchesVisibleDomain(email.To, visibleDomains) {
				continue
			}

			// Already in DB?
			var existingID string
			if err := p.Store.Q().QueryRow(ctx,
				"SELECT id FROM emails WHERE resend_email_id = $1", email.ID,
			).Scan(&existingID); err == nil {
				hitKnown = true
				continue
			}

			// Already queued?
			var existingJobID string
			if err := p.Store.Q().QueryRow(ctx,
				"SELECT id FROM email_jobs WHERE resend_email_id = $1 AND status IN ('pending', 'running')", email.ID,
			).Scan(&existingJobID); err == nil {
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

			var jobID string
			if err := p.Store.Q().QueryRow(ctx,
				`INSERT INTO email_jobs (org_id, user_id, job_type, resend_email_id, webhook_data)
				 VALUES ($1, $2, 'fetch', $3, $4)
				 ON CONFLICT (resend_email_id) WHERE status IN ('pending', 'running') DO NOTHING
				 RETURNING id`,
				orgID, adminUserID, email.ID, webhookDataJSON,
			).Scan(&jobID); err != nil {
				continue
			}

			if err := p.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
				slog.Error("inbox poller: redis lpush failed", "job_id", jobID, "error", err)
				continue
			}
			enqueued++
		}

		totalEnqueued += enqueued

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

	if totalEnqueued > 0 {
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
		"SELECT domain FROM domains WHERE org_id = $1 AND hidden = false AND status = 'active'", orgID)
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

	totalEnqueued := 0
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

		enqueued := 0
		hitKnown := false

		for _, email := range resp.Data {
			if email.ID == "" {
				continue
			}

			// Skip emails from hidden/unselected domains (sent emails: check From address)
			fromParts := strings.Split(service.ExtractEmail(email.From), "@")
			if len(fromParts) == 2 && !visibleDomains[fromParts[1]] {
				continue
			}

			// Already in DB?
			var existingID string
			if err := p.Store.Q().QueryRow(ctx,
				"SELECT id FROM emails WHERE resend_email_id = $1", email.ID,
			).Scan(&existingID); err == nil {
				hitKnown = true
				continue
			}

			// Already queued?
			var existingJobID string
			if err := p.Store.Q().QueryRow(ctx,
				"SELECT id FROM email_jobs WHERE resend_email_id = $1 AND status IN ('pending', 'running')", email.ID,
			).Scan(&existingJobID); err == nil {
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
				"created_at": email.CreatedAt,
			}
			sentDataJSON, err := json.Marshal(sentData)
			if err != nil {
				slog.Error("inbox poller: failed to marshal sent email data", "error", err)
				continue
			}

			var jobID string
			if err := p.Store.Q().QueryRow(ctx,
				`INSERT INTO email_jobs (org_id, user_id, job_type, resend_email_id, webhook_data)
				 VALUES ($1, $2, 'fetch_sent', $3, $4)
				 ON CONFLICT (resend_email_id) WHERE status IN ('pending', 'running') DO NOTHING
				 RETURNING id`,
				orgID, adminUserID, email.ID, sentDataJSON,
			).Scan(&jobID); err != nil {
				continue
			}

			if err := p.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
				slog.Error("inbox poller: redis lpush failed (sent)", "job_id", jobID, "error", err)
				continue
			}
			enqueued++
		}

		totalEnqueued += enqueued

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

	if totalEnqueued > 0 {
		slog.Info("inbox poller: enqueued fetch_sent jobs", "org_id", orgID, "count", totalEnqueued)
	}
}

// emailMatchesVisibleDomain checks if any recipient address belongs to a visible domain.
func emailMatchesVisibleDomain(recipients []string, visibleDomains map[string]bool) bool {
	for _, addr := range recipients {
		parts := strings.Split(addr, "@")
		if len(parts) == 2 && visibleDomains[parts[1]] {
			return true
		}
	}
	return false
}
