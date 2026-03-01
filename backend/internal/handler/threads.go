package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// labelsSubquery is a SQL fragment that aggregates labels for a thread.
const labelsSubquery = `(SELECT COALESCE(array_agg(tl2.label ORDER BY tl2.label), ARRAY[]::text[]) FROM thread_labels tl2 WHERE tl2.thread_id = t.id)`

// getUserAliasAddresses returns the list of alias addresses assigned to a user.
func getUserAliasAddresses(ctx context.Context, db *pgxpool.Pool, userID string) []string {
	rows, err := db.Query(ctx,
		`SELECT a.address FROM aliases a
		 JOIN alias_users au ON au.alias_id = a.id
		 WHERE au.user_id = $1`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var addr string
		if rows.Scan(&addr) == nil {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// batchFetchLabels fetches labels for a set of thread IDs in a single query.
func batchFetchLabels(ctx context.Context, db *pgxpool.Pool, threadIDs []string) map[string][]string {
	labelMap := make(map[string][]string, len(threadIDs))
	rows, err := db.Query(ctx,
		`SELECT thread_id, COALESCE(array_agg(label ORDER BY label), ARRAY[]::text[])
		 FROM thread_labels WHERE thread_id = ANY($1::uuid[]) GROUP BY thread_id`, threadIDs)
	if err != nil {
		slog.Warn("threads: batch label fetch failed", "error", err)
		return labelMap
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var labels []string
		if rows.Scan(&tid, &labels) == nil {
			labelMap[tid] = labels
		}
	}
	return labelMap
}

// appendAliasVisibility appends a visibility filter for non-admin users.
// It checks thread_labels for alias:<address> labels matching the user's assigned aliases.
func appendAliasVisibility(filter string, args []interface{}, argIdx int, aliasAddrs []string) (string, []interface{}, int) {
	labels := make([]string, len(aliasAddrs))
	for i, addr := range aliasAddrs {
		labels[i] = "alias:" + addr
	}
	filter += ` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($` + strconv.Itoa(argIdx) + `::text[]))`
	args = append(args, labels)
	return filter, args, argIdx + 1
}

func (h *ThreadHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domainID := r.URL.Query().Get("domain_id")
	label := r.URL.Query().Get("label")
	if label == "" {
		label = "inbox"
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

	var countQuery, query string
	var args []interface{}
	argIdx := 1

	switch label {
	case "archive":
		// Archive = no inbox label, no trash/spam
		countQuery = `SELECT COUNT(*) FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		args = append(args, claims.OrgID)
		argIdx = 2

	case "trash", "spam":
		// Trash/spam: just the label, no exclusion
		countQuery = `SELECT COUNT(*) FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		args = append(args, claims.OrgID, label)
		argIdx = 3

	default:
		// inbox, sent, starred: label + exclude trash/spam
		countQuery = `SELECT COUNT(*) FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		args = append(args, claims.OrgID, label)
		argIdx = 3
	}

	if domainID != "" {
		domainFilter := " AND t.domain_id = $" + strconv.Itoa(argIdx)
		countQuery += domainFilter
		query += domainFilter
		args = append(args, domainID)
		argIdx++
	}

	// Alias visibility: non-admins only see threads related to their aliases
	if claims.Role != "admin" {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}

		var vis string
		vis, args, argIdx = appendAliasVisibility("", args, argIdx, aliasAddrs)
		countQuery += vis
		query += vis
	}

	// Count total
	var total int
	warnIfErr(h.DB.QueryRow(ctx, countQuery, args...).Scan(&total), "threads: count query failed")

	// Add ORDER BY + pagination
	query += " ORDER BY t.last_message_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch threads")
		return
	}
	defer rows.Close()

	var threads []map[string]interface{}
	var threadIDs []string
	for rows.Next() {
		var id, orgID, userID, dID, subject, snippet string
		var originalTo *string
		var trashExpiresAt *time.Time
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int

		err := rows.Scan(&id, &orgID, &userID, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount, &snippet, &originalTo, &createdAt, &trashExpiresAt)
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
			"snippet":            snippet,
			"created_at":         createdAt,
		}
		setIfNotNil(t, "original_to", originalTo)
		setIfNotNil(t, "trash_expires_at", trashExpiresAt)
		threads = append(threads, t)
		threadIDs = append(threadIDs, id)
	}
	if threads == nil {
		threads = []map[string]interface{}{}
	}

	// Batch-fetch labels for all threads in one query
	if len(threadIDs) > 0 {
		labelMap := batchFetchLabels(ctx, h.DB, threadIDs)
		for _, t := range threads {
			tid := t["id"].(string)
			if lbls, ok := labelMap[tid]; ok {
				t["labels"] = lbls
			} else {
				t["labels"] = []string{}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"threads": threads,
		"page":    page,
		"total":   total,
	})
}

func (h *ThreadHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var id, orgID, userID, domainID, subject, snippet string
	var participants json.RawMessage
	var trashExpiresAt *time.Time
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := h.DB.QueryRow(ctx,
		`SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.created_at,
		 t.trash_expires_at,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, claims.OrgID,
	).Scan(&id, &orgID, &userID, &domainID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &createdAt, &trashExpiresAt, &labels)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if labels == nil {
		labels = []string{}
	}

	// Alias visibility check for non-admins
	if claims.Role != "admin" {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		var visible bool
		if err := h.DB.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = $1 AND al.label = ANY($2::text[]))`,
			threadID, labels,
		).Scan(&visible); err != nil {
			slog.Warn("threads: visibility check failed", "thread_id", threadID, "error", err)
		}
		if !visible {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
	}

	// Fetch emails
	emailRows, err := h.DB.Query(ctx,
		`SELECT id, direction, from_address, to_addresses, cc_addresses, reply_to_addresses, subject,
		 body_html, body_plain, status, attachments, message_id, in_reply_to, references_header,
		 delivered_via_alias, sent_as_alias, is_read, created_at
		 FROM emails WHERE thread_id = $1 AND org_id = $2 ORDER BY created_at ASC`, threadID, claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch emails")
		return
	}
	defer emailRows.Close()

	var emails []map[string]interface{}
	for emailRows.Next() {
		var eID, dir, from, eSubject, eStatus string
		var eTo, eCC, eReplyTo json.RawMessage
		var bodyHTML, bodyPlain, messageID, inReplyTo *string
		var deliveredViaAlias, sentAsAlias *string
		var attachments, refsHeader json.RawMessage
		var isRead bool
		var eCreatedAt time.Time

		emailRows.Scan(&eID, &dir, &from, &eTo, &eCC, &eReplyTo, &eSubject,
			&bodyHTML, &bodyPlain, &eStatus, &attachments, &messageID, &inReplyTo, &refsHeader,
			&deliveredViaAlias, &sentAsAlias, &isRead, &eCreatedAt)

		email := map[string]interface{}{
			"id":                 eID,
			"direction":          dir,
			"from_address":       from,
			"to_addresses":       eTo,
			"cc_addresses":       eCC,
			"reply_to_addresses": eReplyTo,
			"subject":            eSubject,
			"status":             eStatus,
			"attachments":        attachments,
			"is_read":            isRead,
			"created_at":         eCreatedAt,
		}
		setIfNotNil(email, "body_html", bodyHTML)
		setIfNotNil(email, "body_plain", bodyPlain)
		setIfNotNil(email, "message_id", messageID)
		setIfNotNil(email, "in_reply_to", inReplyTo)
		if refsHeader != nil {
			email["references"] = refsHeader
		}
		setIfNotNil(email, "delivered_via_alias", deliveredViaAlias)
		setIfNotNil(email, "sent_as_alias", sentAsAlias)
		emails = append(emails, email)
	}
	if emails == nil {
		emails = []map[string]interface{}{}
	}

	threadMap := map[string]interface{}{
		"id":                 id,
		"domain_id":          domainID,
		"subject":            subject,
		"participant_emails": participants,
		"last_message_at":    lastMessageAt,
		"message_count":      messageCount,
		"unread_count":       unreadCount,
		"labels":             labels,
		"snippet":            snippet,
		"created_at":         createdAt,
		"emails":             emails,
	}
	setIfNotNil(threadMap, "trash_expires_at", trashExpiresAt)

	writeJSON(w, http.StatusOK, map[string]interface{}{"thread": threadMap})
}

func (h *ThreadHandler) BulkAction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		ThreadIDs     []string `json:"thread_ids"`
		Action        string   `json:"action"`
		Label         string   `json:"label"`
		SelectAll     bool     `json:"select_all"`
		FilterLabel   string   `json:"filter_label"`
		FilterDomain  string   `json:"filter_domain_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	ctx := r.Context()

	// Select-all mode: resolve thread IDs from filters
	if req.SelectAll {
		if req.FilterLabel == "" {
			req.FilterLabel = "inbox"
		}
		resolved, err := h.resolveFilteredThreadIDs(ctx, claims, req.FilterLabel, req.FilterDomain)
		if err != nil {
			slog.Error("threads: resolve select-all IDs failed", "error", err)
			if err.Error() == "too many threads selected; please narrow your filter" {
				writeError(w, http.StatusBadRequest, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "failed to resolve threads")
			}
			return
		}
		req.ThreadIDs = resolved
	}

	if len(req.ThreadIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "updated", "affected": 0})
		return
	}

	// Alias visibility: non-admins can only act on threads they have access to
	if claims.Role != "admin" && !req.SelectAll {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		var allowed []string
		rows, err := h.DB.Query(ctx,
			`SELECT DISTINCT tl.thread_id FROM thread_labels tl
			 WHERE tl.thread_id = ANY($1::uuid[]) AND tl.label = ANY($2::text[])`,
			req.ThreadIDs, labels)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tid string
				if rows.Scan(&tid) == nil {
					allowed = append(allowed, tid)
				}
			}
		}
		req.ThreadIDs = allowed
		if len(req.ThreadIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{"message": "updated", "affected": 0})
			return
		}
	}

	var affected int64

	switch req.Action {
	case "archive":
		if err := bulkRemoveLabel(ctx, h.DB, req.ThreadIDs, "inbox"); err != nil {
			slog.Error("threads: bulk archive failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to archive threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "trash":
		tx, err := h.DB.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to trash threads")
			return
		}
		defer tx.Rollback(ctx)
		if err := bulkAddLabel(ctx, tx, req.ThreadIDs, claims.OrgID, "trash"); err != nil {
			slog.Error("threads: bulk trash label failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to trash threads")
			return
		}
		if _, err := tx.Exec(ctx,
			`UPDATE threads SET trash_expires_at = now() + interval '30 days', updated_at = now()
			 WHERE id = ANY($1::uuid[]) AND org_id = $2`, req.ThreadIDs, claims.OrgID); err != nil {
			slog.Error("threads: bulk trash expiry update failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to trash threads")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to trash threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "spam":
		if err := bulkAddLabel(ctx, h.DB, req.ThreadIDs, claims.OrgID, "spam"); err != nil {
			slog.Error("threads: bulk spam label failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as spam")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "read":
		tag, err := h.DB.Exec(ctx,
			"UPDATE threads SET unread_count = 0, updated_at = now() WHERE id = ANY($1::uuid[]) AND org_id = $2",
			req.ThreadIDs, claims.OrgID)
		if err != nil {
			slog.Error("threads: bulk mark read failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as read")
			return
		}
		affected = tag.RowsAffected()

	case "unread":
		tag, err := h.DB.Exec(ctx,
			"UPDATE threads SET unread_count = 1, updated_at = now() WHERE id = ANY($1::uuid[]) AND org_id = $2",
			req.ThreadIDs, claims.OrgID)
		if err != nil {
			slog.Error("threads: bulk mark unread failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as unread")
			return
		}
		affected = tag.RowsAffected()

	case "move":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for move action")
			return
		}
		validSystemLabels := map[string]bool{"inbox": true, "sent": true, "archive": true, "trash": true, "spam": true}
		if !validSystemLabels[req.Label] {
			writeError(w, http.StatusBadRequest, "invalid label: "+req.Label)
			return
		}
		tx, err := h.DB.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move threads")
			return
		}
		defer tx.Rollback(ctx)

		var moveErr error
		switch req.Label {
		case "inbox":
			if moveErr = bulkAddLabel(ctx, tx, req.ThreadIDs, claims.OrgID, "inbox"); moveErr != nil {
				break
			}
			if moveErr = bulkRemoveLabel(ctx, tx, req.ThreadIDs, "trash"); moveErr != nil {
				break
			}
			if moveErr = bulkRemoveLabel(ctx, tx, req.ThreadIDs, "spam"); moveErr != nil {
				break
			}
			_, moveErr = tx.Exec(ctx,
				`UPDATE threads SET trash_expires_at = NULL, updated_at = now()
				 WHERE id = ANY($1::uuid[]) AND org_id = $2`, req.ThreadIDs, claims.OrgID)
		case "trash":
			if moveErr = bulkAddLabel(ctx, tx, req.ThreadIDs, claims.OrgID, "trash"); moveErr != nil {
				break
			}
			_, moveErr = tx.Exec(ctx,
				`UPDATE threads SET trash_expires_at = now() + interval '30 days', updated_at = now()
				 WHERE id = ANY($1::uuid[]) AND org_id = $2`, req.ThreadIDs, claims.OrgID)
		case "spam":
			moveErr = bulkAddLabel(ctx, tx, req.ThreadIDs, claims.OrgID, "spam")
		case "archive":
			moveErr = bulkRemoveLabel(ctx, tx, req.ThreadIDs, "inbox")
		default:
			moveErr = bulkAddLabel(ctx, tx, req.ThreadIDs, claims.OrgID, req.Label)
		}
		if moveErr != nil {
			slog.Error("threads: bulk move failed", "label", req.Label, "error", moveErr)
			writeError(w, http.StatusInternalServerError, "failed to move threads")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "label":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for label action")
			return
		}
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
		if err := bulkAddLabel(ctx, h.DB, req.ThreadIDs, claims.OrgID, req.Label); err != nil {
			slog.Error("threads: bulk add label failed", "label", req.Label, "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "unlabel":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for unlabel action")
			return
		}
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
		if err := bulkRemoveLabel(ctx, h.DB, req.ThreadIDs, req.Label); err != nil {
			slog.Error("threads: bulk remove label failed", "label", req.Label, "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "mute":
		if err := bulkAddLabel(ctx, h.DB, req.ThreadIDs, claims.OrgID, "muted"); err != nil {
			slog.Error("threads: bulk mute failed", "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "unmute":
		if err := bulkRemoveLabel(ctx, h.DB, req.ThreadIDs, "muted"); err != nil {
			slog.Error("threads: bulk unmute failed", "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "delete":
		// Batch: filter to threads with "trash" label, then delete in one transaction
		rows, err := h.DB.Query(ctx,
			`SELECT DISTINCT thread_id FROM thread_labels
			 WHERE thread_id = ANY($1::uuid[]) AND label = 'trash'`,
			req.ThreadIDs)
		if err != nil {
			slog.Error("threads: bulk delete filter failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete threads")
			return
		}
		var trashIDs []string
		for rows.Next() {
			var tid string
			if err := rows.Scan(&tid); err != nil {
				slog.Error("threads: bulk delete scan failed", "error", err)
				continue
			}
			trashIDs = append(trashIDs, tid)
		}
		rows.Close()

		if len(trashIDs) > 0 {
			tx, err := h.DB.Begin(ctx)
			if err != nil {
				slog.Error("threads: bulk delete begin tx failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to delete threads")
				return
			}
			defer tx.Rollback(ctx)

			if _, err := tx.Exec(ctx,
				`DELETE FROM thread_labels WHERE thread_id = ANY($1::uuid[])`,
				trashIDs); err != nil {
				slog.Error("threads: bulk delete remove labels failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to delete threads")
				return
			}
			tag, err := tx.Exec(ctx,
				`UPDATE threads SET deleted_at = now(), updated_at = now()
				 WHERE id = ANY($1::uuid[]) AND org_id = $2`,
				trashIDs, claims.OrgID)
			if err != nil {
				slog.Error("threads: bulk delete soft-delete failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to delete threads")
				return
			}
			if err := tx.Commit(ctx); err != nil {
				slog.Error("threads: bulk delete commit failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to delete threads")
				return
			}
			affected = tag.RowsAffected()
		}

	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadBulkAction,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		Payload: map[string]interface{}{
			"action":     req.Action,
			"thread_ids": req.ThreadIDs,
			"label":      req.Label,
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "updated",
		"affected": affected,
	})
}

func (h *ThreadHandler) Move(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	threadID := chi.URLParam(r, "id")

	var req struct {
		Label string `json:"label"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	ctx := r.Context()

	// Alias visibility check for non-admins
	if claims.Role != "admin" {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		var visible bool
		if err := h.DB.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = $1 AND al.label = ANY($2::text[]))`,
			threadID, labels,
		).Scan(&visible); err != nil {
			slog.Warn("threads: visibility check failed", "thread_id", threadID, "error", err)
		}
		if !visible {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
	}

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move thread")
		return
	}
	defer tx.Rollback(ctx)

	var txErr error
	switch req.Label {
	case "inbox":
		if txErr = addLabel(ctx, tx, threadID, claims.OrgID, "inbox"); txErr != nil {
			break
		}
		if txErr = removeLabel(ctx, tx, threadID, "trash"); txErr != nil {
			break
		}
		if txErr = removeLabel(ctx, tx, threadID, "spam"); txErr != nil {
			break
		}
		_, txErr = tx.Exec(ctx, "UPDATE threads SET trash_expires_at = NULL, updated_at = now() WHERE id = $1 AND org_id = $2",
			threadID, claims.OrgID)
	case "trash":
		if txErr = addLabel(ctx, tx, threadID, claims.OrgID, "trash"); txErr != nil {
			break
		}
		_, txErr = tx.Exec(ctx, "UPDATE threads SET trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = $1 AND org_id = $2",
			threadID, claims.OrgID)
	case "spam":
		txErr = addLabel(ctx, tx, threadID, claims.OrgID, "spam")
	case "archive":
		txErr = removeLabel(ctx, tx, threadID, "inbox")
	default:
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
		txErr = addLabel(ctx, tx, threadID, claims.OrgID, req.Label)
	}
	if txErr != nil {
		slog.Error("threads: move failed", "thread_id", threadID, "label", req.Label, "error", txErr)
		writeError(w, http.StatusInternalServerError, "failed to move thread")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move thread")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadMoved,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"to_label": req.Label,
			"thread":    h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "moved"})
}

func (h *ThreadHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET unread_count = 0, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	// Mark all emails in thread as read
	if _, err := h.DB.Exec(ctx, "UPDATE emails SET is_read = true WHERE thread_id = $1 AND org_id = $2", threadID, claims.OrgID); err != nil {
		slog.Error("threads: mark emails as read failed", "thread_id", threadID, "error", err)
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	tag, err := h.DB.Exec(ctx,
		"UPDATE threads SET unread_count = 1, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	// Mark latest email as unread
	if _, err := h.DB.Exec(ctx,
		`UPDATE emails SET is_read = false WHERE id = (
		  SELECT id FROM emails WHERE thread_id = $1 AND org_id = $2 ORDER BY created_at DESC LIMIT 1
		)`, threadID, claims.OrgID); err != nil {
		slog.Error("threads: mark latest email as unread failed", "thread_id", threadID, "error", err)
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	err := h.DB.QueryRow(ctx,
		"SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	).Scan(&domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	// Accept optional { starred: bool } for idempotent set-value operations.
	// Falls back to toggle behavior if no body is provided.
	var req struct {
		Starred *bool `json:"starred"`
	}
	_ = readJSON(r, &req) // Ignore parse errors — treat as toggle

	var wantStarred bool
	if req.Starred != nil {
		wantStarred = *req.Starred
	} else {
		wantStarred = !hasLabel(ctx, h.DB, threadID, "starred")
	}

	if wantStarred {
		addLabel(ctx, h.DB, threadID, claims.OrgID, "starred")
	} else {
		removeLabel(ctx, h.DB, threadID, "starred")
	}

	evtType := event.ThreadStarred
	if !wantStarred {
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

func (h *ThreadHandler) Mute(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var domainID string
	err := h.DB.QueryRow(ctx,
		"SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	).Scan(&domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	muted := hasLabel(ctx, h.DB, threadID, "muted")
	if muted {
		removeLabel(ctx, h.DB, threadID, "muted")
	} else {
		addLabel(ctx, h.DB, threadID, claims.OrgID, "muted")
	}

	evtType := event.ThreadMuted
	if muted {
		evtType = event.ThreadUnmuted
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	if err := removeLabel(ctx, h.DB, threadID, "inbox"); err != nil {
		slog.Error("threads: archive removeLabel failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to archive thread")
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to trash thread")
		return
	}
	defer tx.Rollback(ctx)

	if err := addLabel(ctx, tx, threadID, claims.OrgID, "trash"); err != nil {
		slog.Error("threads: trash addLabel failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to trash thread")
		return
	}
	if _, err := tx.Exec(ctx,
		"UPDATE threads SET trash_expires_at = now() + interval '30 days', updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID); err != nil {
		slog.Error("threads: trash set trash_expires_at failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to trash thread")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to trash thread")
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req struct {
		Action string `json:"action"`
	}
	readJSON(r, &req)

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	evtType := event.ThreadSpammed
	payload := map[string]interface{}{}

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update spam status")
		return
	}
	defer tx.Rollback(ctx)

	if req.Action == "not_spam" {
		if err := removeLabel(ctx, tx, threadID, "spam"); err != nil {
			slog.Error("threads: not_spam removeLabel failed", "thread_id", threadID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update spam status")
			return
		}
		if err := addLabel(ctx, tx, threadID, claims.OrgID, "inbox"); err != nil {
			slog.Error("threads: not_spam addLabel inbox failed", "thread_id", threadID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update spam status")
			return
		}
		evtType = event.ThreadMoved
		payload["to_label"] = "inbox"
	} else {
		if err := addLabel(ctx, tx, threadID, claims.OrgID, "spam"); err != nil {
			slog.Error("threads: spam addLabel failed", "thread_id", threadID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update spam status")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update spam status")
		return
	}

	payload["thread"] = h.fetchThreadSummary(ctx, threadID, claims.OrgID)
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
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID := threadDomainID(ctx, h.DB, threadID, claims.OrgID)

	// Only allow deleting threads that have trash label
	if !hasLabel(ctx, h.DB, threadID, "trash") {
		writeError(w, http.StatusNotFound, "thread not found or not in trash")
		return
	}

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	defer tx.Rollback(ctx)

	if err := removeAllLabels(ctx, tx, threadID); err != nil {
		slog.Error("threads: delete removeAllLabels failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	tag, err := tx.Exec(ctx,
		"UPDATE threads SET deleted_at = now(), updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
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

// resolveFilteredThreadIDs returns all thread IDs matching a label + domain filter.
// Uses the same WHERE logic as the List handler to ensure consistency.
func (h *ThreadHandler) resolveFilteredThreadIDs(ctx context.Context, claims *middleware.Claims, label, domainID string) ([]string, error) {
	var query string
	var args []interface{}
	argIdx := 1

	switch label {
	case "archive":
		query = `SELECT t.id FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		args = append(args, claims.OrgID)
		argIdx = 2
	case "trash", "spam":
		query = `SELECT t.id FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		args = append(args, claims.OrgID, label)
		argIdx = 3
	default:
		query = `SELECT t.id FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		args = append(args, claims.OrgID, label)
		argIdx = 3
	}

	if domainID != "" {
		query += " AND t.domain_id = $" + strconv.Itoa(argIdx)
		args = append(args, domainID)
		argIdx++
	}

	if claims.Role != "admin" {
		aliasAddrs := getUserAliasAddresses(ctx, h.DB, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		var vis string
		vis, args, _ = appendAliasVisibility("", args, argIdx, aliasAddrs)
		query += vis
	}

	query += " LIMIT 10001"

	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 10000 {
		return nil, fmt.Errorf("too many threads selected; please narrow your filter")
	}
	return ids, nil
}

// fetchThreadSummary returns a thread map suitable for event payloads.
func (h *ThreadHandler) fetchThreadSummary(ctx context.Context, threadID, orgID string) map[string]interface{} {
	var id, dID, subject, snippet string
	var originalTo *string
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := h.DB.QueryRow(ctx,
		`SELECT t.id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.original_to, t.created_at,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID,
	).Scan(&id, &dID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &originalTo, &createdAt, &labels)
	if err != nil {
		return nil
	}
	if labels == nil {
		labels = []string{}
	}
	t := map[string]interface{}{
		"id":                 id,
		"domain_id":          dID,
		"subject":            subject,
		"participant_emails": participants,
		"last_message_at":    lastMessageAt,
		"message_count":      messageCount,
		"unread_count":       unreadCount,
		"labels":             labels,
		"snippet":            snippet,
		"created_at":         createdAt,
	}
	setIfNotNil(t, "original_to", originalTo)
	return t
}

func (h *ThreadHandler) updateThread(w http.ResponseWriter, r *http.Request, query string) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(r.Context(), query, threadID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}
