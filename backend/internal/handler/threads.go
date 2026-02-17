package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ThreadHandler struct {
	DB  *pgxpool.Pool
	Bus *event.Bus
}

func (h *ThreadHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	domainID := r.URL.Query().Get("domain_id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "inbox"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	ctx := r.Context()

	// Build WHERE clause for reuse in COUNT and SELECT
	where := " WHERE org_id = $1 AND folder = $2 AND deleted_at IS NULL"
	args := []interface{}{claims.OrgID, folder}
	argIdx := 3

	if domainID != "" {
		where += " AND domain_id = $" + strconv.Itoa(argIdx)
		args = append(args, domainID)
		argIdx++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM threads" + where
	h.DB.QueryRow(ctx, countQuery, args...).Scan(&total)

	// Fetch threads
	query := `SELECT id, org_id, user_id, domain_id, subject, participant_emails,
		last_message_at, message_count, unread_count, starred, folder, snippet, original_to, created_at
		FROM threads` + where + " ORDER BY last_message_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch threads")
		return
	}
	defer rows.Close()

	var threads []map[string]interface{}
	for rows.Next() {
		var id, orgID, userID, dID, subject, folder, snippet string
		var originalTo *string
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int
		var starred bool

		err := rows.Scan(&id, &orgID, &userID, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount, &starred, &folder, &snippet, &originalTo, &createdAt)
		if err != nil {
			continue
		}
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
		threads = append(threads, t)
	}
	if threads == nil {
		threads = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"threads": threads,
		"page":    page,
		"total":   total,
	})
}

func (h *ThreadHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var id, orgID, userID, domainID, subject, folder, snippet string
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var starred bool

	err := h.DB.QueryRow(ctx,
		`SELECT id, org_id, user_id, domain_id, subject, participant_emails,
		 last_message_at, message_count, unread_count, starred, folder, snippet, created_at
		 FROM threads WHERE id = $1 AND org_id = $2`,
		threadID, claims.OrgID,
	).Scan(&id, &orgID, &userID, &domainID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &starred, &folder, &snippet, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	// Fetch emails
	emailRows, err := h.DB.Query(ctx,
		`SELECT id, direction, from_address, to_addresses, cc_addresses, subject,
		 body_html, body_plain, status, attachments, message_id, in_reply_to, references_header, created_at
		 FROM emails WHERE thread_id = $1 ORDER BY created_at ASC`, threadID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch emails")
		return
	}
	defer emailRows.Close()

	var emails []map[string]interface{}
	for emailRows.Next() {
		var eID, dir, from, eSubject, eStatus string
		var eTo, eCC json.RawMessage
		var bodyHTML, bodyPlain, messageID, inReplyTo *string
		var attachments, refsHeader json.RawMessage
		var eCreatedAt time.Time

		emailRows.Scan(&eID, &dir, &from, &eTo, &eCC, &eSubject,
			&bodyHTML, &bodyPlain, &eStatus, &attachments, &messageID, &inReplyTo, &refsHeader, &eCreatedAt)

		email := map[string]interface{}{
			"id":           eID,
			"direction":    dir,
			"from_address": from,
			"to_addresses": eTo,
			"cc_addresses": eCC,
			"subject":      eSubject,
			"status":       eStatus,
			"attachments":  attachments,
			"created_at":   eCreatedAt,
		}
		if bodyHTML != nil {
			email["body_html"] = *bodyHTML
		}
		if bodyPlain != nil {
			email["body_plain"] = *bodyPlain
		}
		if messageID != nil {
			email["message_id"] = *messageID
		}
		if inReplyTo != nil {
			email["in_reply_to"] = *inReplyTo
		}
		if refsHeader != nil {
			email["references"] = refsHeader
		}
		emails = append(emails, email)
	}
	if emails == nil {
		emails = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"thread": map[string]interface{}{
			"id":                 id,
			"domain_id":          domainID,
			"subject":            subject,
			"participant_emails": participants,
			"last_message_at":    lastMessageAt,
			"message_count":      messageCount,
			"unread_count":       unreadCount,
			"starred":            starred,
			"folder":             folder,
			"snippet":            snippet,
			"created_at":         createdAt,
			"emails":             emails,
		},
	})
}

func (h *ThreadHandler) BulkAction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		ThreadIDs []string `json:"thread_ids"`
		Action    string   `json:"action"`
		Folder    string   `json:"folder"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.ThreadIDs) == 0 || req.Action == "" {
		writeError(w, http.StatusBadRequest, "thread_ids and action are required")
		return
	}

	ctx := r.Context()
	var query string

	switch req.Action {
	case "archive":
		query = "UPDATE threads SET folder = 'archive', updated_at = now() WHERE id = ANY($1) AND org_id = $2"
	case "trash":
		query = "UPDATE threads SET folder = 'trash', trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = ANY($1) AND org_id = $2"
	case "spam":
		query = "UPDATE threads SET folder = 'spam', updated_at = now() WHERE id = ANY($1) AND org_id = $2"
	case "read":
		query = "UPDATE threads SET unread_count = 0, updated_at = now() WHERE id = ANY($1) AND org_id = $2"
	case "unread":
		query = "UPDATE threads SET unread_count = 1, updated_at = now() WHERE id = ANY($1) AND org_id = $2"
	case "move":
		if req.Folder == "" {
			writeError(w, http.StatusBadRequest, "folder is required for move action")
			return
		}
		validFolders := map[string]bool{"inbox": true, "sent": true, "archive": true, "trash": true, "spam": true}
		if !validFolders[req.Folder] {
			writeError(w, http.StatusBadRequest, "invalid folder: "+req.Folder)
			return
		}
		if req.Folder == "trash" {
			query = "UPDATE threads SET folder = 'trash', trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = ANY($1) AND org_id = $2"
		} else {
			query = "UPDATE threads SET folder = '" + req.Folder + "', trash_expires_at = NULL, updated_at = now() WHERE id = ANY($1) AND org_id = $2"
		}
	case "delete":
		query = "DELETE FROM threads WHERE id = ANY($1) AND org_id = $2 AND folder = 'trash'"
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}

	tag, err := h.DB.Exec(ctx, query, req.ThreadIDs, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to execute bulk action")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadBulkAction,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		Payload: map[string]interface{}{
			"action":     req.Action,
			"thread_ids": req.ThreadIDs,
			"folder":     req.Folder,
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "updated",
		"affected": tag.RowsAffected(),
	})
}

func (h *ThreadHandler) Move(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	threadID := chi.URLParam(r, "id")

	var req struct {
		Folder string `json:"folder"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Folder == "" {
		writeError(w, http.StatusBadRequest, "folder is required")
		return
	}

	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	var query string
	if req.Folder == "trash" {
		query = "UPDATE threads SET folder = 'trash', trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = $1 AND org_id = $2"
	} else {
		query = "UPDATE threads SET folder = $3, trash_expires_at = NULL, updated_at = now() WHERE id = $1 AND org_id = $2"
	}

	var tag interface{ RowsAffected() int64 }
	var err error
	if req.Folder == "trash" {
		tag2, err2 := h.DB.Exec(ctx, query, threadID, claims.OrgID)
		tag = tag2
		err = err2
	} else {
		tag2, err2 := h.DB.Exec(ctx, query, threadID, claims.OrgID, req.Folder)
		tag = tag2
		err = err2
	}

	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadMoved,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"to_folder": req.Folder,
			"thread":    h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "moved"})
}

func (h *ThreadHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET unread_count = 0, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadRead,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) MarkUnread(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET unread_count = 1, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadUnread,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Star(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	// Query current starred state + domain_id before toggling
	var starred bool
	var domainID string
	err := h.DB.QueryRow(ctx,
		"SELECT starred, domain_id FROM threads WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	).Scan(&starred, &domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET starred = NOT starred, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	evtType := event.ThreadStarred
	if starred {
		evtType = event.ThreadUnstarred
	}
	h.Bus.Publish(ctx, event.Event{
		EventType: evtType,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Archive(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET folder = 'archive', updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadArchived,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Trash(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET folder = 'trash', trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadTrashed,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Spam(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req struct {
		Action string `json:"action"`
	}
	readJSON(r, &req)

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	query := "UPDATE threads SET folder = 'spam', updated_at = now() WHERE id = $1 AND org_id = $2"
	evtType := event.ThreadSpammed
	if req.Action == "not_spam" {
		query = "UPDATE threads SET folder = 'inbox', updated_at = now() WHERE id = $1 AND org_id = $2"
		evtType = event.ThreadMoved
	}

	tag, err := h.DB.Exec(ctx, query, threadID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	payload := map[string]interface{}{
		"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
	}
	if req.Action == "not_spam" {
		payload["to_folder"] = "inbox"
	}
	h.Bus.Publish(ctx, event.Event{
		EventType: evtType,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload:   payload,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	h.DB.QueryRow(ctx, "SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2", threadID, claims.OrgID).Scan(&domainID)

	// Only allow deleting threads that are in trash
	tag, err := h.DB.Exec(ctx,
		"DELETE FROM threads WHERE id = $1 AND org_id = $2 AND folder = 'trash'",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found or not in trash")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadDeleted,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (h *ThreadHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	domainID := r.URL.Query().Get("domain_id")
	ctx := r.Context()

	if domainID != "" {
		var count int
		h.DB.QueryRow(ctx,
			`SELECT COALESCE(SUM(unread_count), 0) FROM threads
			 WHERE org_id = $1 AND domain_id = $2 AND folder = 'inbox' AND deleted_at IS NULL`,
			claims.OrgID, domainID,
		).Scan(&count)
		writeJSON(w, http.StatusOK, map[string]int{"count": count})
		return
	}

	// All domains
	rows, err := h.DB.Query(ctx,
		`SELECT domain_id, COALESCE(SUM(unread_count), 0)
		 FROM threads WHERE org_id = $1 AND folder = 'inbox' AND deleted_at IS NULL
		 GROUP BY domain_id`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch counts")
		return
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var dID string
		var count int
		rows.Scan(&dID, &count)
		counts[dID] = count
	}
	writeJSON(w, http.StatusOK, counts)
}

// fetchThreadSummary returns a thread map suitable for event payloads.
func (h *ThreadHandler) fetchThreadSummary(ctx context.Context, threadID, orgID string) map[string]interface{} {
	var id, dID, subject, folder, snippet string
	var originalTo *string
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var starred bool

	err := h.DB.QueryRow(ctx,
		`SELECT id, domain_id, subject, participant_emails,
		 last_message_at, message_count, unread_count, starred, folder, snippet, original_to, created_at
		 FROM threads WHERE id = $1 AND org_id = $2`,
		threadID, orgID,
	).Scan(&id, &dID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &starred, &folder, &snippet, &originalTo, &createdAt)
	if err != nil {
		return nil
	}
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
	return t
}

func (h *ThreadHandler) updateThread(w http.ResponseWriter, r *http.Request, query string) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	threadID := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(r.Context(), query, threadID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}
