package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/redis/go-redis/v9"
)

type EmailHandler struct {
	Store     store.Store
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

	// Recipient count limit
	totalRecipients := len(req.To) + len(req.CC) + len(req.BCC)
	if totalRecipients > 50 {
		writeError(w, http.StatusBadRequest, "too many recipients (max 50)")
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

	// If replying into an existing thread, verify it belongs to the caller's
	// org and that the caller's aliases can see it. Without the visibility
	// check a member could inject messages into threads they cannot read.
	if req.ReplyTo != "" {
		if _, err := h.Store.GetThreadDomainID(ctx, req.ReplyTo, claims.OrgID); err != nil {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
		if claims.Role != "admin" {
			aliasAddrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
			aliasLabels := make([]string, len(aliasAddrs))
			for i, addr := range aliasAddrs {
				aliasLabels[i] = "alias:" + addr
			}
			visible, _ := h.Store.CheckThreadVisibility(ctx, req.ReplyTo, aliasLabels)
			if !visible {
				writeError(w, http.StatusNotFound, "thread not found")
				return
			}
		}
	}

	// Bounce block check: reject if any recipient is on the bounce list
	var allRecipients []string
	allRecipients = append(allRecipients, req.To...)
	allRecipients = append(allRecipients, req.CC...)
	allRecipients = append(allRecipients, req.BCC...)
	blocked, _ := h.Store.CheckBouncedRecipients(ctx, claims.OrgID, allRecipients)
	if len(blocked) > 0 {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Cannot send: the following addresses have bounced: %s", strings.Join(blocked, ", ")))
		return
	}

	// Verify sender is authorized to send from this address
	canSend, _ := h.Store.CanSendAs(ctx, claims.UserID, claims.OrgID, req.From, claims.Role)
	if !canSend {
		writeError(w, http.StatusForbidden, "you are not authorized to send from this address")
		return
	}

	// Check domain status before proceeding
	if req.DomainID != "" {
		if domainStatus, err := h.Store.GetDomainStatus(ctx, req.DomainID, claims.OrgID); err == nil {
			if domainStatus == "disconnected" || domainStatus == "pending" || domainStatus == "deleted" {
				writeError(w, http.StatusBadRequest, "cannot send email: domain is "+domainStatus)
				return
			}
		}
	}

	slog.Info("email: sending", "from", req.From, "to", req.To, "subject", req.Subject, "domain_id", req.DomainID)

	// Resolve display name for From field
	fromDisplay, _ := h.Store.ResolveFromDisplay(ctx, claims.OrgID, req.From)

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
		attachments, err := h.Store.LoadAttachmentsForResend(ctx, req.AttachmentIDs, claims.OrgID)
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
			if id, err := h.Store.LookupDomainByName(ctx, claims.OrgID, parts[1]); err == nil {
				domainID = id
			} else {
				slog.Warn("email: domain lookup failed", "domain", parts[1], "error", err)
			}
		}
	}

	// Build snippet from outbound text
	snippet := util.TruncateRunes(req.Text, 200)

	// Marshal JSON fields before the transaction
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

	// Wrap thread find/create + email INSERT + thread stats + job INSERT in a transaction
	var threadID, emailID, jobID string
	txErr := h.Store.WithTx(ctx, func(tx store.Store) error {
		// Find or create thread
		if req.ReplyTo != "" {
			threadID = req.ReplyTo
		} else {
			participants, ok := marshalOrFail(w, append([]string{req.From}, req.To...), "failed to create email")
			if !ok {
				return fmt.Errorf("failed to marshal participants")
			}
			var err error
			threadID, err = tx.CreateThread(ctx, claims.OrgID, claims.UserID, domainID, req.Subject, participants, snippet, req.From)
			if err != nil {
				slog.Error("email: create thread failed", "error", err)
				return fmt.Errorf("failed to create thread")
			}
			if err := tx.AddLabel(ctx, threadID, claims.OrgID, "sent"); err != nil {
				slog.Error("email: add sent label failed", "error", err)
			}
		}

		// Store email with status='queued'
		var err error
		emailID, err = tx.InsertEmail(ctx, threadID, claims.UserID, claims.OrgID, domainID, "outbound",
			req.From, toJSON, ccJSON, bccJSON, req.Subject, req.HTML, req.Text, "queued",
			req.InReplyTo, refsJSON)
		if err != nil {
			slog.Error("email: insert email failed", "error", err)
			return fmt.Errorf("failed to create email")
		}

		// Update thread stats
		if err := tx.UpdateThreadStats(ctx, threadID, claims.OrgID, snippet, req.From); err != nil {
			slog.Error("email: update thread failed", "thread_id", threadID, "error", err)
		}

		// Create email job
		jobID, err = tx.CreateEmailJob(ctx, claims.OrgID, claims.UserID, domainID, "send", emailID, threadID, resendPayloadJSON, nil)
		if err != nil {
			slog.Error("email: create send job failed", "error", err)
			return fmt.Errorf("failed to queue email")
		}

		return nil
	})
	if txErr != nil {
		writeError(w, http.StatusInternalServerError, txErr.Error())
		return
	}

	// Push to Redis queue (outside transaction — best-effort; stale recovery cron handles failures)
	undoSeconds, _ := h.Store.GetUndoSendSeconds(ctx, claims.UserID)
	enqueueSend(ctx, h.RDB, jobID, undoSeconds)

	slog.Info("email: queued", "email_id", emailID, "thread_id", threadID, "job_id", jobID, "undo_seconds", undoSeconds)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"email_id":     emailID,
		"thread_id":    threadID,
		"job_id":       jobID,
		"status":       "queued",
		"undo_seconds": undoSeconds,
	})
}

func (h *EmailHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	q := r.URL.Query().Get("q")
	domainID := r.URL.Query().Get("domain_id")
	label := r.URL.Query().Get("label")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	const searchLimit = 50

	ctx := r.Context()

	// Get alias addresses for non-admin visibility filter
	var aliasAddrs []string
	if claims.Role != "admin" {
		aliasAddrs, _ = h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
	}

	results, total, err := h.Store.SearchEmails(ctx, claims.OrgID, q, domainID, label, claims.Role, aliasAddrs, page, searchLimit)
	if err != nil {
		slog.Error("email search failed", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"threads": results,
		"total":   total,
		"page":    page,
		"limit":   searchLimit,
	})
}

func (h *EmailHandler) AdminJobs(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	jobs, err := h.Store.ListAdminJobs(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

// Retry re-queues a failed outbound email. It reuses the stored send job
// payload, so the email goes out exactly as composed.
func (h *EmailHandler) Retry(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	emailID := chi.URLParam(r, "id")
	ctx := r.Context()

	var status, direction, threadID, domainID, emailUserID string
	err := h.Store.Q().QueryRow(ctx,
		`SELECT status, direction, thread_id, domain_id, user_id FROM emails WHERE id = $1 AND org_id = $2`,
		emailID, claims.OrgID,
	).Scan(&status, &direction, &threadID, &domainID, &emailUserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	if direction != "outbound" {
		writeError(w, http.StatusBadRequest, "only outbound emails can be retried")
		return
	}
	// Only the original sender (or an admin) can re-send the stored payload.
	if emailUserID != claims.UserID && claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "only the sender can retry this email")
		return
	}
	if status != "failed" && status != "bounced" && status != "queued" {
		writeError(w, http.StatusConflict, "email is not in a failed state")
		return
	}

	// Find the most recent send job for this email and reset it.
	var jobID string
	err = h.Store.Q().QueryRow(ctx,
		`SELECT id FROM email_jobs WHERE email_id = $1 AND job_type = 'send'
		 ORDER BY created_at DESC LIMIT 1`,
		emailID,
	).Scan(&jobID)
	if err != nil {
		writeError(w, http.StatusConflict, "no send job found for this email — edit the recovery draft instead")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE email_jobs SET status = 'pending', retry_count = 0, error_message = '',
		        heartbeat_at = now(), updated_at = now() WHERE id = $1`,
		jobID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset send job")
		return
	}
	// A retry un-dismisses the email: the user acts on it again.
	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE emails SET status = 'queued', dismissed_at = NULL, updated_at = now() WHERE id = $1`,
		emailID,
	); err != nil {
		slog.Error("email retry: failed to reset email status", "email_id", emailID, "error", err)
	}
	if err := h.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue send job")
		return
	}

	if h.Bus != nil {
		h.Bus.Publish(ctx, event.Event{
			EventType: event.EmailStatusUpdated,
			OrgID:     claims.OrgID,
			DomainID:  domainID,
			ThreadID:  threadID,
			Payload: map[string]interface{}{
				"email_id": emailID,
				"status":   "queued",
			},
		})
	}

	slog.Info("email retry: re-queued", "email_id", emailID, "job_id", jobID, "user", claims.UserID)
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"email_id": emailID,
		"job_id":   jobID,
		"status":   "queued",
	})
}

// Dismiss marks a failed or bounced outbound email as handled. The email
// stays in the database, so sync cannot resurrect it, but the Failed view
// hides it. A later retry clears the flag.
func (h *EmailHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	emailID := chi.URLParam(r, "id")
	ctx := r.Context()

	var status, direction, threadID, domainID, emailUserID string
	err := h.Store.Q().QueryRow(ctx,
		`SELECT status, direction, thread_id, domain_id, user_id FROM emails WHERE id = $1 AND org_id = $2`,
		emailID, claims.OrgID,
	).Scan(&status, &direction, &threadID, &domainID, &emailUserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	if direction != "outbound" {
		writeError(w, http.StatusBadRequest, "only outbound emails can be dismissed")
		return
	}
	if emailUserID != claims.UserID && claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "only the sender can dismiss this email")
		return
	}
	if status != "failed" && status != "bounced" {
		writeError(w, http.StatusConflict, "email is not in a failed state")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE emails SET dismissed_at = now(), updated_at = now() WHERE id = $1`,
		emailID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to dismiss email")
		return
	}

	if h.Bus != nil {
		h.Bus.Publish(ctx, event.Event{
			EventType: event.EmailStatusUpdated,
			OrgID:     claims.OrgID,
			DomainID:  domainID,
			ThreadID:  threadID,
			Payload: map[string]interface{}{
				"email_id":  emailID,
				"status":    status,
				"dismissed": true,
			},
		})
	}

	slog.Info("email dismiss: dismissed", "email_id", emailID, "user", claims.UserID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email_id":  emailID,
		"dismissed": true,
	})
}

// CancelSend undoes a send that still sits in its undo window. It cancels
// the pending job, removes the queued email, and hands back a draft so the
// user can edit and resend.
func (h *EmailHandler) CancelSend(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	emailID := chi.URLParam(r, "id")
	ctx := r.Context()

	// The email must belong to the caller: undo restores a private draft.
	var threadID, domainID string
	var emailUserID string
	err := h.Store.Q().QueryRow(ctx,
		`SELECT user_id, thread_id, domain_id FROM emails
		 WHERE id = $1 AND org_id = $2 AND direction = 'outbound' AND status IN ('queued', 'scheduled')`,
		emailID, claims.OrgID,
	).Scan(&emailUserID, &threadID, &domainID)
	if err != nil {
		writeError(w, http.StatusConflict, "too late to undo — the email already left")
		return
	}
	if emailUserID != claims.UserID {
		writeError(w, http.StatusForbidden, "only the sender can undo a send")
		return
	}

	var jobID string
	var draftID *string
	err = h.Store.Q().QueryRow(ctx,
		`SELECT id, draft_id FROM email_jobs WHERE email_id = $1 AND job_type = 'send'
		 ORDER BY created_at DESC LIMIT 1`,
		emailID,
	).Scan(&jobID, &draftID)
	if err != nil {
		writeError(w, http.StatusConflict, "too late to undo — the email already left")
		return
	}

	// The conditional update is the undo gate: it wins only while the job
	// is still pending. A running or completed job means the send started.
	res, err := h.Store.Q().Exec(ctx,
		`UPDATE email_jobs SET status = 'cancelled', error_message = 'cancelled by undo', updated_at = now()
		 WHERE id = $1 AND status = 'pending'`,
		jobID,
	)
	if err != nil || res.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "too late to undo — the email already left")
		return
	}

	// Best-effort: pull the job out of the delay set before the dispatcher
	// moves it. The cancelled status guards the race either way.
	h.RDB.ZRem(ctx, queue.DelayedJobsSet, jobID)

	// A draft-based send still has its draft (the worker deletes it only
	// after a successful send). A direct send needs a recovery draft.
	restoredDraftID := ""
	if draftID != nil && *draftID != "" {
		restoredDraftID = *draftID
	} else {
		if id := createDraftFromEmail(ctx, h.Store, emailID); id != "" {
			restoredDraftID = id
		}
	}

	// Remove the queued email row; drop the thread too when this was its
	// only email (a fresh compose creates a thread per send). Clear the
	// cancelled job's references first so the deletes pass the FKs.
	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE email_jobs SET email_id = NULL, thread_id = NULL WHERE email_id = $1`, emailID); err != nil {
		slog.Error("email undo: clear job refs failed", "email_id", emailID, "error", err)
	}
	if _, err := h.Store.Q().Exec(ctx, `DELETE FROM emails WHERE id = $1 AND org_id = $2`, emailID, claims.OrgID); err != nil {
		slog.Error("email undo: delete email failed", "email_id", emailID, "error", err)
	}
	var remaining int
	if err := h.Store.Q().QueryRow(ctx, `SELECT count(*) FROM emails WHERE thread_id = $1`, threadID).Scan(&remaining); err == nil && remaining == 0 {
		if _, err := h.Store.Q().Exec(ctx, `DELETE FROM thread_labels WHERE thread_id = $1`, threadID); err != nil {
			slog.Error("email undo: delete thread labels failed", "thread_id", threadID, "error", err)
		}
		if _, err := h.Store.Q().Exec(ctx, `DELETE FROM threads WHERE id = $1 AND org_id = $2`, threadID, claims.OrgID); err != nil {
			slog.Error("email undo: delete empty thread failed", "thread_id", threadID, "error", err)
		}
	}

	if h.Bus != nil {
		h.Bus.Publish(ctx, event.Event{
			EventType: event.EmailStatusUpdated,
			OrgID:     claims.OrgID,
			DomainID:  domainID,
			ThreadID:  threadID,
			Payload: map[string]interface{}{
				"email_id": emailID,
				"status":   "cancelled",
			},
		})
	}

	slog.Info("email undo: send cancelled", "email_id", emailID, "job_id", jobID, "draft_id", restoredDraftID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "cancelled",
		"draft_id": restoredDraftID,
	})
}

// createDraftFromEmail rebuilds a draft from a queued email (direct sends
// have no draft to fall back to). Returns "" on failure.
func createDraftFromEmail(ctx context.Context, st store.Store, emailID string) string {
	var orgID, userID, domainID, fromAddr, subject, bodyHTML, bodyPlain string
	var threadID, inReplyTo *string
	var toJSON, ccJSON, bccJSON, attachmentIDs json.RawMessage

	err := st.Q().QueryRow(ctx,
		`SELECT org_id, user_id, domain_id, from_address, to_addresses, cc_addresses, bcc_addresses,
		 subject, body_html, body_plain, thread_id, in_reply_to, COALESCE(attachment_ids, '[]')
		 FROM emails WHERE id = $1`,
		emailID,
	).Scan(&orgID, &userID, &domainID, &fromAddr, &toJSON, &ccJSON, &bccJSON,
		&subject, &bodyHTML, &bodyPlain, &threadID, &inReplyTo, &attachmentIDs)
	if err != nil {
		slog.Error("email undo: draft rebuild query failed", "email_id", emailID, "error", err)
		return ""
	}

	kind := "compose"
	if inReplyTo != nil && *inReplyTo != "" {
		kind = "reply"
	}

	var draftID string
	err = st.Q().QueryRow(ctx,
		`INSERT INTO drafts (org_id, user_id, domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain, attachment_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id`,
		orgID, userID, domainID, threadID, kind, subject, fromAddr,
		toJSON, ccJSON, bccJSON, bodyHTML, bodyPlain, attachmentIDs,
	).Scan(&draftID)
	if err != nil {
		slog.Error("email undo: draft rebuild insert failed", "email_id", emailID, "error", err)
		return ""
	}
	return draftID
}
