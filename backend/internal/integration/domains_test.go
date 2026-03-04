//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

func TestListDomains(t *testing.T) {
	orgID, _ := seedOrg(t, "dom-list-org", "dom-list@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	seedDomain(t, orgID, "domainone-list.test")
	seedDomain(t, orgID, "domaintwo-list.test")

	ctx := context.Background()

	domains, err := testStore.ListDomains(ctx, orgID, true)
	if err != nil {
		t.Fatalf("ListDomains failed: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}

	// Verify domain data
	names := map[string]bool{}
	for _, d := range domains {
		name, ok := d["domain"].(string)
		if !ok {
			t.Fatalf("expected string domain name, got %T: %v", d["domain"], d["domain"])
		}
		names[name] = true
	}
	if !names["domainone-list.test"] || !names["domaintwo-list.test"] {
		t.Fatalf("expected both domains in results, got %v", names)
	}
}

func TestListDomainsHidden(t *testing.T) {
	orgID, _ := seedOrg(t, "dom-hidden-org", "dom-hidden@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	d1 := seedDomain(t, orgID, "visible-hidden.test")
	seedDomain(t, orgID, "hidden-hidden.test")

	ctx := context.Background()

	// Hide the second domain by making only d1 visible
	err := testStore.UpdateDomainVisibility(ctx, orgID, []string{d1})
	if err != nil {
		t.Fatalf("UpdateDomainVisibility failed: %v", err)
	}

	// With includeHidden=false, should only return the visible one
	domains, err := testStore.ListDomains(ctx, orgID, false)
	if err != nil {
		t.Fatalf("ListDomains failed: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("expected 1 visible domain, got %d", len(domains))
	}
	name, _ := domains[0]["domain"].(string)
	if name != "visible-hidden.test" {
		t.Fatalf("expected 'visible-hidden.test', got '%s'", name)
	}

	// With includeHidden=true, should return both
	allDomains, err := testStore.ListDomains(ctx, orgID, true)
	if err != nil {
		t.Fatalf("ListDomains (includeHidden) failed: %v", err)
	}
	if len(allDomains) != 2 {
		t.Fatalf("expected 2 domains with includeHidden, got %d", len(allDomains))
	}
}

func TestSoftDeleteDomain(t *testing.T) {
	orgID, _ := seedOrg(t, "dom-delete-org", "dom-delete@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, "deletable-domain.test")

	ctx := context.Background()

	n, err := testStore.SoftDeleteDomain(ctx, domainID, orgID)
	if err != nil {
		t.Fatalf("SoftDeleteDomain failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}

	// Verify status changed to "deleted"
	status, err := testStore.GetDomainStatus(ctx, domainID, orgID)
	if err != nil {
		t.Fatalf("GetDomainStatus failed: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("expected status 'deleted', got '%s'", status)
	}
}

func TestReorderDomains(t *testing.T) {
	orgID, _ := seedOrg(t, "dom-reorder-org", "dom-reorder@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	d1 := seedDomain(t, orgID, "first-reorder.test")
	d2 := seedDomain(t, orgID, "second-reorder.test")
	d3 := seedDomain(t, orgID, "third-reorder.test")

	ctx := context.Background()

	// Reorder: third first, first second, second third
	err := testStore.ReorderDomains(ctx, orgID, []store.DomainOrder{
		{ID: d3, Order: 0},
		{ID: d1, Order: 1},
		{ID: d2, Order: 2},
	})
	if err != nil {
		t.Fatalf("ReorderDomains failed: %v", err)
	}

	// List domains and verify order
	domains, err := testStore.ListDomains(ctx, orgID, true)
	if err != nil {
		t.Fatalf("ListDomains failed: %v", err)
	}
	if len(domains) != 3 {
		t.Fatalf("expected 3 domains, got %d", len(domains))
	}

	// Domains are ordered by display_order, then created_at
	expectedOrder := []string{"third-reorder.test", "first-reorder.test", "second-reorder.test"}
	for i, d := range domains {
		name, _ := d["domain"].(string)
		if name != expectedOrder[i] {
			t.Fatalf("position %d: expected '%s', got '%s'", i, expectedOrder[i], name)
		}
	}
}

func TestUnreadCounts(t *testing.T) {
	orgID, userID := seedOrg(t, "dom-unread-org", "dom-unread@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID1 := seedDomain(t, orgID, "unread1-domain.test")
	domainID2 := seedDomain(t, orgID, "unread2-domain.test")

	ctx := context.Background()

	// Create threads with unread counts in different domains
	t1 := seedThread(t, orgID, userID, domainID1, "Unread Thread 1")
	t2 := seedThread(t, orgID, userID, domainID1, "Unread Thread 2")
	t3 := seedThread(t, orgID, userID, domainID2, "Unread Thread 3")

	testStore.UpdateThreadUnread(ctx, t1, orgID, 2)
	testStore.UpdateThreadUnread(ctx, t2, orgID, 1)
	testStore.UpdateThreadUnread(ctx, t3, orgID, 3)

	counts, err := testStore.GetUnreadCounts(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("GetUnreadCounts failed: %v", err)
	}

	if counts[domainID1] != 3 {
		t.Fatalf("expected unread count 3 for domain1, got %d", counts[domainID1])
	}
	if counts[domainID2] != 3 {
		t.Fatalf("expected unread count 3 for domain2, got %d", counts[domainID2])
	}
}

func TestUpdateDomainVisibilityEmptyList(t *testing.T) {
	orgID, _ := seedOrg(t, "dom-vis-empty-org", "dom-vis-empty@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	seedDomain(t, orgID, "vis-empty-one.test")
	seedDomain(t, orgID, "vis-empty-two.test")

	ctx := context.Background()

	// Pass an empty list of visible IDs -- this should hide ALL domains
	err := testStore.UpdateDomainVisibility(ctx, orgID, []string{})
	if err != nil {
		t.Fatalf("UpdateDomainVisibility(empty): %v", err)
	}

	// With includeHidden=false, should return 0 domains (all hidden)
	visible, err := testStore.ListDomains(ctx, orgID, false)
	if err != nil {
		t.Fatalf("ListDomains(visible only): %v", err)
	}
	if len(visible) != 0 {
		t.Errorf("expected 0 visible domains, got %d", len(visible))
	}

	// With includeHidden=true, should return all 2 domains
	all, err := testStore.ListDomains(ctx, orgID, true)
	if err != nil {
		t.Fatalf("ListDomains(includeHidden): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 total domains, got %d", len(all))
	}
}
