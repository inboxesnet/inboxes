package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/redis/go-redis/v9"
)

type Hub struct {
	ctx                context.Context
	clients            map[string]map[*Client]bool // userID -> set of clients
	register           chan *Client
	unregister         chan *Client
	mu                 sync.RWMutex
	rdb                *redis.Client
	store              store.Store
	maxConnsPerUser    int
	tokenCheckInterval time.Duration
}

type eventMessage struct {
	ID       int64           `json:"id"`
	Event    string          `json:"event"`
	OrgID    string          `json:"org_id"`
	UserID   string          `json:"user_id"`
	DomainID string          `json:"domain_id"`
	ThreadID string          `json:"thread_id"`
	Payload  json.RawMessage `json:"payload"`
}

func NewHub(rdb *redis.Client, st store.Store, maxConnsPerUser int, tokenCheckInterval time.Duration) *Hub {
	if maxConnsPerUser <= 0 {
		maxConnsPerUser = 5
	}
	if tokenCheckInterval <= 0 {
		tokenCheckInterval = 1 * time.Minute
	}
	return &Hub{
		ctx:                context.Background(),
		clients:            make(map[string]map[*Client]bool),
		register:           make(chan *Client),
		unregister:         make(chan *Client),
		rdb:                rdb,
		store:              st,
		maxConnsPerUser:    maxConnsPerUser,
		tokenCheckInterval: tokenCheckInterval,
	}
}

func (h *Hub) Run(ctx context.Context) {
	h.ctx = ctx
	util.SafeGo("ws-subscribe-redis", func() { h.subscribeRedis(ctx) })
	util.SafeGo("ws-disconnect-listener", func() { h.subscribeDisconnect(ctx) })

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.UserID]; !ok {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			// Enforce per-user connection limit — evict oldest by IssuedAt
			if len(h.clients[client.UserID]) >= h.maxConnsPerUser {
				var oldest *Client
				for c := range h.clients[client.UserID] {
					if oldest == nil || c.IssuedAt.Before(oldest.IssuedAt) {
						oldest = c
					}
				}
				if oldest != nil {
					oldest.conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(4001, "connection limit exceeded"))
					delete(h.clients[client.UserID], oldest)
					close(oldest.send)
					slog.Info("ws: evicted oldest connection", "user_id", client.UserID, "limit", h.maxConnsPerUser)
				}
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()
			slog.Info("ws: client connected", "user_id", client.UserID, "connections", len(h.clients[client.UserID]))

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.UserID]; ok {
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.clients, client.UserID)
				}
			}
			close(client.send)
			h.mu.Unlock()
			slog.Info("ws: client disconnected", "user_id", client.UserID)

		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) subscribeRedis(ctx context.Context) {
	sub := h.rdb.Subscribe(ctx, "ws:events")
	defer sub.Close()

	// Events that should always be sent to all org members (no alias filtering)
	broadcastEvents := map[string]bool{
		"sync.completed":    true,
		"plan.changed":      true,
		"thread.bulk_action": true,
	}

	ch := sub.Channel()
	for {
		select {
		case msg := <-ch:
			var evt eventMessage
			if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
				slog.Error("ws: parse redis event", "error", err)
				continue
			}

			// Build frontend-friendly message
			frontendMsg, marshalErr := json.Marshal(map[string]interface{}{
				"id":        evt.ID,
				"event":     evt.Event,
				"thread_id": evt.ThreadID,
				"domain_id": evt.DomainID,
				"payload":   evt.Payload,
			})
			if marshalErr != nil {
				slog.Error("ws: failed to marshal frontend message", "error", marshalErr)
				continue
			}

			// For thread-specific events, filter by alias permissions
			if evt.ThreadID != "" && !broadcastEvents[evt.Event] {
				aliasLabels, err := h.getThreadAliasLabels(ctx, evt.ThreadID)
				if err != nil {
					slog.Error("ws: alias filter query failed, dropping event",
						"thread_id", evt.ThreadID, "event", evt.Event, "error", err)
					continue // Fail-closed: drop event rather than broadcast to all
				}
				if len(aliasLabels) > 0 {
					h.sendToOrgFiltered(evt.OrgID, aliasLabels, string(frontendMsg))
					continue
				}
			}

			// Broadcast events or threads without alias labels go to all
			h.sendToOrg(evt.OrgID, string(frontendMsg))

		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) sendToOrg(orgID string, message string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.clients {
		for client := range clients {
			if client.OrgID == orgID {
				select {
				case client.send <- []byte(message):
				default:
					// Buffer full, skip
				}
			}
		}
	}
}

// getThreadAliasLabels returns the alias labels (e.g. "alias:user@domain.com")
// associated with a thread. Used to determine which users should receive
// real-time updates for thread-specific events.
func (h *Hub) getThreadAliasLabels(ctx context.Context, threadID string) ([]string, error) {
	rows, err := h.store.Q().Query(ctx,
		`SELECT label FROM thread_labels WHERE thread_id = $1 AND label LIKE 'alias:%'`,
		threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// sendToOrgFiltered sends a message to clients in an org, but only to admins
// and to non-admin members whose alias addresses match the thread's alias labels.
func (h *Hub) sendToOrgFiltered(orgID string, aliasLabels []string, message string) {
	// Extract addresses from "alias:addr" labels
	addrSet := make(map[string]bool, len(aliasLabels))
	for _, label := range aliasLabels {
		if addr := strings.TrimPrefix(label, "alias:"); addr != label {
			addrSet[addr] = true
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.clients {
		for client := range clients {
			if client.OrgID != orgID {
				continue
			}
			// Admins always receive all events
			if client.Role == "admin" {
				select {
				case client.send <- []byte(message):
				default:
				}
				continue
			}
			// Non-admins: check if any of their alias addresses match
			for _, addr := range client.AliasAddresses {
				if addrSet[addr] {
					select {
					case client.send <- []byte(message):
					default:
					}
					break
				}
			}
		}
	}
}

// subscribeDisconnect listens for push-based user disconnection events via Redis.
// When a token is revoked (logout, disable, password change, org delete), the
// revoking handler publishes the user ID to ws:disconnect for immediate eviction.
func (h *Hub) subscribeDisconnect(ctx context.Context) {
	sub := h.rdb.Subscribe(ctx, "ws:disconnect")
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case msg := <-ch:
			userID := msg.Payload
			h.disconnectUser(userID)
		case <-ctx.Done():
			return
		}
	}
}

// disconnectUser closes all WebSocket connections for a user immediately.
func (h *Hub) disconnectUser(userID string) {
	h.mu.Lock()
	clients, ok := h.clients[userID]
	if !ok || len(clients) == 0 {
		h.mu.Unlock()
		return
	}
	// Collect clients to close (avoid modifying map while iterating under lock)
	toClose := make([]*Client, 0, len(clients))
	for c := range clients {
		toClose = append(toClose, c)
	}
	delete(h.clients, userID)
	h.mu.Unlock()

	for _, c := range toClose {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4002, "session revoked"))
		close(c.send)
	}
	slog.Info("ws: force-disconnected user", "user_id", userID, "connections", len(toClose))
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}
