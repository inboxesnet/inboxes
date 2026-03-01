package ws

import (
	"testing"
	"time"
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
