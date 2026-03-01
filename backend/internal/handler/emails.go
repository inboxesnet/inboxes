package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// loadAttachmentsForResend fetches attachment data from the DB and returns the Resend-format attachment array.
func loadAttachmentsForResend(ctx context.Context, db *pgxpool.Pool, ids []string, orgID string) ([]map[string]string, error) {
	rows, err := db.Query(ctx,
		`SELECT filename, content_type, data FROM attachments WHERE id = ANY($1) AND org_id = $2`,
		ids, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []map[string]string
	for rows.Next() {
		var filename, contentType string
		var data []byte
		if rows.Scan(&filename, &contentType, &data) == nil {
			attachments = append(attachments, map[string]string{
				"content":  base64.StdEncoding.EncodeToString(data),
				"filename": filename,
			})
		}
	}
	return attachments, rows.Err()
}

type EmailHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Bus       *event.Bus
	RDB       *redis.Client
}

type sendRequest struct {
	DomainID      string   `json:"domain_id"`
	From          string   `json:"from"`
	To            []string `json:"to"`
	CC            []string `json:"cc"`
	BCC           []string `json:"bcc"`
	Subject       string   `json:"subject"`
	HTML          string   `json:"html"`
	Text          string   `json:"text"`
	ReplyTo       string   `json:"reply_to_thread_id"`
	InReplyTo     string   `json:"in_reply_to"`
	References    []string `json:"references"`
	AttachmentIDs []string `json:"attachment_ids"`
}

func (h *EmailHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req sendRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" || len(req.To) == 0 || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "from, to, and subject are required")
		return
	}
	if err := validateLength(req.Subject, "subject", 500); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate recipient email addresses
	for _, addr := range req.To {
		if err := validateEmail(addr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid To address: "+addr)
			return
		}
	}
	for _, addr := range req.CC {
		if err := validateEmail(addr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid CC address: "+addr)
			return
		}
	}
	for _, addr := range req.BCC {
		if err := validateEmail(addr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid BCC address: "+addr)
			return
		}
	}

	// Body size check (512 KB max)
	if len(req.HTML) > 512*1024 {
		writeError(w, http.StatusBadRequest, "email body too large (max 512 KB)")
		return
	}

	ctx := r.Context()

	// Verify sender is authorized to send from this address
	if !canSendAs(ctx, h.DB, claims.UserID, claims.OrgID, req.From, claims.Role) {
		writeError(w, http.StatusForbidden, "you are not authorized to send from this address")
		return
	}

	// Check domain status before proceeding
	if req.DomainID != "" {
		var domainStatus string
		if err := h.DB.QueryRow(ctx,
			"SELECT status FROM domains WHERE id = $1 AND org_id = $2",
			req.DomainID, claims.OrgID,
		).Scan(&domainStatus); err == nil {
			if domainStatus == "disconnected" || domainStatus == "pending" || domainStatus == "deleted" {
				writeError(w, http.StatusBadRequest, "cannot send email: domain is "+domainStatus)
				return
			}
		}
	}

	slog.Info("email: sending", "from", req.From, "to", req.To, "subject", req.Subject, "domain_id", req.DomainID)

	// Resolve display name for From field
	fromDisplay := resolveFromDisplay(ctx, h.DB, claims.OrgID, req.From)

	// Build Resend payload (serialized to JSON for job storage)
	resendPayload := map[string]interface{}{
		"from":    fromDisplay,
		"to":      req.To,
		"subject": req.Subject,
	}
	if req.HTML != "" {
		resendPayload["html"] = req.HTML
	}
	if req.Text != "" {
		resendPayload["text"] = req.Text
	}
	if len(req.CC) > 0 {
		resendPayload["cc"] = req.CC
	}
	if len(req.BCC) > 0 {
		resendPayload["bcc"] = req.BCC
	}
	if req.InReplyTo != "" {
		headers := map[string]string{"In-Reply-To": req.InReplyTo}
		if len(req.References) > 0 {
			headers["References"] = strings.Join(req.References, " ")
		}
		resendPayload["headers"] = headers
	}

	// Attach files from attachments table
	if len(req.AttachmentIDs) > 0 {
		attachments, err := loadAttachmentsForResend(ctx, h.DB, req.AttachmentIDs, claims.OrgID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to load attachments")
			return
		}
		if len(attachments) > 0 {
			resendPayload["attachments"] = attachments
		}
	}

	resendPayloadJSON, ok := marshalOrFail(w, resendPayload, "failed to prepare email")
	if !ok {
		return
	}

	// Determine domain_id
	domainID := req.DomainID
	if domainID == "" {
		parts := strings.Split(req.From, "@")
		if len(parts) == 2 {
			warnIfErr(h.DB.QueryRow(ctx,
				"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
				claims.OrgID, parts[1],
			).Scan(&domainID), "email: domain lookup failed", "domain", parts[1])
		}
	}

	// Build snippet from outbound text
	snippet := util.TruncateRunes(req.Text, 200)

	// Wrap thread find/create + email INSERT + thread stats + job INSERT in a transaction
	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtx)

	// Find or create thread
	var threadID string
	if req.ReplyTo != "" {
		threadID = req.ReplyTo
	} else {
		participants, ok := marshalOrFail(w, append([]string{req.From}, req.To...), "failed to create email")
		if !ok {
			return
		}
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, snippet)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			claims.OrgID, claims.UserID, domainID, req.Subject, participants, snippet,
		).Scan(&threadID); err != nil {
			slog.Error("email: create thread failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create thread")
			return
		}
		addLabel(dbCtx, tx, threadID, claims.OrgID, "sent")
	}

	// Store email with status='queued'
	toJSON, ok := marshalOrFail(w, req.To, "failed to create email")
	if !ok {
		return
	}
	ccJSON, ok := marshalOrFail(w, req.CC, "failed to create email")
	if !ok {
		return
	}
	bccJSON, ok := marshalOrFail(w, req.BCC, "failed to create email")
	if !ok {
		return
	}
	refsJSON, ok := marshalOrFail(w, req.References, "failed to create email")
	if !ok {
		return
	}

	var emailID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain,
		 status, in_reply_to, references_header)
		 VALUES ($1, $2, $3, $4, 'outbound', $5, $6, $7, $8, $9, $10, $11, 'queued', $12, $13)
		 RETURNING id`,
		threadID, claims.UserID, claims.OrgID, domainID,
		req.From, toJSON, ccJSON, bccJSON, req.Subject, req.HTML, req.Text,
		req.InReplyTo, refsJSON,
	).Scan(&emailID); err != nil {
		slog.Error("email: insert email failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create email")
		return
	}

	// Update thread stats
	if _, err := tx.Exec(dbCtx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, updated_at = now()
		 WHERE id = $1`, threadID, snippet,
	); err != nil {
		slog.Error("email: update thread failed", "thread_id", threadID, "error", err)
	}

	// Create email job
	var jobID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO email_jobs (org_id, user_id, domain_id, job_type, email_id, thread_id, resend_payload)
		 VALUES ($1, $2, $3, 'send', $4, $5, $6)
		 RETURNING id`,
		claims.OrgID, claims.UserID, domainID, emailID, threadID, resendPayloadJSON,
	).Scan(&jobID); err != nil {
		slog.Error("email: create send job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to queue email")
		return
	}

	if err := tx.Commit(dbCtx); err != nil {
		slog.Error("email: commit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send email")
		return
	}

	// Push to Redis queue (outside transaction — best-effort; stale recovery cron handles failures)
	if err := h.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
		slog.Error("email: redis lpush failed", "job_id", jobID, "error", err)
	}

	slog.Info("email: queued", "email_id", emailID, "thread_id", threadID, "job_id", jobID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"email_id":  emailID,
		"thread_id": threadID,
		"job_id":    jobID,
		"status":    "queued",
	})
}

func (h *EmailHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	q := r.URL.Query().Get("q")
	domainID := r.URL.Query().Get("domain_id")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	ctx := r.Context()

	query := `SELECT DISTINCT ON (t.id)
		t.id, t.domain_id, t.subject, t.participant_emails,
		t.last_message_at, t.message_count, t.unread_count,
		t.snippet, t.original_to, t.created_at
		FROM threads t
		JOIN emails e ON e.thread_id = t.id
		WHERE e.org_id = $1 AND e.search_vector @@ plainto_tsquery('english', $2)
		AND t.deleted_at IS NULL`
	args := []interface{}{claims.OrgID, q}
	argIdx := 3

	if domainID != "" {
		query += " AND e.domain_id = $" + strconv.Itoa(argIdx)
		args = append(args, domainID)
		argIdx++
	}

	// Alias visibility filter for non-admins
	if claims.Role != "admin" {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		query += ` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($` + strconv.Itoa(argIdx) + `::text[]))`
		args = append(args, labels)
	}

	query = "SELECT * FROM (" + query + ") sub ORDER BY last_message_at DESC LIMIT 50"

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	var threadIDs []string
	for rows.Next() {
		var id, dID, subject, snippet string
		var originalTo *string
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int

		rows.Scan(&id, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount,
			&snippet, &originalTo, &createdAt)

		t := map[string]interface{}{
			"id":                 id,
			"domain_id":          dID,
			"subject":            subject,
			"participant_emails": participants,
			"last_message_at":    lastMessageAt,
			"message_count":      messageCount,
			"unread_count":       unreadCount,
			"snippet":            snippet,
			"created_at":         createdAt,
		}
		setIfNotNil(t, "original_to", originalTo)
		results = append(results, t)
		threadIDs = append(threadIDs, id)
	}
	if results == nil {
		results = []map[string]interface{}{}
	}

	// Batch-fetch labels for all result threads
	if len(threadIDs) > 0 {
		labelMap := batchFetchLabels(ctx, h.DB, threadIDs)
		for _, t := range results {
			tid := t["id"].(string)
			if lbls, ok := labelMap[tid]; ok {
				t["labels"] = lbls
			} else {
				t["labels"] = []string{}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"threads": results})
}

func (h *EmailHandler) AdminJobs(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()
	rows, err := h.DB.Query(ctx,
		`SELECT id, job_type, status, email_id, thread_id, error_message, attempts, created_at, updated_at
		 FROM email_jobs WHERE org_id = $1
		 ORDER BY created_at DESC LIMIT 100`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	defer rows.Close()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, jobType, status string
		var emailID, threadID, errorMsg *string
		var attempts int
		var createdAt, updatedAt time.Time

		if rows.Scan(&id, &jobType, &status, &emailID, &threadID, &errorMsg, &attempts, &createdAt, &updatedAt) == nil {
			job := map[string]interface{}{
				"id":         id,
				"job_type":   jobType,
				"status":     status,
				"attempts":   attempts,
				"created_at": createdAt,
				"updated_at": updatedAt,
			}
			setIfNotNil(job, "email_id", emailID)
			setIfNotNil(job, "thread_id", threadID)
			setIfNotNil(job, "error_message", errorMsg)
			jobs = append(jobs, job)
		}
	}
	if jobs == nil {
		jobs = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}
