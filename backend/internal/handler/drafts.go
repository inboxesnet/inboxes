package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type DraftHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Bus       *event.Bus
	RDB       *redis.Client
}

func (h *DraftHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domainID := r.URL.Query().Get("domain_id")
	ctx := r.Context()

	query := `SELECT id, domain_id, thread_id, kind, subject, from_address,
		to_addresses, cc_addresses, bcc_addresses, body_html, body_plain,
		created_at, updated_at, COALESCE(attachment_ids, '[]')
		FROM drafts WHERE user_id = $1 AND org_id = $2`
	args := []interface{}{claims.UserID, claims.OrgID}

	if domainID != "" {
		query += " AND domain_id = $3"
		args = append(args, domainID)
	}
	query += " ORDER BY updated_at DESC"

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list drafts")
		return
	}
	defer rows.Close()

	var drafts []map[string]interface{}
	for rows.Next() {
		var id, domID, kind, subject, fromAddr, bodyHTML, bodyPlain string
		var threadID *string
		var toAddr, ccAddr, bccAddr, attIDs json.RawMessage
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &domID, &threadID, &kind, &subject, &fromAddr,
			&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain,
			&createdAt, &updatedAt, &attIDs)
		if err != nil {
			continue
		}

		draft := map[string]interface{}{
			"id":             id,
			"domain_id":      domID,
			"thread_id":      threadID,
			"kind":           kind,
			"subject":        subject,
			"from_address":   fromAddr,
			"to_addresses":   json.RawMessage(toAddr),
			"cc_addresses":   json.RawMessage(ccAddr),
			"bcc_addresses":  json.RawMessage(bccAddr),
			"body_html":      bodyHTML,
			"body_plain":     bodyPlain,
			"attachment_ids": json.RawMessage(attIDs),
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		}
		drafts = append(drafts, draft)
	}
	if drafts == nil {
		drafts = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"drafts": drafts})
}

func (h *DraftHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		DomainID      string   `json:"domain_id"`
		ThreadID      *string  `json:"thread_id"`
		Kind          string   `json:"kind"`
		Subject       string   `json:"subject"`
		FromAddress   string   `json:"from_address"`
		To            []string `json:"to_addresses"`
		CC            []string `json:"cc_addresses"`
		BCC           []string `json:"bcc_addresses"`
		BodyHTML      string   `json:"body_html"`
		BodyPlain     string   `json:"body_plain"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DomainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	if err := validateLength(req.Subject, "subject", 500); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Kind == "" {
		req.Kind = "compose"
	}

	toJSON, ok := marshalOrFail(w, req.To, "failed to create draft")
	if !ok {
		return
	}
	ccJSON, ok := marshalOrFail(w, req.CC, "failed to create draft")
	if !ok {
		return
	}
	bccJSON, ok := marshalOrFail(w, req.BCC, "failed to create draft")
	if !ok {
		return
	}
	attJSON, ok := marshalOrFail(w, req.AttachmentIDs, "failed to create draft")
	if !ok {
		return
	}

	var id string
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO drafts (org_id, user_id, domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain, attachment_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id`,
		claims.OrgID, claims.UserID, req.DomainID, req.ThreadID, req.Kind,
		req.Subject, req.FromAddress, toJSON, ccJSON, bccJSON,
		req.BodyHTML, req.BodyPlain, attJSON,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create draft")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id})
}

func (h *DraftHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	id := chi.URLParam(r, "id")

	var req struct {
		Subject       *string  `json:"subject"`
		FromAddress   *string  `json:"from_address"`
		To            []string `json:"to_addresses"`
		CC            []string `json:"cc_addresses"`
		BCC           []string `json:"bcc_addresses"`
		BodyHTML      *string  `json:"body_html"`
		BodyPlain     *string  `json:"body_plain"`
		AttachmentIDs []string `json:"attachment_ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Build dynamic update
	setClauses := []string{"updated_at = now()"}
	args := []interface{}{id, claims.UserID}
	argIdx := 3

	if req.Subject != nil {
		setClauses = append(setClauses, "subject = $"+itoa(argIdx))
		args = append(args, *req.Subject)
		argIdx++
	}
	if req.FromAddress != nil {
		setClauses = append(setClauses, "from_address = $"+itoa(argIdx))
		args = append(args, *req.FromAddress)
		argIdx++
	}
	if req.To != nil {
		toJSON, ok := marshalOrFail(w, req.To, "failed to update draft")
		if !ok {
			return
		}
		setClauses = append(setClauses, "to_addresses = $"+itoa(argIdx))
		args = append(args, toJSON)
		argIdx++
	}
	if req.CC != nil {
		ccJSON, ok := marshalOrFail(w, req.CC, "failed to update draft")
		if !ok {
			return
		}
		setClauses = append(setClauses, "cc_addresses = $"+itoa(argIdx))
		args = append(args, ccJSON)
		argIdx++
	}
	if req.BCC != nil {
		bccJSON, ok := marshalOrFail(w, req.BCC, "failed to update draft")
		if !ok {
			return
		}
		setClauses = append(setClauses, "bcc_addresses = $"+itoa(argIdx))
		args = append(args, bccJSON)
		argIdx++
	}
	if req.BodyHTML != nil {
		setClauses = append(setClauses, "body_html = $"+itoa(argIdx))
		args = append(args, *req.BodyHTML)
		argIdx++
	}
	if req.BodyPlain != nil {
		setClauses = append(setClauses, "body_plain = $"+itoa(argIdx))
		args = append(args, *req.BodyPlain)
		argIdx++
	}
	if req.AttachmentIDs != nil {
		attJSON, ok := marshalOrFail(w, req.AttachmentIDs, "failed to update draft")
		if !ok {
			return
		}
		setClauses = append(setClauses, "attachment_ids = $"+itoa(argIdx))
		args = append(args, attJSON)
		argIdx++
	}

	query := "UPDATE drafts SET "
	for i, c := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += c
	}
	query += " WHERE id = $1 AND user_id = $2"

	tag, err := h.DB.Exec(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update draft")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DraftHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	id := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(r.Context(),
		"DELETE FROM drafts WHERE id = $1 AND user_id = $2",
		id, claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete draft")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DraftHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Idempotency: reject if a send job already exists for this draft
	var alreadySending bool
	if err := h.DB.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_jobs WHERE draft_id = $1 AND job_type = 'send')`,
		id).Scan(&alreadySending); err != nil {
		slog.Error("draft: idempotency check failed", "draft_id", id, "error", err)
	}
	if alreadySending {
		writeError(w, http.StatusConflict, "this draft is already being sent")
		return
	}

	// Fetch draft
	var domainID, kind, subject, fromAddr, bodyHTML, bodyPlain string
	var threadID *string
	var toAddr, ccAddr, bccAddr json.RawMessage
	var attachmentIDsRaw json.RawMessage

	err := h.DB.QueryRow(ctx,
		`SELECT domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain,
		 COALESCE(attachment_ids, '[]')
		 FROM drafts WHERE id = $1 AND user_id = $2`,
		id, claims.UserID,
	).Scan(&domainID, &threadID, &kind, &subject, &fromAddr,
		&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain, &attachmentIDsRaw)
	if err != nil {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	var to, cc, bcc []string
	if err := json.Unmarshal(toAddr, &to); err != nil {
		slog.Error("draft: failed to unmarshal to addresses", "draft_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "draft data is corrupted")
		return
	}
	if err := json.Unmarshal(ccAddr, &cc); err != nil {
		slog.Error("draft: failed to unmarshal cc addresses", "draft_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "draft data is corrupted")
		return
	}
	if err := json.Unmarshal(bccAddr, &bcc); err != nil {
		slog.Error("draft: failed to unmarshal bcc addresses", "draft_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "draft data is corrupted")
		return
	}

	if fromAddr == "" || len(to) == 0 || subject == "" {
		writeError(w, http.StatusBadRequest, "from, to, and subject are required to send")
		return
	}

	// Verify sender is authorized to send from this address
	if !canSendAs(ctx, h.DB, claims.UserID, claims.OrgID, fromAddr, claims.Role) {
		writeError(w, http.StatusForbidden, "you are not authorized to send from this address")
		return
	}

	// Resolve display name for From field
	fromDisplay := resolveFromDisplay(ctx, h.DB, claims.OrgID, fromAddr)

	// Build Resend payload (serialized to JSON for job storage)
	resendPayload := map[string]interface{}{
		"from":    fromDisplay,
		"to":      to,
		"subject": subject,
	}
	if bodyHTML != "" {
		resendPayload["html"] = bodyHTML
	}
	if bodyPlain != "" {
		resendPayload["text"] = bodyPlain
	}
	if len(cc) > 0 {
		resendPayload["cc"] = cc
	}
	if len(bcc) > 0 {
		resendPayload["bcc"] = bcc
	}

	// Attach files from draft's attachment_ids
	var attachmentIDs []string
	warnIfErr(json.Unmarshal(attachmentIDsRaw, &attachmentIDs), "draft: failed to unmarshal attachment IDs", "draft_id", id)
	if len(attachmentIDs) > 0 {
		attachments, attErr := loadAttachmentsForResend(ctx, h.DB, attachmentIDs, claims.OrgID)
		if attErr == nil && len(attachments) > 0 {
			resendPayload["attachments"] = attachments
		}
	}

	resendPayloadJSON, ok := marshalOrFail(w, resendPayload, "failed to prepare email")
	if !ok {
		return
	}

	// Build snippet
	snippet := util.TruncateRunes(bodyPlain, 200)

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
	var finalThreadID string
	if threadID != nil && *threadID != "" {
		finalThreadID = *threadID
	} else {
		participants, ok := marshalOrFail(w, append([]string{fromAddr}, to...), "failed to create thread")
		if !ok {
			return
		}
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, snippet)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			claims.OrgID, claims.UserID, domainID, subject, participants, snippet,
		).Scan(&finalThreadID); err != nil {
			slog.Error("draft: create thread failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create thread")
			return
		}
		addLabel(dbCtx, tx, finalThreadID, claims.OrgID, "sent")
	}

	// Store email with status='queued'
	toJSON, ok := marshalOrFail(w, to, "failed to create email")
	if !ok {
		return
	}
	ccJSON, ok := marshalOrFail(w, cc, "failed to create email")
	if !ok {
		return
	}
	bccJSON, ok := marshalOrFail(w, bcc, "failed to create email")
	if !ok {
		return
	}

	var emailID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain, status)
		 VALUES ($1, $2, $3, $4, 'outbound', $5, $6, $7, $8, $9, $10, $11, 'queued')
		 RETURNING id`,
		finalThreadID, claims.UserID, claims.OrgID, domainID,
		fromAddr, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain,
	).Scan(&emailID); err != nil {
		slog.Error("draft: insert email failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create email")
		return
	}

	// Update thread stats
	if _, err := tx.Exec(dbCtx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, updated_at = now()
		 WHERE id = $1`, finalThreadID, snippet,
	); err != nil {
		slog.Error("draft: update thread failed", "thread_id", finalThreadID, "error", err)
	}

	// Create email job (draft NOT deleted here — worker deletes after successful send)
	var jobID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO email_jobs (org_id, user_id, domain_id, job_type, email_id, thread_id, resend_payload, draft_id)
		 VALUES ($1, $2, $3, 'send', $4, $5, $6, $7)
		 RETURNING id`,
		claims.OrgID, claims.UserID, domainID, emailID, finalThreadID, resendPayloadJSON, id,
	).Scan(&jobID); err != nil {
		slog.Error("draft: create send job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to queue email")
		return
	}

	if err := tx.Commit(dbCtx); err != nil {
		slog.Error("draft: commit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send email")
		return
	}

	// Push to Redis queue (outside transaction — best-effort)
	if err := h.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
		slog.Error("draft: redis lpush failed", "job_id", jobID, "error", err)
	}

	slog.Info("draft: queued send", "email_id", emailID, "thread_id", finalThreadID, "job_id", jobID, "draft_id", id)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"email_id":  emailID,
		"thread_id": finalThreadID,
		"job_id":    jobID,
		"status":    "queued",
	})
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
