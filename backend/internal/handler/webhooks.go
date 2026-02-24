package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookHandler struct {
	DB        *pgxpool.Pool
	Bus       *event.Bus
	ResendSvc *service.ResendService
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

	// Verify Svix signature with org-specific webhook secret
	var webhookSecret string
	if err := h.DB.QueryRow(r.Context(), "SELECT resend_webhook_secret FROM orgs WHERE id = $1", orgID).Scan(&webhookSecret); err == nil && webhookSecret != "" {
		if err := verifySvixSignature(body, r.Header, webhookSecret); err != nil {
			slog.Warn("webhook: signature verification failed", "org_id", orgID, "error", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

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
	// If no match found in To, check CC addresses
	if domainID == "" {
		for _, cc := range emailData.CC {
			parts := strings.Split(cc, "@")
			if len(parts) != 2 {
				continue
			}
			err := h.DB.QueryRow(ctx,
				"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
				orgID, parts[1],
			).Scan(&domainID)
			if err == nil {
				recipientAddress = cc
				break
			}
		}
	}
	if domainID == "" {
		slog.Warn("webhook: no matching domain for received email", "to", emailData.To, "cc", emailData.CC)
		return
	}

	// Idempotency: skip if we already processed this email
	var existingID string
	if err := h.DB.QueryRow(ctx,
		"SELECT id FROM emails WHERE resend_email_id = $1", emailData.EmailID,
	).Scan(&existingID); err == nil {
		slog.Info("webhook: duplicate email, skipping", "resend_email_id", emailData.EmailID)
		return
	}

	// Fetch full email (body, headers, attachments) from Resend API —
	// the webhook payload only contains metadata, not the actual content.
	type resendFullEmail struct {
		HTML        string            `json:"html"`
		Text        string            `json:"text"`
		Headers     map[string]string `json:"headers"`
		ReplyTo     []string          `json:"reply_to"`
		Attachments []struct {
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
			URL         string `json:"url"`
		} `json:"attachments"`
	}
	var full resendFullEmail
	bodyData, err := h.ResendSvc.Fetch(ctx, orgID, "GET", "/emails/receiving/"+emailData.EmailID, nil)
	if err != nil {
		slog.Error("webhook: fetch full email from Resend", "error", err, "email_id", emailData.EmailID)
		// Fall back to whatever the webhook payload contained (likely empty)
	} else {
		if err := json.Unmarshal(bodyData, &full); err != nil {
			slog.Error("webhook: parse full email response", "error", err)
		}
	}

	// Merge fetched data — prefer API response over webhook payload
	bodyHTML := full.HTML
	bodyPlain := full.Text
	headers := full.Headers
	if headers == nil {
		headers = emailData.Headers
	}
	replyTo := full.ReplyTo
	if replyTo == nil {
		replyTo = emailData.ReplyTo
	}

	// 3-phase routing: direct user -> alias -> catch-all
	userID := h.routeEmail(ctx, orgID, domainID, recipientAddress)
	if userID == "" {
		slog.Warn("webhook: no user found for", "address", recipientAddress)
		return
	}

	// Spam classification
	spamResult := service.ClassifySpam(headers, emailData.From, emailData.Subject, bodyPlain)
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

	snippet := truncateRunes(bodyPlain, 200)

	// Step 1: Match by In-Reply-To header
	inReplyToHeader := ""
	if headers != nil {
		inReplyToHeader = headers["In-Reply-To"]
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
		if err := h.DB.QueryRow(ctx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, folder, original_to, snippet)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			orgID, userID, domainID, cleanSubject, participants, folder, recipientAddress, snippet,
		).Scan(&threadID); err != nil {
			slog.Error("webhook: create thread failed", "org_id", orgID, "from", emailData.From, "error", err)
			return
		}
	}

	// Insert email with all available fields
	toJSON, _ := json.Marshal(emailData.To)
	ccJSON, _ := json.Marshal(emailData.CC)
	bccJSON, _ := json.Marshal(emailData.BCC)
	replyToJSON, _ := json.Marshal(replyTo)
	headersJSON, _ := json.Marshal(headers)
	spamReasons, _ := json.Marshal(spamResult.Reasons)

	// Build attachments JSON from API response
	attachmentsJSON := []byte("[]")
	if len(full.Attachments) > 0 {
		attachmentsJSON, _ = json.Marshal(full.Attachments)
	}

	// Extract threading headers
	var inReplyTo string
	var refsJSON []byte
	if headers != nil {
		inReplyTo = headers["In-Reply-To"]
		if refs, ok := headers["References"]; ok {
			refsJSON, _ = json.Marshal(strings.Fields(refs))
		}
	}
	if refsJSON == nil {
		refsJSON = []byte("[]")
	}

	var emailID string
	if err := h.DB.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id,
		 direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
		 subject, body_html, body_plain, status, headers, in_reply_to, references_header,
		 spam_score, spam_reasons, attachments)
		 VALUES ($1, $2, $3, $4, $5, $6, 'inbound', $7, $8, $9, $10, $11, $12, $13, $14, 'received',
		 $15, $16, $17, $18, $19, $20)
		 RETURNING id`,
		threadID, userID, orgID, domainID, emailData.EmailID, emailData.MessageID,
		emailData.From, toJSON, ccJSON, bccJSON, replyToJSON,
		emailData.Subject, bodyHTML, bodyPlain,
		headersJSON, inReplyTo, refsJSON, spamResult.Score, spamReasons, attachmentsJSON,
	).Scan(&emailID); err != nil {
		slog.Error("webhook: insert email failed", "thread_id", threadID, "resend_email_id", emailData.EmailID, "error", err)
		return
	}

	// Update thread stats and snippet
	if _, err := h.DB.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, unread_count = unread_count + 1,
		 last_message_at = now(), snippet = $2, updated_at = now() WHERE id = $1`, threadID, snippet,
	); err != nil {
		slog.Error("webhook: update thread failed", "thread_id", threadID, "error", err)
	}

	slog.Info("webhook: email processed", "email_id", emailID, "thread_id", threadID, "folder", folder, "from", emailData.From, "resend_email_id", emailData.EmailID)

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
		slog.Error("webhook: parse status event", "status", status, "error", err)
		return
	}

	var threadID, domainID string
	if err := h.DB.QueryRow(ctx,
		"SELECT thread_id, domain_id FROM emails WHERE resend_email_id = $1",
		statusData.EmailID,
	).Scan(&threadID, &domainID); err != nil {
		slog.Warn("webhook: email not found for status update", "resend_email_id", statusData.EmailID, "status", status)
	}

	if _, err := h.DB.Exec(ctx,
		"UPDATE emails SET status = $1, updated_at = now() WHERE resend_email_id = $2",
		status, statusData.EmailID,
	); err != nil {
		slog.Error("webhook: update email status failed", "resend_email_id", statusData.EmailID, "status", status, "error", err)
	}

	slog.Info("webhook: status update", "resend_email_id", statusData.EmailID, "status", status)

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

// verifySvixSignature verifies a Svix-signed webhook payload.
// secret is the webhook signing secret (with or without "whsec_" prefix).
func verifySvixSignature(payload []byte, headers http.Header, secret string) error {
	msgID := headers.Get("svix-id")
	timestamp := headers.Get("svix-timestamp")
	signature := headers.Get("svix-signature")

	if msgID == "" || timestamp == "" || signature == "" {
		return fmt.Errorf("missing svix headers")
	}

	// Validate timestamp is within 5 minutes
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	diff := math.Abs(float64(time.Now().Unix() - ts))
	if diff > 300 {
		return fmt.Errorf("timestamp too old or too new")
	}

	// Decode secret key (strip "whsec_" prefix if present)
	keyStr := strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return fmt.Errorf("invalid secret key")
	}

	// Compute expected signature: HMAC-SHA256(msgID.timestamp.body)
	signedContent := fmt.Sprintf("%s.%s.%s", msgID, timestamp, string(payload))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signedContent))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Compare against all provided signatures (comma-separated, each prefixed with "v1,")
	for _, sig := range strings.Split(signature, " ") {
		parts := strings.SplitN(sig, ",", 2)
		if len(parts) != 2 {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(parts[1])) {
			return nil
		}
	}

	return fmt.Errorf("no matching signature found")
}

func truncateRunes(s string, maxRunes int) string {
	s = html.UnescapeString(s)
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
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
