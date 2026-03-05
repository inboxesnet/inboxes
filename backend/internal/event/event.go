package event

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Event types
const (
	EmailReceived      = "email.received"
	EmailSent          = "email.sent"
	EmailStatusUpdated = "email.status_updated"

	ThreadStarred   = "thread.starred"
	ThreadUnstarred = "thread.unstarred"
	ThreadMuted     = "thread.muted"
	ThreadUnmuted   = "thread.unmuted"
	ThreadRead      = "thread.read"
	ThreadUnread    = "thread.unread"
	ThreadArchived  = "thread.archived"
	ThreadTrashed   = "thread.trashed"
	ThreadSpammed   = "thread.spammed"
	ThreadMoved     = "thread.moved"
	ThreadDeleted   = "thread.deleted"
	ThreadBulkAction = "thread.bulk_action"

	SyncCompleted = "sync.completed"

	PlanChanged = "plan.changed"

	DomainDisconnected = "domain.disconnected"
	DomainReconnected  = "domain.reconnected"
	DomainNotFound     = "domain.not_found"
	DomainDiscovered   = "domain.discovered"
)

type Event struct {
	EventType string                 `json:"event_type"`
	OrgID     string                 `json:"org_id"`
	UserID    string                 `json:"user_id,omitempty"`
	DomainID  string                 `json:"domain_id,omitempty"`
	ThreadID  string                 `json:"thread_id,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

type Bus struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

func NewBus(pool *pgxpool.Pool, rdb *redis.Client) *Bus {
	return &Bus{pool: pool, rdb: rdb}
}

func (b *Bus) Publish(ctx context.Context, e Event) (int64, error) {
	if e.Payload == nil {
		e.Payload = map[string]interface{}{}
	}
	payloadJSON, err := json.Marshal(e.Payload)
	if err != nil {
		return 0, err
	}

	// Insert into events table (durable log for catchup)
	var id int64
	var dbErr error
	if b.pool != nil {
		dbErr = b.pool.QueryRow(ctx,
			`INSERT INTO events (event_type, org_id, user_id, domain_id, thread_id, payload)
			 VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6)
			 RETURNING id`,
			e.EventType, e.OrgID, e.UserID, e.DomainID, e.ThreadID, payloadJSON,
		).Scan(&id)
		if dbErr != nil {
			slog.Warn("event: postgres insert failed, continuing with redis",
				"error", dbErr, "event_type", e.EventType)
		}
	}

	// Publish to Redis for live delivery (independent of Postgres)
	msg, marshalErr := json.Marshal(map[string]interface{}{
		"id":         id,
		"event":      e.EventType,
		"org_id":     e.OrgID,
		"user_id":    e.UserID,
		"domain_id":  e.DomainID,
		"thread_id":  e.ThreadID,
		"payload":    e.Payload,
	})
	if marshalErr != nil {
		slog.Error("event: failed to marshal event for redis", "error", marshalErr, "event_type", e.EventType)
		return id, nil
	}
	if b.rdb != nil {
		if err := b.rdb.Publish(ctx, "ws:events", msg).Err(); err != nil {
			slog.Warn("event: redis publish failed", "error", err, "event_type", e.EventType)
		}
	}

	return id, dbErr
}
