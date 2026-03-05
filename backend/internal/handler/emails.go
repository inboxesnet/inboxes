package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
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
		if err := tx.UpdateThreadStats(ctx, threadID, snippet, req.From); err != nil {
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

	// Get alias addresses for non-admin visibility filter
	var aliasAddrs []string
	if claims.Role != "admin" {
		aliasAddrs, _ = h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
	}

	results, err := h.Store.SearchEmails(ctx, claims.OrgID, q, domainID, claims.Role, aliasAddrs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"threads": results})
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
