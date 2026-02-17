package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DraftHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Bus       *event.Bus
}

func (h *DraftHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	domainID := r.URL.Query().Get("domain_id")
	ctx := r.Context()

	query := `SELECT id, domain_id, thread_id, kind, subject, from_address,
		to_addresses, cc_addresses, bcc_addresses, body_html, body_plain,
		created_at, updated_at
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
		var toAddr, ccAddr, bccAddr json.RawMessage
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &domID, &threadID, &kind, &subject, &fromAddr,
			&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain,
			&createdAt, &updatedAt)
		if err != nil {
			continue
		}

		draft := map[string]interface{}{
			"id":           id,
			"domain_id":    domID,
			"thread_id":    threadID,
			"kind":         kind,
			"subject":      subject,
			"from_address": fromAddr,
			"to_addresses": json.RawMessage(toAddr),
			"cc_addresses": json.RawMessage(ccAddr),
			"bcc_addresses": json.RawMessage(bccAddr),
			"body_html":    bodyHTML,
			"body_plain":   bodyPlain,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
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
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		DomainID    string   `json:"domain_id"`
		ThreadID    *string  `json:"thread_id"`
		Kind        string   `json:"kind"`
		Subject     string   `json:"subject"`
		FromAddress string   `json:"from_address"`
		To          []string `json:"to_addresses"`
		CC          []string `json:"cc_addresses"`
		BCC         []string `json:"bcc_addresses"`
		BodyHTML    string   `json:"body_html"`
		BodyPlain   string   `json:"body_plain"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DomainID == "" {
		writeError(w, http.StatusBadRequest, "domain_id is required")
		return
	}
	if req.Kind == "" {
		req.Kind = "compose"
	}

	toJSON, _ := json.Marshal(req.To)
	ccJSON, _ := json.Marshal(req.CC)
	bccJSON, _ := json.Marshal(req.BCC)

	var id string
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO drafts (org_id, user_id, domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id`,
		claims.OrgID, claims.UserID, req.DomainID, req.ThreadID, req.Kind,
		req.Subject, req.FromAddress, toJSON, ccJSON, bccJSON,
		req.BodyHTML, req.BodyPlain,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create draft")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id})
}

func (h *DraftHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")

	var req struct {
		Subject     *string  `json:"subject"`
		FromAddress *string  `json:"from_address"`
		To          []string `json:"to_addresses"`
		CC          []string `json:"cc_addresses"`
		BCC         []string `json:"bcc_addresses"`
		BodyHTML    *string  `json:"body_html"`
		BodyPlain   *string  `json:"body_plain"`
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
		toJSON, _ := json.Marshal(req.To)
		setClauses = append(setClauses, "to_addresses = $"+itoa(argIdx))
		args = append(args, toJSON)
		argIdx++
	}
	if req.CC != nil {
		ccJSON, _ := json.Marshal(req.CC)
		setClauses = append(setClauses, "cc_addresses = $"+itoa(argIdx))
		args = append(args, ccJSON)
		argIdx++
	}
	if req.BCC != nil {
		bccJSON, _ := json.Marshal(req.BCC)
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
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Fetch draft
	var domainID, kind, subject, fromAddr, bodyHTML, bodyPlain string
	var threadID *string
	var toAddr, ccAddr, bccAddr json.RawMessage

	err := h.DB.QueryRow(ctx,
		`SELECT domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain
		 FROM drafts WHERE id = $1 AND user_id = $2`,
		id, claims.UserID,
	).Scan(&domainID, &threadID, &kind, &subject, &fromAddr,
		&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain)
	if err != nil {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	var to, cc, bcc []string
	json.Unmarshal(toAddr, &to)
	json.Unmarshal(ccAddr, &cc)
	json.Unmarshal(bccAddr, &bcc)

	if fromAddr == "" || len(to) == 0 || subject == "" {
		writeError(w, http.StatusBadRequest, "from, to, and subject are required to send")
		return
	}

	// Resolve display name for From field
	fromDisplay := resolveFromDisplay(ctx, h.DB, claims.OrgID, fromAddr)

	// Send via Resend
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

	data, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "POST", "/emails", resendPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send email: "+err.Error())
		return
	}

	var resendResp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &resendResp)

	// Build snippet
	snippet := truncateRunes(bodyPlain, 200)

	// Find or create thread
	var finalThreadID string
	if threadID != nil && *threadID != "" {
		finalThreadID = *threadID
	} else {
		participants, _ := json.Marshal(append([]string{fromAddr}, to...))
		h.DB.QueryRow(ctx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, folder, snippet)
			 VALUES ($1, $2, $3, $4, $5, 'sent', $6) RETURNING id`,
			claims.OrgID, claims.UserID, domainID, subject, participants, snippet,
		).Scan(&finalThreadID)
	}

	// Store email
	toJSON, _ := json.Marshal(to)
	ccJSON, _ := json.Marshal(cc)
	bccJSON, _ := json.Marshal(bcc)

	var emailID string
	h.DB.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain, status)
		 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, $8, $9, $10, $11, $12, 'sent')
		 RETURNING id`,
		finalThreadID, claims.UserID, claims.OrgID, domainID, resendResp.ID,
		fromAddr, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain,
	).Scan(&emailID)

	// Update thread
	h.DB.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, updated_at = now()
		 WHERE id = $1`, finalThreadID, snippet,
	)

	// Delete draft
	h.DB.Exec(ctx, "DELETE FROM drafts WHERE id = $1", id)

	h.Bus.Publish(ctx, event.Event{
		EventType: event.EmailSent,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  finalThreadID,
		Payload: map[string]interface{}{
			"email_id": emailID,
		},
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"email_id":  emailID,
		"resend_id": resendResp.ID,
		"thread_id": finalThreadID,
	})
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
