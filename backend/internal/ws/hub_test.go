package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewHub_DefaultMaxConns(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 0, 0)
	if h.maxConnsPerUser != 5 {
		t.Errorf("maxConnsPerUser: got %d, want 5", h.maxConnsPerUser)
	}
}

func TestNewHub_CustomMaxConns(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 10, 0)
	if h.maxConnsPerUser != 10 {
		t.Errorf("maxConnsPerUser: got %d, want 10", h.maxConnsPerUser)
	}
}

func TestNewHub_DefaultTokenCheckInterval(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 0, 0)
	if h.tokenCheckInterval != 1*time.Minute {
		t.Errorf("tokenCheckInterval: got %v, want %v", h.tokenCheckInterval, 1*time.Minute)
	}
}

func TestNewHub_CustomTokenCheckInterval(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 0, 30*time.Second)
	if h.tokenCheckInterval != 30*time.Second {
		t.Errorf("tokenCheckInterval: got %v, want %v", h.tokenCheckInterval, 30*time.Second)
	}
}

func TestSendToOrgFiltered_AdminAlwaysReceives(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	adminClient := &Client{
		UserID:         "admin1",
		OrgID:          "org1",
		Role:           "admin",
		send:           make(chan []byte, 10),
		AliasAddresses: nil,
	}
	h.clients["admin1"] = map[*Client]bool{adminClient: true}

	h.sendToOrgFiltered("org1", []string{"alias:user@test.com"}, `{"event":"test"}`)

	select {
	case msg := <-adminClient.send:
		if string(msg) != `{"event":"test"}` {
			t.Errorf("admin got %q", msg)
		}
	default:
		t.Error("admin should receive message")
	}
}

func TestSendToOrgFiltered_MemberWithMatchingAlias(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	memberClient := &Client{
		UserID:         "member1",
		OrgID:          "org1",
		Role:           "member",
		send:           make(chan []byte, 10),
		AliasAddresses: []string{"user@test.com"},
	}
	h.clients["member1"] = map[*Client]bool{memberClient: true}

	h.sendToOrgFiltered("org1", []string{"alias:user@test.com"}, `{"event":"test"}`)

	select {
	case <-memberClient.send:
		// ok
	default:
		t.Error("member with matching alias should receive message")
	}
}

func TestSendToOrgFiltered_MemberWithoutAlias(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	memberClient := &Client{
		UserID:         "member1",
		OrgID:          "org1",
		Role:           "member",
		send:           make(chan []byte, 10),
		AliasAddresses: []string{"other@test.com"},
	}
	h.clients["member1"] = map[*Client]bool{memberClient: true}

	h.sendToOrgFiltered("org1", []string{"alias:user@test.com"}, `{"event":"test"}`)

	select {
	case <-memberClient.send:
		t.Error("member without matching alias should NOT receive message")
	default:
		// ok
	}
}

func TestSendToOrgFiltered_DifferentOrgExcluded(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	otherClient := &Client{
		UserID:         "user1",
		OrgID:          "org2",
		Role:           "admin",
		send:           make(chan []byte, 10),
		AliasAddresses: nil,
	}
	h.clients["user1"] = map[*Client]bool{otherClient: true}

	h.sendToOrgFiltered("org1", []string{"alias:user@test.com"}, `{"event":"test"}`)

	select {
	case <-otherClient.send:
		t.Error("client in different org should NOT receive message")
	default:
		// ok
	}
}

func TestSendToOrgFiltered_AliasLabelParsing(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	memberClient := &Client{
		UserID:         "member1",
		OrgID:          "org1",
		Role:           "member",
		send:           make(chan []byte, 10),
		AliasAddresses: []string{"info@company.com"},
	}
	h.clients["member1"] = map[*Client]bool{memberClient: true}

	// alias labels are in "alias:addr" format
	h.sendToOrgFiltered("org1", []string{"alias:info@company.com", "alias:support@company.com"}, `{"event":"test"}`)

	select {
	case <-memberClient.send:
		// ok - info@company.com matches
	default:
		t.Error("member with matching alias label should receive message")
	}
}

func TestSendToOrg_AllClientsInOrg(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	c1 := &Client{UserID: "u1", OrgID: "org1", Role: "admin", send: make(chan []byte, 10)}
	c2 := &Client{UserID: "u2", OrgID: "org1", Role: "member", send: make(chan []byte, 10)}
	h.clients["u1"] = map[*Client]bool{c1: true}
	h.clients["u2"] = map[*Client]bool{c2: true}

	h.sendToOrg("org1", `{"event":"broadcast"}`)

	for _, c := range []*Client{c1, c2} {
		select {
		case <-c.send:
		default:
			t.Errorf("client %s should receive broadcast", c.UserID)
		}
	}
}

func TestSendToOrg_OtherOrgExcluded(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	c1 := &Client{UserID: "u1", OrgID: "org1", Role: "admin", send: make(chan []byte, 10)}
	c2 := &Client{UserID: "u2", OrgID: "org2", Role: "admin", send: make(chan []byte, 10)}
	h.clients["u1"] = map[*Client]bool{c1: true}
	h.clients["u2"] = map[*Client]bool{c2: true}

	h.sendToOrg("org1", `{"event":"test"}`)

	select {
	case <-c2.send:
		t.Error("org2 client should NOT receive org1 message")
	default:
		// ok
	}
}

func TestDisconnectUser_NoPanicWithNoClients(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	// Should not panic for non-existent user
	h.disconnectUser("nonexistent")
}

// newTestWSConn creates a real websocket.Conn via httptest for use in tests
// that exercise code paths calling conn.WriteMessage (e.g., eviction).
// Returns the server-side conn and a cleanup function.
func newTestWSConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	serverConnCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	serverConn := <-serverConnCh
	cleanup := func() {
		clientConn.Close()
		serverConn.Close()
		srv.Close()
	}
	return serverConn, cleanup
}

func TestRegisterAndUnregister(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, 5, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run hub in background
	go h.Run(ctx)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	client := &Client{
		UserID:   "user-1",
		OrgID:    "org-1",
		Role:     "admin",
		send:     make(chan []byte, 10),
		conn:     conn,
		IssuedAt: time.Now(),
	}

	// Register
	h.register <- client
	// Give the Run loop time to process
	time.Sleep(50 * time.Millisecond)

	h.mu.RLock()
	clients, ok := h.clients["user-1"]
	h.mu.RUnlock()
	if !ok {
		t.Fatal("expected user-1 in clients map after register")
	}
	if !clients[client] {
		t.Fatal("expected client to be in clients set")
	}

	// Unregister
	h.unregister <- client
	time.Sleep(50 * time.Millisecond)

	h.mu.RLock()
	_, ok = h.clients["user-1"]
	h.mu.RUnlock()
	if ok {
		t.Error("expected user-1 removed from clients map after unregister (was only client)")
	}
}

func TestMaxConnsPerUser_Eviction(t *testing.T) {
	t.Parallel()
	maxConns := 5
	h := NewHub(nil, nil, maxConns, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Run(ctx)

	// Create 6 clients for the same user. Each needs a real websocket conn
	// because the eviction path calls conn.WriteMessage.
	type testClient struct {
		client  *Client
		cleanup func()
	}
	clients := make([]testClient, 6)
	for i := 0; i < 6; i++ {
		conn, cleanup := newTestWSConn(t)
		c := &Client{
			UserID:   "user-flood",
			OrgID:    "org-1",
			Role:     "member",
			send:     make(chan []byte, 10),
			conn:     conn,
			IssuedAt: time.Now().Add(time.Duration(i) * time.Second), // oldest = index 0
		}
		clients[i] = testClient{client: c, cleanup: cleanup}
	}
	defer func() {
		for _, tc := range clients {
			tc.cleanup()
		}
	}()

	// Register all 6 through the hub's Run loop
	for _, tc := range clients {
		h.register <- tc.client
		time.Sleep(20 * time.Millisecond) // let the Run loop process each one
	}

	// Give the Run loop time to finish the last registration + eviction
	time.Sleep(100 * time.Millisecond)

	// The oldest client (index 0) should have been evicted (send channel closed)
	select {
	case _, ok := <-clients[0].client.send:
		if ok {
			t.Error("expected oldest client's send channel to be closed, but got a message")
		}
		// Channel closed = evicted, this is the expected outcome
	default:
		// Channel still open and empty -- check if client was removed from the map
		h.mu.RLock()
		_, stillPresent := h.clients["user-flood"][clients[0].client]
		h.mu.RUnlock()
		if stillPresent {
			t.Error("expected oldest client to be evicted from clients map")
		}
	}

	// Verify there are exactly maxConns clients remaining
	h.mu.RLock()
	remaining := len(h.clients["user-flood"])
	h.mu.RUnlock()
	if remaining != maxConns {
		t.Errorf("expected %d clients remaining, got %d", maxConns, remaining)
	}

	// Verify clients 1-5 are still in the map
	h.mu.RLock()
	for i := 1; i < 6; i++ {
		if !h.clients["user-flood"][clients[i].client] {
			t.Errorf("expected client %d to still be in the map", i)
		}
	}
	h.mu.RUnlock()
}
