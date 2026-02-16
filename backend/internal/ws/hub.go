package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Hub struct {
	clients    map[string]map[*Client]bool // userID -> set of clients
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	rdb        *redis.Client
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

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		rdb:        rdb,
	}
}

func (h *Hub) Run(ctx context.Context) {
	go h.subscribeRedis(ctx)

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.UserID]; !ok {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()
			slog.Info("ws: client connected", "user_id", client.UserID)

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
			frontendMsg, _ := json.Marshal(map[string]interface{}{
				"event":     evt.Event,
				"thread_id": evt.ThreadID,
				"domain_id": evt.DomainID,
				"payload":   evt.Payload,
			})

			// Route to all clients in the same org
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

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}
