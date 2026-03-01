package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventHandler struct {
	DB             *pgxpool.Pool
	CatchupMaxAge  time.Duration
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
		err := h.DB.QueryRow(ctx,
			"SELECT created_at FROM events WHERE id = $1", sinceID,
		).Scan(&eventAge)
		if err != nil || time.Since(eventAge) > maxAge {
			writeJSON(w, http.StatusGone, map[string]interface{}{
				"error": "events too old, please refetch",
			})
			return
		}
	}

	rows, err := h.DB.Query(ctx,
		`SELECT id, event_type, domain_id, thread_id, payload, created_at
		 FROM events
		 WHERE org_id = $1 AND id > $2 AND created_at > $4
		 ORDER BY id ASC
		 LIMIT $3`,
		claims.OrgID, sinceID, limit, time.Now().Add(-maxAge),
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
