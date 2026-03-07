package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
		req.ThreadID, req.Kind, req.Subject, req.FromAddress, toJSON, ccJSON, bccJSON, attJSON)
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
	domainID, threadID, kind, subject, fromAddr, bodyHTML, bodyPlain, toAddr, ccAddr, bccAddr, attachmentIDsRaw, err := h.Store.GetDraft(ctx, id, claims.UserID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}
	_ = kind

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

		// Store email with status='queued'
		var err error
		emailID, err = tx.InsertEmail(ctx, finalThreadID, claims.UserID, claims.OrgID, domainID,
			"outbound", fromAddr, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain, "queued", "", nil)
		if err != nil {
			slog.Error("draft: insert email failed", "error", err)
			return err
		}

		// Update thread stats
		if err := tx.UpdateThreadStats(ctx, finalThreadID, snippet, fromAddr); err != nil {
			slog.Error("draft: update thread failed", "thread_id", finalThreadID, "error", err)
		}

		// Create email job (draft NOT deleted here — worker deletes after successful send)
		draftID := id
		jobID, err = tx.CreateEmailJob(ctx, claims.OrgID, claims.UserID, domainID, "send", emailID, finalThreadID, resendPayloadJSON, &draftID)
		if err != nil {
			slog.Error("draft: create send job failed", "error", err)
			return err
		}

		return nil
	})
	if txErr != nil {
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
