package ws

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	maxMsgSize = 512
)

type Client struct {
	hub             *Hub
	conn            *websocket.Conn
	send            chan []byte
	UserID          string
	OrgID           string
	Role            string
	JTI             string
	IssuedAt        time.Time
	TokenExp        time.Time
	AliasAddresses  []string
}

func ServeWS(hub *Hub, secret string, appURL string, w http.ResponseWriter, r *http.Request) {
	// Auth from cookie
	cookie, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Per-call upgrader that validates WebSocket origin against configured app URL
	allowedHost := ""
	if parsed, err := url.Parse(appURL); err == nil {
		allowedHost = parsed.Host
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // Non-browser clients
			}
			parsed, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return parsed.Host == allowedHost
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "error", err)
		return
	}

	issuedAt := time.Time{}
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	tokenExp := time.Time{}
	if claims.ExpiresAt != nil {
		tokenExp = claims.ExpiresAt.Time
	}

	// Load alias addresses for non-admin filtering
	var aliasAddrs []string
	if hub.pool != nil {
		aliasAddrs = loadUserAliases(hub.pool, claims.UserID)
	}

	client := &Client{
		hub:            hub,
		conn:           conn,
		send:           make(chan []byte, 256),
		UserID:         claims.UserID,
		OrgID:          claims.OrgID,
		Role:           claims.Role,
		JTI:            claims.ID,
		IssuedAt:       issuedAt,
		TokenExp:       tokenExp,
		AliasAddresses: aliasAddrs,
	}
	hub.Register(client)

	util.SafeGo("ws-write-pump", client.writePump)
	util.SafeGo("ws-read-pump", client.readPump)
}

// loadUserAliases queries the DB for all alias addresses assigned to a user.
func loadUserAliases(pool *pgxpool.Pool, userID string) []string {
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()
	rows, err := pool.Query(queryCtx,
		`SELECT a.address FROM aliases a JOIN alias_users au ON au.alias_id = a.id WHERE au.user_id = $1`,
		userID)
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

func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	validationTicker := time.NewTicker(c.hub.tokenCheckInterval)
	defer func() {
		ticker.Stop()
		validationTicker.Stop()
		c.conn.Close()
	}()

	blacklist := service.NewTokenBlacklist(c.hub.rdb)

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-validationTicker.C:
			// Check if token has expired
			if time.Now().After(c.TokenExp) {
				slog.Info("ws: closing connection — token expired", "user_id", c.UserID)
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// Check if token has been revoked
			revokeCtx, revokeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			revoked := blacklist.IsRevoked(revokeCtx, c.JTI, c.UserID, c.IssuedAt)
			revokeCancel()
			if revoked {
				slog.Info("ws: closing connection — token revoked", "user_id", c.UserID)
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// Refresh alias addresses for event filtering
			if c.hub.pool != nil {
				c.AliasAddresses = loadUserAliases(c.hub.pool, c.UserID)
			}
		}
	}
}
