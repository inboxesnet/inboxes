package store

import (
	"context"
	"encoding/json"
	"time"
)

func (s *PgStore) GetEventsSince(ctx context.Context, orgID string, since time.Time) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, event_type, domain_id, thread_id, payload, created_at
		 FROM events
		 WHERE org_id = $1 AND created_at > $2
		 ORDER BY id ASC`,
		orgID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var id int64
		var eventType string
		var domainID, threadID *string
		var payload json.RawMessage
		var createdAt time.Time

		if err := rows.Scan(&id, &eventType, &domainID, &threadID, &payload, &createdAt); err != nil {
			continue
		}

		evt := map[string]any{
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
			var p map[string]any
			if json.Unmarshal(payload, &p) == nil {
				evt["payload"] = p
			}
		}
		events = append(events, evt)
	}

	if events == nil {
		events = []map[string]any{}
	}
	return events, nil
}
