package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmailHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Bus       *event.Bus
}

type sendRequest struct {
	DomainID   string   `json:"domain_id"`
	From       string   `json:"from"`
	To         []string `json:"to"`
	CC         []string `json:"cc"`
	BCC        []string `json:"bcc"`
	Subject    string   `json:"subject"`
	HTML       string   `json:"html"`
	Text       string   `json:"text"`
	ReplyTo    string   `json:"reply_to_thread_id"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
}

func (h *EmailHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req sendRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" || len(req.To) == 0 || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "from, to, and subject are required")
		return
	}

	ctx := r.Context()

	// Resolve display name for From field
	fromDisplay := resolveFromDisplay(ctx, h.DB, claims.OrgID, req.From)

	// Send via Resend
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
	// Add threading headers if replying to a specific email
	if req.InReplyTo != "" {
		headers := map[string]string{"In-Reply-To": req.InReplyTo}
		if len(req.References) > 0 {
			headers["References"] = strings.Join(req.References, " ")
		}
		resendPayload["headers"] = headers
	}

	data, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "POST", "/emails", resendPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send email: "+err.Error())
		return
	}

	var resendResp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &resendResp)

	// Determine domain_id
	domainID := req.DomainID
	if domainID == "" {
		parts := strings.Split(req.From, "@")
		if len(parts) == 2 {
			h.DB.QueryRow(ctx,
				"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
				claims.OrgID, parts[1],
			).Scan(&domainID)
		}
	}

	// Build snippet from outbound text
	snippet := truncateRunes(req.Text, 200)

	// Find or create thread
	var threadID string
	if req.ReplyTo != "" {
		threadID = req.ReplyTo
	} else {
		// Create new thread
		participants, _ := json.Marshal(append([]string{req.From}, req.To...))
		h.DB.QueryRow(ctx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, folder, snippet)
			 VALUES ($1, $2, $3, $4, $5, 'sent', $6) RETURNING id`,
			claims.OrgID, claims.UserID, domainID, req.Subject, participants, snippet,
		).Scan(&threadID)
	}

	// Store email
	toJSON, _ := json.Marshal(req.To)
	ccJSON, _ := json.Marshal(req.CC)
	bccJSON, _ := json.Marshal(req.BCC)
	refsJSON, _ := json.Marshal(req.References)

	var emailID string
	h.DB.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain,
		 status, in_reply_to, references_header)
		 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, $8, $9, $10, $11, $12, 'sent', $13, $14)
		 RETURNING id`,
		threadID, claims.UserID, claims.OrgID, domainID, resendResp.ID,
		req.From, toJSON, ccJSON, bccJSON, req.Subject, req.HTML, req.Text,
		req.InReplyTo, refsJSON,
	).Scan(&emailID)

	// Update thread stats and snippet
	h.DB.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, updated_at = now()
		 WHERE id = $1`, threadID, snippet,
	)

	h.Bus.Publish(ctx, event.Event{
		EventType: event.EmailSent,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"email_id": emailID,
		},
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"email_id":       emailID,
		"resend_id":      resendResp.ID,
		"thread_id":      threadID,
	})
}

func (h *EmailHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query().Get("q")
	domainID := r.URL.Query().Get("domain_id")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	ctx := r.Context()

	// Find matching thread IDs via email full-text search, then return thread-shaped results
	query := `SELECT DISTINCT ON (t.id)
		t.id, t.domain_id, t.subject, t.participant_emails,
		t.last_message_at, t.message_count, t.unread_count,
		t.starred, t.folder, t.snippet, t.original_to, t.created_at
		FROM threads t
		JOIN emails e ON e.thread_id = t.id
		WHERE e.org_id = $1 AND e.search_vector @@ plainto_tsquery('english', $2)
		AND t.deleted_at IS NULL`
	args := []interface{}{claims.OrgID, q}

	if domainID != "" {
		query += " AND e.domain_id = $3"
		args = append(args, domainID)
	}
	query = "SELECT * FROM (" + query + ") sub ORDER BY last_message_at DESC LIMIT 50"

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, dID, subject, folder, snippet string
		var originalTo *string
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int
		var starred bool

		rows.Scan(&id, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount,
			&starred, &folder, &snippet, &originalTo, &createdAt)

		t := map[string]interface{}{
			"id":                 id,
			"domain_id":          dID,
			"subject":            subject,
			"participant_emails": participants,
			"last_message_at":    lastMessageAt,
			"message_count":      messageCount,
			"unread_count":       unreadCount,
			"starred":            starred,
			"folder":             folder,
			"snippet":            snippet,
			"created_at":         createdAt,
		}
		if originalTo != nil {
			t["original_to"] = *originalTo
		}
		results = append(results, t)
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"threads": results})
}
