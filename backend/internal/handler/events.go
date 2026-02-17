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
	DB *pgxpool.Pool
}

// Since returns events after a given event ID for reconnection catchup.
// GET /api/events?since={lastEventId}&limit=100
func (h *EventHandler) Since(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sinceID, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	ctx := r.Context()

	rows, err := h.DB.Query(ctx,
		`SELECT id, event_type, domain_id, thread_id, payload, created_at
		 FROM events
		 WHERE org_id = $1 AND id > $2
		 ORDER BY id ASC
		 LIMIT $3`,
		claims.OrgID, sinceID, limit,
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
		if domainID != nil {
			evt["domain_id"] = *domainID
		}
		if threadID != nil {
			evt["thread_id"] = *threadID
		}
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
