package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type WebhookHandler struct {
	DB        *pgxpool.Pool
	Bus       *event.Bus
	ResendSvc *service.ResendService
	RDB       *redis.Client
	EncSvc    *service.EncryptionService
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

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Early guard: return 410 for deleted orgs to signal Resend to stop
	var orgDeletedAt *time.Time
	if err := h.DB.QueryRow(r.Context(),
		"SELECT deleted_at FROM orgs WHERE id = $1", orgID,
	).Scan(&orgDeletedAt); err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}
	if orgDeletedAt != nil {
		w.WriteHeader(http.StatusGone) // 410
		return
	}

	// Verify Svix signature with org-specific webhook secret
	// Prefer encrypted secret; fall back to plaintext for pre-migration orgs
	var webhookSecret string
	var encSecret, encIV, encTag *string
	var plainSecret *string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT resend_webhook_secret, resend_webhook_secret_encrypted, resend_webhook_secret_iv, resend_webhook_secret_tag FROM orgs WHERE id = $1`, orgID,
	).Scan(&plainSecret, &encSecret, &encIV, &encTag); err != nil {
		slog.Error("webhook: failed to look up webhook secret", "org_id", orgID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if encSecret != nil && *encSecret != "" && h.EncSvc != nil {
		decrypted, decErr := h.EncSvc.Decrypt(*encSecret, *encIV, *encTag)
		if decErr != nil {
			slog.Error("webhook: failed to decrypt webhook secret", "org_id", orgID, "error", decErr)
		} else {
			webhookSecret = decrypted
		}
	}
	if webhookSecret == "" && plainSecret != nil {
		webhookSecret = *plainSecret
	}
	if webhookSecret == "" {
		slog.Warn("webhook: no webhook secret configured, rejecting", "org_id", orgID)
		http.Error(w, "webhook secret not configured", http.StatusUnauthorized)
		return
	}
	if err := verifySvixSignature(body, r.Header, webhookSecret); err != nil {
		slog.Warn("webhook: signature verification failed", "org_id", orgID, "error", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
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
	case "email.complained":
		h.handleEmailStatus(ctx, orgID, "complained", payload.Data)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) handleEmailReceived(ctx context.Context, orgID string, data json.RawMessage) {
	var emailData emailReceivedData
	if err := json.Unmarshal(data, &emailData); err != nil {
		slog.Error("webhook: parse received email", "error", err)
		return
	}

	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	// Guard: skip processing for deleted orgs
	var orgDeletedAt *time.Time
	if err := h.DB.QueryRow(dbCtx,
		"SELECT deleted_at FROM orgs WHERE id = $1", orgID,
	).Scan(&orgDeletedAt); err != nil {
		slog.Error("webhook: org lookup failed", "org_id", orgID, "error", err)
		return
	}
	if orgDeletedAt != nil {
		slog.Info("webhook: org deleted, ignoring email", "org_id", orgID, "resend_email_id", emailData.EmailID)
		return
	}

	// Idempotency: skip if we already processed this email
	var existingID string
	if err := h.DB.QueryRow(dbCtx,
		"SELECT id FROM emails WHERE resend_email_id = $1", emailData.EmailID,
	).Scan(&existingID); err == nil {
		slog.Info("webhook: duplicate email, skipping", "resend_email_id", emailData.EmailID)
		return
	}

	// Find admin user for this org
	var adminUserID string
	if err := h.DB.QueryRow(dbCtx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&adminUserID); err != nil {
		slog.Error("webhook: no admin user found", "org_id", orgID, "error", err)
		return
	}

	webhookDataJSON, err := json.Marshal(emailData)
	if err != nil {
		slog.Error("webhook: failed to marshal email data", "error", err)
		return
	}

	// Atomic idempotent INSERT — partial unique index on (resend_email_id) WHERE status IN ('pending','running')
	// prevents duplicate jobs without a check-then-insert race condition
	var jobID string
	if err := h.DB.QueryRow(dbCtx,
		`INSERT INTO email_jobs (org_id, user_id, job_type, resend_email_id, webhook_data)
		 VALUES ($1, $2, 'fetch', $3, $4)
		 ON CONFLICT (resend_email_id) WHERE status IN ('pending', 'running') DO NOTHING
		 RETURNING id`,
		orgID, adminUserID, emailData.EmailID, webhookDataJSON,
	).Scan(&jobID); err != nil {
		// ON CONFLICT DO NOTHING returns no rows — this is a duplicate, not an error
		slog.Info("webhook: duplicate job, skipping", "resend_email_id", emailData.EmailID)
		return
	}

	// Push to Redis queue
	if err := h.RDB.LPush(ctx, "email:jobs", jobID).Err(); err != nil {
		slog.Error("webhook: redis lpush failed", "job_id", jobID, "error", err)
	}
	slog.Info("webhook: enqueued fetch job", "job_id", jobID, "resend_email_id", emailData.EmailID)
}

func (h *WebhookHandler) handleEmailStatus(ctx context.Context, orgID, status string, data json.RawMessage) {
	h.handleEmailStatusWithRetry(ctx, orgID, status, data, 0)
}

func (h *WebhookHandler) handleEmailStatusWithRetry(ctx context.Context, orgID, status string, data json.RawMessage, attempt int) {
	var statusData emailStatusData
	if err := json.Unmarshal(data, &statusData); err != nil {
		slog.Error("webhook: parse status event", "status", status, "error", err)
		return
	}

	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	// UPDATE first — check rows affected
	tag, err := h.DB.Exec(dbCtx,
		"UPDATE emails SET status = $1, updated_at = now() WHERE resend_email_id = $2",
		status, statusData.EmailID,
	)
	if err != nil {
		slog.Error("webhook: update email status failed", "resend_email_id", statusData.EmailID, "status", status, "error", err)
		return
	}

	if tag.RowsAffected() == 0 {
		// Email not yet inserted (race: status webhook arrived before fetch job completed)
		if attempt >= 3 {
			slog.Warn("webhook: email not found after retries, dropping status update",
				"resend_email_id", statusData.EmailID, "status", status)
			return
		}

		// Use Redis INCR as a retry counter with 5-min TTL
		retryKey := fmt.Sprintf("webhook:status:retry:%s", statusData.EmailID)
		count, err := h.RDB.Incr(ctx, retryKey).Result()
		if err != nil {
			slog.Error("webhook: redis retry counter failed", "error", err)
			return
		}
		if count == 1 {
			h.RDB.Expire(ctx, retryKey, 5*time.Minute)
		}
		if count > 3 {
			slog.Warn("webhook: max retries exceeded for status update",
				"resend_email_id", statusData.EmailID, "status", status)
			return
		}

		// Retry after 10s delay
		slog.Info("webhook: email not found, scheduling retry",
			"resend_email_id", statusData.EmailID, "status", status, "attempt", count)
		util.SafeGo("webhook-status-retry", func() {
			time.Sleep(10 * time.Second)
			// Fire-and-forget retry — use a bounded timeout since the original request ctx is done
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer retryCancel()
			h.handleEmailStatusWithRetry(retryCtx, orgID, status, data, int(count))
		})
		return
	}

	var threadID, domainID string
	warnIfErr(h.DB.QueryRow(dbCtx,
		"SELECT thread_id, domain_id FROM emails WHERE resend_email_id = $1",
		statusData.EmailID,
	).Scan(&threadID, &domainID), "webhook: failed to look up thread/domain for event", "resend_email_id", statusData.EmailID)

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

