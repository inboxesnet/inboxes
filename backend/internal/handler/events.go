package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

type EventHandler struct {
	Store         store.Store
	CatchupMaxAge time.Duration
}

// Since returns events after a given event ID for reconnection catchup.
// GET /api/events?since={lastEventId}&limit=100
// Returns 410 Gone if the sinceID points to an event older than CatchupMaxAge.
func (h *EventHandler) Since(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	sinceID, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	ctx := r.Context()
	maxAge := h.CatchupMaxAge
	if maxAge <= 0 {
		maxAge = 48 * time.Hour
	}

	// Check if sinceID is too old — return 410 Gone so client does a full refetch
	if sinceID > 0 {
		var eventAge time.Time
		err := h.Store.Q().QueryRow(ctx,
			"SELECT created_at FROM events WHERE id = $1", sinceID,
		).Scan(&eventAge)
		if err != nil || time.Since(eventAge) > maxAge {
			writeJSON(w, http.StatusGone, map[string]interface{}{
				"error": "events too old, please refetch",
			})
			return
		}
	}

	// Non-admins only get events for threads their aliases can see. This
	// mirrors the WS hub: broadcast event types and events without a thread
	// (or whose thread has no alias labels) go to every org member.
	isAdmin := claims.Role == "admin"
	var aliasLabels []string
	if !isAdmin {
		addrs, aliasErr := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasErr != nil {
			// Fail closed: without the alias list we cannot scope thread events.
			writeError(w, http.StatusInternalServerError, "failed to fetch events")
			return
		}
		aliasLabels = make([]string, len(addrs))
		for i, addr := range addrs {
			aliasLabels[i] = "alias:" + addr
		}
	}
	broadcastTypes := []string{"sync.completed", "plan.changed", "thread.bulk_action"}

	rows, err := h.Store.Q().Query(ctx,
		`SELECT id, event_type, domain_id, thread_id, payload, created_at
		 FROM events
		 WHERE org_id = $1 AND id > $2 AND created_at > $4
		 AND ($5 OR thread_id IS NULL OR event_type = ANY($6::text[])
		      OR NOT EXISTS (SELECT 1 FROM thread_labels tl
		                     WHERE tl.thread_id = events.thread_id AND tl.label LIKE 'alias:%')
		      OR EXISTS (SELECT 1 FROM thread_labels tl
		                 WHERE tl.thread_id = events.thread_id AND tl.label = ANY($7::text[])))
		 ORDER BY id ASC
		 LIMIT $3`,
		claims.OrgID, sinceID, limit, time.Now().Add(-maxAge),
		isAdmin, broadcastTypes, aliasLabels,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch events")
		return
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id int64
		var eventType string
		var domainID, threadID *string
		var payload json.RawMessage
		var createdAt time.Time

		if err := rows.Scan(&id, &eventType, &domainID, &threadID, &payload, &createdAt); err != nil {
			continue
		}

		evt := map[string]interface{}{
			"id":         id,
			"event":      eventType,
			"created_at": createdAt,
		}
		setIfNotNil(evt, "domain_id", domainID)
		setIfNotNil(evt, "thread_id", threadID)
		if payload != nil {
			var p map[string]interface{}
			if json.Unmarshal(payload, &p) == nil {
				evt["payload"] = p
			}
		}
		events = append(events, evt)
	}

	if events == nil {
		events = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
	})
}

// warningEventsWhere selects the event types the bell notification center
// shows. Transient events stay out — toasts cover them.
const warningEventsWhere = `(
	event_type IN ('domain.disconnected','domain.not_found','domain.dns_degraded','domain.reconnected')
	OR (event_type = 'email.status_updated' AND payload->>'status' IN ('failed','bounced','complained')
	    AND payload->>'dismissed' IS NULL)
	OR (event_type = 'plan.changed' AND payload->>'plan' IN ('cancelled','past_due'))
)`

// Notifications returns recent warning events for the bell, newest first,
// with the caller's read cursor and an unread count.
// GET /api/notifications?limit=50
func (h *EventHandler) Notifications(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var readCursor int64
	if err := h.Store.Q().QueryRow(ctx,
		"SELECT notifications_read_id FROM users WHERE id = $1", claims.UserID,
	).Scan(&readCursor); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notifications")
		return
	}

	// Non-admins only get thread-bound events their aliases can see. This
	// mirrors the catchup endpoint above.
	isAdmin := claims.Role == "admin"
	var aliasLabels []string
	if !isAdmin {
		addrs, aliasErr := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch notifications")
			return
		}
		aliasLabels = make([]string, len(addrs))
		for i, addr := range addrs {
			aliasLabels[i] = "alias:" + addr
		}
	}

	rows, err := h.Store.Q().Query(ctx,
		`SELECT id, event_type, domain_id, thread_id, payload, created_at
		 FROM events
		 WHERE org_id = $1 AND created_at > now() - interval '30 days'
		 AND `+warningEventsWhere+`
		 AND ($3 OR thread_id IS NULL
		      OR NOT EXISTS (SELECT 1 FROM thread_labels tl
		                     WHERE tl.thread_id = events.thread_id AND tl.label LIKE 'alias:%')
		      OR EXISTS (SELECT 1 FROM thread_labels tl
		                 WHERE tl.thread_id = events.thread_id AND tl.label = ANY($4::text[])))
		 ORDER BY id DESC
		 LIMIT $2`,
		claims.OrgID, limit, isAdmin, aliasLabels,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notifications")
		return
	}
	defer rows.Close()

	unread := 0
	var events []map[string]interface{}
	for rows.Next() {
		var id int64
		var eventType string
		var domainID, threadID *string
		var payload json.RawMessage
		var createdAt time.Time

		if err := rows.Scan(&id, &eventType, &domainID, &threadID, &payload, &createdAt); err != nil {
			continue
		}

		evt := map[string]interface{}{
			"id":         id,
			"event":      eventType,
			"created_at": createdAt,
			"read":       id <= readCursor,
		}
		if id > readCursor {
			unread++
		}
		setIfNotNil(evt, "domain_id", domainID)
		setIfNotNil(evt, "thread_id", threadID)
		if payload != nil {
			var p map[string]interface{}
			if json.Unmarshal(payload, &p) == nil {
				evt["payload"] = p
			}
		}
		events = append(events, evt)
	}

	if events == nil {
		events = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events":       events,
		"unread_count": unread,
		"read_cursor":  readCursor,
	})
}

// MarkNotificationsRead moves the caller's read cursor forward. The cursor
// never moves backward.
// POST /api/notifications/read {"last_id": 123}
func (h *EventHandler) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		LastID int64 `json:"last_id"`
	}
	if err := readJSON(r, &req); err != nil || req.LastID <= 0 {
		writeError(w, http.StatusBadRequest, "last_id is required")
		return
	}

	if _, err := h.Store.Q().Exec(r.Context(),
		`UPDATE users SET notifications_read_id = GREATEST(notifications_read_id, $1) WHERE id = $2`,
		req.LastID, claims.UserID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark notifications read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
