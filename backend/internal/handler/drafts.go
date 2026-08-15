package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/redis/go-redis/v9"
)

type DraftHandler struct {
	Store     store.Store
	ResendSvc *service.ResendService
	Bus       *event.Bus
	RDB       *redis.Client
}

func (h *DraftHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	domainID := r.URL.Query().Get("domain_id")

	drafts, err := h.Store.ListDrafts(r.Context(), claims.UserID, claims.OrgID, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list drafts")
		return
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
		InReplyTo     string   `json:"in_reply_to"`
		References    []string `json:"references"`
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

	id, err := h.Store.CreateDraft(r.Context(), claims.OrgID, claims.UserID, req.DomainID,
		req.ThreadID, req.Kind, req.Subject, req.FromAddress, toJSON, ccJSON, bccJSON, attJSON,
		req.InReplyTo, strings.Join(req.References, " "))
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
		ThreadID      *string  `json:"thread_id"`
		InReplyTo     *string  `json:"in_reply_to"`
		References    []string `json:"references"`
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
	if req.ThreadID != nil {
		setClauses = append(setClauses, "thread_id = $"+itoa(argIdx))
		args = append(args, *req.ThreadID)
		argIdx++
	}
	if req.InReplyTo != nil {
		setClauses = append(setClauses, "in_reply_to = $"+itoa(argIdx))
		args = append(args, *req.InReplyTo)
		argIdx++
	}
	if req.References != nil {
		setClauses = append(setClauses, "references_header = $"+itoa(argIdx))
		args = append(args, strings.Join(req.References, " "))
		argIdx++
	}

	n, err := h.Store.UpdateDraft(ctx, id, claims.UserID, setClauses, args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update draft")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DraftHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	id := chi.URLParam(r, "id")
	n, err := h.Store.DeleteDraft(r.Context(), id, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete draft")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DraftHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Optional schedule: {"scheduled_at": "2026-08-16T08:00:00Z"} parks the
	// send job until that time. An empty body sends now.
	var sendReq struct {
		ScheduledAt *time.Time `json:"scheduled_at"`
	}
	readJSON(r, &sendReq) // body is optional — ignore parse errors
	var scheduledAt *time.Time
	if sendReq.ScheduledAt != nil {
		t := sendReq.ScheduledAt.UTC()
		if t.Before(time.Now().Add(time.Minute)) {
			writeError(w, http.StatusBadRequest, "scheduled_at must be at least 1 minute in the future")
			return
		}
		if t.After(time.Now().Add(30 * 24 * time.Hour)) {
			writeError(w, http.StatusBadRequest, "scheduled_at must be within 30 days")
			return
		}
		scheduledAt = &t
	}

	// Idempotency: reject if a send job already exists for this draft
	alreadySending, err := h.Store.CheckSendJobExists(ctx, id)
	if err != nil {
		slog.Error("draft: idempotency check failed", "draft_id", id, "error", err)
	}
	if alreadySending {
		writeError(w, http.StatusConflict, "this draft is already being sent")
		return
	}

	// Fetch draft
	domainID, threadID, kind, subject, fromAddr, bodyHTML, bodyPlain, toAddr, ccAddr, bccAddr, attachmentIDsRaw, inReplyTo, referencesHeader, err := h.Store.GetDraft(ctx, id, claims.UserID, claims.OrgID)
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

	// Check the domain status up front, like the direct send path does.
	// Without this check the job fails later in the worker, near-silently.
	if domainID != "" {
		if domainStatus, err := h.Store.GetDomainStatus(ctx, domainID, claims.OrgID); err == nil {
			if domainStatus == "disconnected" || domainStatus == "pending" || domainStatus == "deleted" {
				writeError(w, http.StatusBadRequest, "cannot send email: domain is "+domainStatus)
				return
			}
		}
	}

	// If the draft targets an existing thread, verify it belongs to the caller's org.
	if threadID != nil && *threadID != "" {
		if _, err := h.Store.GetThreadDomainID(ctx, *threadID, claims.OrgID); err != nil {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
	}

	// Bounce block check: reject if any recipient is on the bounce list
	allRecipients := append(append(to, cc...), bcc...)
	blocked, _ := h.Store.CheckBouncedRecipients(ctx, claims.OrgID, allRecipients)
	if len(blocked) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Cannot send: the following addresses have bounced: %s", strings.Join(blocked, ", ")))
		return
	}

	// Verify sender is authorized to send from this address
	canSend, _ := h.Store.CanSendAs(ctx, claims.UserID, claims.OrgID, fromAddr, claims.Role)
	if !canSend {
		writeError(w, http.StatusForbidden, "you are not authorized to send from this address")
		return
	}

	// Resolve display name for From field
	fromDisplay, _ := h.Store.ResolveFromDisplay(ctx, claims.OrgID, fromAddr)

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
	// Threading headers, like the direct send path. Without them a reply
	// sent from a draft starts a new conversation at the recipient.
	if inReplyTo != "" {
		headers := map[string]string{"In-Reply-To": inReplyTo}
		if referencesHeader != "" {
			headers["References"] = referencesHeader
		}
		resendPayload["headers"] = headers
	}

	// Attach files from draft's attachment_ids
	var attachmentIDs []string
	warnIfErr(json.Unmarshal(attachmentIDsRaw, &attachmentIDs), "draft: failed to unmarshal attachment IDs", "draft_id", id)
	if len(attachmentIDs) > 0 {
		attachments, attErr := h.Store.LoadAttachmentsForResend(ctx, attachmentIDs, claims.OrgID)
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

	// Marshal JSON for transaction
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

	// Wrap thread find/create + email INSERT + thread stats + job INSERT in a transaction
	var finalThreadID, emailID, jobID string
	txErr := h.Store.WithTx(ctx, func(tx store.Store) error {
		// Find or create thread
		if threadID != nil && *threadID != "" {
			finalThreadID = *threadID
		} else {
			participants, err := json.Marshal(append([]string{fromAddr}, to...))
			if err != nil {
				return fmt.Errorf("failed to marshal participants: %w", err)
			}
			var err2 error
			finalThreadID, err2 = tx.CreateThread(ctx, claims.OrgID, claims.UserID, domainID, subject, participants, snippet, fromAddr)
			if err2 != nil {
				slog.Error("draft: create thread failed", "error", err2)
				return err2
			}
			if err := tx.AddLabel(ctx, finalThreadID, claims.OrgID, "sent"); err != nil {
				slog.Error("draft: add sent label failed", "error", err)
			}
		}

		// Store email with status='queued' (or 'scheduled'), keeping the
		// threading context
		var refsJSON []byte
		if referencesHeader != "" {
			refsJSON, _ = json.Marshal(strings.Fields(referencesHeader))
		}
		emailStatus := "queued"
		if scheduledAt != nil {
			emailStatus = "scheduled"
		}
		var err error
		emailID, err = tx.InsertEmail(ctx, finalThreadID, claims.UserID, claims.OrgID, domainID,
			"outbound", fromAddr, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain, emailStatus, inReplyTo, refsJSON)
		if err != nil {
			slog.Error("draft: insert email failed", "error", err)
			return err
		}

		// Update thread stats
		if err := tx.UpdateThreadStats(ctx, finalThreadID, claims.OrgID, snippet, fromAddr); err != nil {
			slog.Error("draft: update thread failed", "thread_id", finalThreadID, "error", err)
		}

		// Create email job (draft NOT deleted here — worker deletes after successful send)
		draftID := id
		jobID, err = tx.CreateEmailJob(ctx, claims.OrgID, claims.UserID, domainID, "send", emailID, finalThreadID, resendPayloadJSON, &draftID)
		if err != nil {
			slog.Error("draft: create send job failed", "error", err)
			return err
		}

		if scheduledAt != nil {
			if _, err := tx.Q().Exec(ctx,
				`UPDATE email_jobs SET run_after = $1 WHERE id = $2`, *scheduledAt, jobID); err != nil {
				return fmt.Errorf("set run_after: %w", err)
			}
			if _, err := tx.Q().Exec(ctx,
				`UPDATE drafts SET scheduled_send_at = $1 WHERE id = $2`, *scheduledAt, id); err != nil {
				slog.Error("draft: set scheduled_send_at failed", "draft_id", id, "error", err)
			}
		}

		return nil
	})
	if txErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to send email")
		return
	}

	if scheduledAt != nil {
		// The scheduler worker releases the job when run_after comes due.
		slog.Info("draft: scheduled send", "email_id", emailID, "job_id", jobID, "draft_id", id, "scheduled_at", *scheduledAt)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"email_id":     emailID,
			"thread_id":    finalThreadID,
			"job_id":       jobID,
			"status":       "scheduled",
			"scheduled_at": scheduledAt.Format(time.RFC3339),
		})
		return
	}

	// Push to Redis queue (outside transaction — best-effort). A non-zero
	// undo window parks the job in the delay set first.
	undoSeconds, _ := h.Store.GetUndoSendSeconds(ctx, claims.UserID)
	enqueueSend(ctx, h.RDB, jobID, undoSeconds)

	slog.Info("draft: queued send", "email_id", emailID, "thread_id", finalThreadID, "job_id", jobID, "draft_id", id, "kind", kind, "undo_seconds", undoSeconds)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"email_id":     emailID,
		"thread_id":    finalThreadID,
		"job_id":       jobID,
		"status":       "queued",
		"undo_seconds": undoSeconds,
	})
}

// CancelSchedule cancels a scheduled send and keeps the draft editable.
func (h *DraftHandler) CancelSchedule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	draftID := chi.URLParam(r, "id")
	ctx := r.Context()

	// The draft must belong to the caller.
	var draftOwner string
	if err := h.Store.Q().QueryRow(ctx,
		`SELECT user_id FROM drafts WHERE id = $1 AND org_id = $2`, draftID, claims.OrgID,
	).Scan(&draftOwner); err != nil || draftOwner != claims.UserID {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	var jobID string
	var emailID *string
	err := h.Store.Q().QueryRow(ctx,
		`SELECT id, email_id FROM email_jobs
		 WHERE draft_id = $1 AND job_type = 'send' AND status = 'pending' AND run_after IS NOT NULL
		 ORDER BY created_at DESC LIMIT 1`,
		draftID,
	).Scan(&jobID, &emailID)
	if err != nil {
		writeError(w, http.StatusConflict, "no scheduled send found for this draft")
		return
	}

	res, err := h.Store.Q().Exec(ctx,
		`UPDATE email_jobs SET status = 'cancelled', error_message = 'schedule cancelled', updated_at = now()
		 WHERE id = $1 AND status = 'pending'`, jobID)
	if err != nil || res.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "too late to cancel — the email already left")
		return
	}

	// Remove the parked email row; the draft keeps the content.
	if emailID != nil {
		var threadID string
		h.Store.Q().QueryRow(ctx, `SELECT thread_id FROM emails WHERE id = $1`, *emailID).Scan(&threadID)
		h.Store.Q().Exec(ctx, `UPDATE email_jobs SET email_id = NULL, thread_id = NULL WHERE email_id = $1`, *emailID)
		if _, err := h.Store.Q().Exec(ctx, `DELETE FROM emails WHERE id = $1 AND org_id = $2`, *emailID, claims.OrgID); err != nil {
			slog.Error("draft: cancel-schedule email delete failed", "email_id", *emailID, "error", err)
		}
		if threadID != "" {
			var remaining int
			if err := h.Store.Q().QueryRow(ctx, `SELECT count(*) FROM emails WHERE thread_id = $1`, threadID).Scan(&remaining); err == nil && remaining == 0 {
				h.Store.Q().Exec(ctx, `DELETE FROM thread_labels WHERE thread_id = $1`, threadID)
				h.Store.Q().Exec(ctx, `DELETE FROM threads WHERE id = $1 AND org_id = $2`, threadID, claims.OrgID)
			}
		}
	}

	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE drafts SET scheduled_send_at = NULL, updated_at = now() WHERE id = $1`, draftID); err != nil {
		slog.Error("draft: clear scheduled_send_at failed", "draft_id", draftID, "error", err)
	}

	slog.Info("draft: schedule cancelled", "draft_id", draftID, "job_id", jobID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
