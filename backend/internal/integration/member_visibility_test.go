//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inboxes/backend/internal/handler"
)

// A member sees only the threads of the aliases assigned to that member.
func TestThreadsList_MemberSeesOnlyAssignedAliases(t *testing.T) {
	ctx := context.Background()
	orgID, adminID := seedOrg(t, "member-vis-org", "member-vis-admin@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainA := seedDomain(t, orgID, "member-vis-a.test")
	domainB := seedDomain(t, orgID, "member-vis-b.test")
	aliasA := seedAlias(t, orgID, domainA, "hello@member-vis-a.test", "Hello")
	seedAlias(t, orgID, domainB, "info@member-vis-b.test", "Info")

	tA := seedThread(t, orgID, adminID, domainA, "For alias A")
	testStore.AddLabel(ctx, tA, orgID, "alias:hello@member-vis-a.test")
	tB := seedThread(t, orgID, adminID, domainB, "For alias B")
	testStore.AddLabel(ctx, tB, orgID, "alias:info@member-vis-b.test")
	seedThread(t, orgID, adminID, domainB, "No alias label")

	memberID := seedUser(t, orgID, "member-vis-member@test.io", "member")
	h := &handler.ThreadHandler{Store: testStore, Bus: nil}

	list := func(query string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/threads?label=inbox"+query, nil)
		req = withClaims(req, memberID, orgID, "member")
		w := httptest.NewRecorder()
		h.List(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Threads []map[string]any `json:"threads"`
		}
		parseJSON(t, w, &resp)
		return len(resp.Threads)
	}

	if n := list(""); n != 0 {
		t.Fatalf("member with no alias: expected 0 threads, got %d", n)
	}

	if err := testStore.AddAliasUser(ctx, aliasA, orgID, memberID, true); err != nil {
		t.Fatal(err)
	}
	if n := list(""); n != 1 {
		t.Fatalf("member with alias A: expected 1 thread, got %d", n)
	}
	if n := list("&domain_id=" + domainB); n != 0 {
		t.Fatalf("member with alias A in domain B: expected 0 threads, got %d", n)
	}

	// The member must not open a thread of a different alias.
	req := httptest.NewRequest(http.MethodGet, "/api/threads/"+tB, nil)
	req = withClaims(req, memberID, orgID, "member")
	req = withChiParam(req, "id", tB)
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("member opens thread of alias B: expected 404, got %d", w.Code)
	}
}

// A member sees only the domains in which the member has an assigned alias.
func TestDomainsList_MemberSeesOnlyAssignedDomains(t *testing.T) {
	ctx := context.Background()
	orgID, adminID := seedOrg(t, "member-dom-org", "member-dom-admin@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainA := seedDomain(t, orgID, "member-dom-a.test")
	seedDomain(t, orgID, "member-dom-b.test")
	aliasA := seedAlias(t, orgID, domainA, "hello@member-dom-a.test", "Hello")
	memberID := seedUser(t, orgID, "member-dom-member@test.io", "member")

	h := &handler.DomainHandler{Store: testStore}
	list := func(userID, role string, all bool) []map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
		req = withClaims(req, userID, orgID, role)
		w := httptest.NewRecorder()
		if all {
			h.ListAll(w, req)
		} else {
			h.List(w, req)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp []map[string]any
		parseJSON(t, w, &resp)
		return resp
	}

	if n := len(list(adminID, "admin", false)); n != 2 {
		t.Fatalf("admin: expected 2 domains, got %d", n)
	}
	if n := len(list(memberID, "member", false)); n != 0 {
		t.Fatalf("member with no alias: expected 0 domains, got %d", n)
	}
	if err := testStore.AddAliasUser(ctx, aliasA, orgID, memberID, true); err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		got := list(memberID, "member", all)
		if len(got) != 1 || got[0]["id"] != domainA {
			t.Fatalf("member with alias in domain A (all=%v): expected only domain A, got %v", all, got)
		}
	}
}
