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
	DomainID string   `json:"domain_id"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	CC       []string `json:"cc"`
	BCC      []string `json:"bcc"`
	Subject  string   `json:"subject"`
	HTML     string   `json:"html"`
	Text     string   `json:"text"`
	ReplyTo  string   `json:"reply_to_thread_id"`
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

	// Send via Resend
	resendPayload := map[string]interface{}{
		"from":    req.From,
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
	snippet := req.Text
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}

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

	var emailID string
	h.DB.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain, status)
		 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, $8, $9, $10, $11, $12, 'sent')
		 RETURNING id`,
		threadID, claims.UserID, claims.OrgID, domainID, resendResp.ID,
		req.From, toJSON, ccJSON, bccJSON, req.Subject, req.HTML, req.Text,
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
	query := `SELECT e.id, e.thread_id, e.from_address, e.subject, e.created_at,
		t.folder, t.domain_id
		FROM emails e JOIN threads t ON e.thread_id = t.id
		WHERE e.org_id = $1 AND e.search_vector @@ plainto_tsquery('english', $2)`
	args := []interface{}{claims.OrgID, q}

	if domainID != "" {
		query += " AND e.domain_id = $3"
		args = append(args, domainID)
	}
	query += " ORDER BY e.created_at DESC LIMIT 50"

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var eID, threadID, from, subject, folder, dID string
		var createdAt time.Time
		rows.Scan(&eID, &threadID, &from, &subject, &createdAt, &folder, &dID)
		results = append(results, map[string]interface{}{
			"id":           eID,
			"thread_id":    threadID,
			"from_address": from,
			"subject":      subject,
			"folder":       folder,
			"domain_id":    dID,
			"created_at":   createdAt,
		})
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"threads": results})
}
