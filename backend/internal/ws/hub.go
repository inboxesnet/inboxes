package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

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
			h.addClient(client)

		case client := <-h.unregister:
			h.mu.Lock()
			removed := h.dropLocked(client, 0, "")
			h.mu.Unlock()
			if removed {
				slog.Info("ws: client disconnected", "user_id", client.UserID)
			}

		case <-ctx.Done():
			return
		}
	}
}

// Close codes that the hub sends when it removes a client.
const (
	closeConnLimit      = 4001
	closeSessionRevoked = 4002
	closeSlowClient     = 4003
)

// dropLocked removes a client from the map and closes its send channel.
// The caller must hold h.mu for writing. A second call for the same client
// does nothing, so the channel cannot be closed twice. The write pump sends
// the close frame; only that goroutine writes to the connection.
func (h *Hub) dropLocked(c *Client, code int, reason string) bool {
	clients, ok := h.clients[c.UserID]
	if !ok || !clients[c] {
		return false
	}
	delete(clients, c)
	if len(clients) == 0 {
		delete(h.clients, c.UserID)
	}
	c.closeCode = code
	c.closeReason = reason
	close(c.send)
	return true
}

// addClient registers a client. It evicts the oldest connection of the
// user when the user is at the connection limit.
func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.clients[client.UserID]) >= h.maxConnsPerUser {
		var oldest *Client
		for c := range h.clients[client.UserID] {
			if oldest == nil || c.IssuedAt.Before(oldest.IssuedAt) {
				oldest = c
			}
		}
		if oldest != nil && h.dropLocked(oldest, closeConnLimit, "connection limit exceeded") {
			slog.Info("ws: evicted oldest connection", "user_id", client.UserID, "limit", h.maxConnsPerUser)
		}
	}
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]bool)
	}
	h.clients[client.UserID][client] = true
	slog.Info("ws: client connected", "user_id", client.UserID, "connections", len(h.clients[client.UserID]))
}

// deliver queues a message for a client. It returns false when the send
// buffer of the client is full.
func deliver(c *Client, message string) bool {
	select {
	case c.send <- []byte(message):
		return true
	default:
		return false
	}
}

// dropSlow disconnects clients whose send buffer was full. A dropped
// message is not sent again, so the client must reconnect and resync.
func (h *Hub) dropSlow(slow []*Client) {
	if len(slow) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range slow {
		if h.dropLocked(c, closeSlowClient, "send buffer full") {
			slog.Warn("ws: disconnected slow client", "user_id", c.UserID)
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
	var slow []*Client
	h.mu.RLock()
	for _, clients := range h.clients {
		for client := range clients {
			if client.OrgID == orgID && !deliver(client, message) {
				slow = append(slow, client)
			}
		}
	}
	h.mu.RUnlock()
	h.dropSlow(slow)
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

	var slow []*Client
	h.mu.RLock()
	for _, clients := range h.clients {
		for client := range clients {
			if client.OrgID != orgID {
				continue
			}
			// Admins always receive all events
			if client.Role == "admin" {
				if !deliver(client, message) {
					slow = append(slow, client)
				}
				continue
			}
			// Non-admins: check if any of their alias addresses match
			for _, addr := range client.aliasAddresses() {
				if addrSet[addr] {
					if !deliver(client, message) {
						slow = append(slow, client)
					}
					break
				}
			}
		}
	}
	h.mu.RUnlock()
	h.dropSlow(slow)
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
	defer h.mu.Unlock()

	n := 0
	for c := range h.clients[userID] {
		if h.dropLocked(c, closeSessionRevoked, "session revoked") {
			n++
		}
	}
	if n > 0 {
		slog.Info("ws: force-disconnected user", "user_id", userID, "connections", n)
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}
