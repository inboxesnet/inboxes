package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookHandler struct {
	DB  *pgxpool.Pool
	Bus *event.Bus
}

type webhookPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type emailReceivedData struct {
	EmailID   string            `json:"email_id"`
	From      string            `json:"from"`
	To        []string          `json:"to"`
	CC        []string          `json:"cc"`
	BCC       []string          `json:"bcc"`
	ReplyTo   []string          `json:"reply_to"`
	Subject   string            `json:"subject"`
	HTML      string            `json:"html"`
	Text      string            `json:"text"`
	MessageID string            `json:"message_id"`
	Headers   map[string]string `json:"headers"`
	CreatedAt string            `json:"created_at"`
}

type emailStatusData struct {
	EmailID   string `json:"email_id"`
	CreatedAt string `json:"created_at"`
}

func (h *WebhookHandler) HandleResend(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	if orgID == "" {
		http.Error(w, "missing org id", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// TODO: Verify Svix signature with org-specific webhook secret

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	switch payload.Type {
	case "email.received":
		h.handleEmailReceived(ctx, orgID, payload.Data)
	case "email.sent":
		h.handleEmailStatus(ctx, orgID, "sent", payload.Data)
	case "email.delivered":
		h.handleEmailStatus(ctx, orgID, "delivered", payload.Data)
	case "email.bounced":
		h.handleEmailStatus(ctx, orgID, "bounced", payload.Data)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) handleEmailReceived(ctx context.Context, orgID string, data json.RawMessage) {
	var emailData emailReceivedData
	if err := json.Unmarshal(data, &emailData); err != nil {
		slog.Error("webhook: parse received email", "error", err)
		return
	}

	// Determine which domain this email is for (based on TO addresses)
	var domainID string
	var recipientAddress string
	for _, to := range emailData.To {
		parts := strings.Split(to, "@")
		if len(parts) != 2 {
			continue
		}
		err := h.DB.QueryRow(ctx,
			"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
			orgID, parts[1],
		).Scan(&domainID)
		if err == nil {
			recipientAddress = to
			break
		}
	}
	if domainID == "" {
		slog.Warn("webhook: no matching domain for received email", "to", emailData.To)
		return
	}

	// 3-phase routing: direct user -> alias -> catch-all
	userID := h.routeEmail(ctx, orgID, domainID, recipientAddress)
	if userID == "" {
		slog.Warn("webhook: no user found for", "address", recipientAddress)
		return
	}

	// Spam classification
	spamResult := service.ClassifySpam(emailData.Headers, emailData.From, emailData.Subject, emailData.Text)
	folder := "inbox"
	if spamResult.IsSpam {
		folder = "spam"
		slog.Info("webhook: email classified as spam", "from", emailData.From, "score", spamResult.Score)
	}

	// Find or create thread using 3-step matching:
	// 1. In-Reply-To header match
	// 2. Subject + counterparty match
	// 3. Create new thread
	cleanSubject := cleanSubjectForThread(emailData.Subject)
	var threadID string
	found := false

	snippet := emailData.Text
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}

	// Step 1: Match by In-Reply-To header
	inReplyToHeader := ""
	if emailData.Headers != nil {
		inReplyToHeader = emailData.Headers["In-Reply-To"]
	}
	if inReplyToHeader != "" {
		err := h.DB.QueryRow(ctx,
			`SELECT thread_id FROM emails WHERE message_id = $1 AND org_id = $2 LIMIT 1`,
			inReplyToHeader, orgID,
		).Scan(&threadID)
		if err == nil {
			found = true
		}
	}

	// Step 2: Match by subject + counterparty (the from address for inbound)
	if !found {
		fromClean := service.ExtractEmail(emailData.From)
		counterpartyJSON, _ := json.Marshal([]string{fromClean})
		err := h.DB.QueryRow(ctx,
			`SELECT id FROM threads WHERE org_id = $1 AND domain_id = $2 AND subject = $3
			 AND participant_emails @> $4::jsonb
			 ORDER BY last_message_at DESC LIMIT 1`,
			orgID, domainID, cleanSubject, counterpartyJSON,
		).Scan(&threadID)
		if err == nil {
			found = true
		}
	}

	// Step 3: Create new thread
	if !found {
		participants, _ := json.Marshal(append([]string{emailData.From}, emailData.To...))
		h.DB.QueryRow(ctx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, folder, original_to, snippet)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			orgID, userID, domainID, cleanSubject, participants, folder, recipientAddress, snippet,
		).Scan(&threadID)
	}

	// Insert email with all available fields
	toJSON, _ := json.Marshal(emailData.To)
	ccJSON, _ := json.Marshal(emailData.CC)
	bccJSON, _ := json.Marshal(emailData.BCC)
	replyToJSON, _ := json.Marshal(emailData.ReplyTo)
	headersJSON, _ := json.Marshal(emailData.Headers)
	spamReasons, _ := json.Marshal(spamResult.Reasons)

	// Extract threading headers
	var inReplyTo string
	var refsJSON []byte
	if emailData.Headers != nil {
		inReplyTo = emailData.Headers["In-Reply-To"]
		if refs, ok := emailData.Headers["References"]; ok {
			refsJSON, _ = json.Marshal(strings.Fields(refs))
		}
	}
	if refsJSON == nil {
		refsJSON = []byte("[]")
	}

	var emailID string
	h.DB.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id,
		 direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
		 subject, body_html, body_plain, status, headers, in_reply_to, references_header,
		 spam_score, spam_reasons)
		 VALUES ($1, $2, $3, $4, $5, $6, 'inbound', $7, $8, $9, $10, $11, $12, $13, $14, 'received',
		 $15, $16, $17, $18, $19)
		 RETURNING id`,
		threadID, userID, orgID, domainID, emailData.EmailID, emailData.MessageID,
		emailData.From, toJSON, ccJSON, bccJSON, replyToJSON,
		emailData.Subject, emailData.HTML, emailData.Text,
		headersJSON, inReplyTo, refsJSON, spamResult.Score, spamReasons,
	).Scan(&emailID)

	// Update thread stats and snippet
	h.DB.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, unread_count = unread_count + 1,
		 last_message_at = now(), snippet = $2, updated_at = now() WHERE id = $1`, threadID, snippet,
	)

	// Publish event
	h.Bus.Publish(ctx, event.Event{
		EventType: event.EmailReceived,
		OrgID:     orgID,
		UserID:    userID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"email_id": emailID,
			"from":     emailData.From,
			"subject":  emailData.Subject,
			"folder":   folder,
		},
	})
}

func (h *WebhookHandler) handleEmailStatus(ctx context.Context, orgID, status string, data json.RawMessage) {
	var statusData emailStatusData
	if err := json.Unmarshal(data, &statusData); err != nil {
		return
	}

	var threadID, domainID string
	h.DB.QueryRow(ctx,
		"SELECT thread_id, domain_id FROM emails WHERE resend_email_id = $1",
		statusData.EmailID,
	).Scan(&threadID, &domainID)

	h.DB.Exec(ctx,
		"UPDATE emails SET status = $1, updated_at = now() WHERE resend_email_id = $2",
		status, statusData.EmailID,
	)

	h.Bus.Publish(ctx, event.Event{
		EventType: event.EmailStatusUpdated,
		OrgID:     orgID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"email_id": statusData.EmailID,
			"status":   status,
		},
	})
}

func (h *WebhookHandler) routeEmail(ctx context.Context, orgID, domainID, address string) string {
	// Phase 1: Direct user match
	var userID string
	err := h.DB.QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND email = $2 AND status = 'active'",
		orgID, address,
	).Scan(&userID)
	if err == nil {
		return userID
	}

	// Phase 2: Alias match
	var aliasID string
	err = h.DB.QueryRow(ctx,
		"SELECT id FROM aliases WHERE org_id = $1 AND address = $2",
		orgID, address,
	).Scan(&aliasID)
	if err == nil {
		// Get first user assigned to this alias
		err = h.DB.QueryRow(ctx,
			"SELECT user_id FROM alias_users WHERE alias_id = $1 LIMIT 1",
			aliasID,
		).Scan(&userID)
		if err == nil {
			return userID
		}
	}

	// Phase 3: Catch-all — assign to org admin
	err = h.DB.QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&userID)
	if err == nil {
		return userID
	}

	return ""
}

func cleanSubjectForThread(subject string) string {
	s := subject
	for {
		lower := strings.ToLower(s)
		if strings.HasPrefix(lower, "re: ") {
			s = s[4:]
		} else if strings.HasPrefix(lower, "fwd: ") {
			s = s[5:]
		} else {
			break
		}
	}
	// Collapse multiple spaces into one
	parts := strings.Fields(strings.TrimSpace(s))
	return strings.Join(parts, " ")
}
