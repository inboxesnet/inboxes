//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEmailInsertAndRetrieve(t *testing.T) {
	orgID, userID := seedOrg(t, "email-insert-org", "email-insert@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "email-insert.test")
	threadID := seedThread(t, orgID, userID, domainID, "Email Insert Test")

	ctx := context.Background()

	toJSON, _ := json.Marshal([]string{"recipient@example.com"})
	ccJSON, _ := json.Marshal([]string{})
	bccJSON, _ := json.Marshal([]string{})
	refsJSON, _ := json.Marshal([]string{})

	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"inbound", "sender@example.com", toJSON, ccJSON, bccJSON,
		"Test Subject", "<p>Hello</p>", "Hello", "received", "", refsJSON)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}
	if emailID == "" {
		t.Fatal("expected non-empty email ID")
	}

	// Retrieve via GetThreadEmails
	emails, err := testStore.GetThreadEmails(ctx, threadID, orgID)
	if err != nil {
		t.Fatalf("GetThreadEmails failed: %v", err)
	}

	found := false
	for _, e := range emails {
		if e["id"] == emailID {
			found = true
			if e["from_address"] != "sender@example.com" {
				t.Fatalf("expected from_address 'sender@example.com', got %v", e["from_address"])
			}
			if e["subject"] != "Test Subject" {
				t.Fatalf("expected subject 'Test Subject', got %v", e["subject"])
			}
			if e["direction"] != "inbound" {
				t.Fatalf("expected direction 'inbound', got %v", e["direction"])
			}
			if e["body_html"] != "<p>Hello</p>" {
				t.Fatalf("expected body_html '<p>Hello</p>', got %v", e["body_html"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("email %s not found in thread emails", emailID)
	}
}

func TestEmailSearchByKeyword(t *testing.T) {
	orgID, userID := seedOrg(t, "email-search-org", "email-search@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "email-search.test")
	threadID := seedThread(t, orgID, userID, domainID, "Payment Notification")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "billing@example.com", "Payment Notification")

	ctx := context.Background()

	results, err := testStore.SearchEmails(ctx, orgID, "payment", "", "admin", nil)
	if err != nil {
		t.Fatalf("SearchEmails failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result for 'payment'")
	}

	found := false
	for _, r := range results {
		if r["id"] == threadID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected thread %s in search results for 'payment'", threadID)
	}
}

func TestEmailSearchEmpty(t *testing.T) {
	orgID, userID := seedOrg(t, "email-empty-org", "email-empty@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "email-empty.test")
	threadID := seedThread(t, orgID, userID, domainID, "Irrelevant Thread")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "nobody@example.com", "Irrelevant Thread")

	ctx := context.Background()

	results, err := testStore.SearchEmails(ctx, orgID, "xyzzynonexistent99", "", "admin", nil)
	if err != nil {
		t.Fatalf("SearchEmails failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 search results, got %d", len(results))
	}
}

func TestCheckBouncedRecipients(t *testing.T) {
	orgID, _ := seedOrg(t, "bounce-check-org", "bounce-check@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	// Insert a bounce
	err := testStore.InsertBounce(ctx, orgID, "bounced@example.com", "hard")
	if err != nil {
		t.Fatalf("InsertBounce failed: %v", err)
	}

	// Check bounced recipients
	blocked, err := testStore.CheckBouncedRecipients(ctx, orgID, []string{"bounced@example.com", "ok@example.com"})
	if err != nil {
		t.Fatalf("CheckBouncedRecipients failed: %v", err)
	}
	if len(blocked) != 1 {
		t.Fatalf("expected 1 blocked address, got %d", len(blocked))
	}
	if blocked[0] != "bounced@example.com" {
		t.Fatalf("expected 'bounced@example.com', got '%s'", blocked[0])
	}
}

func TestClearBounce(t *testing.T) {
	orgID, _ := seedOrg(t, "bounce-clear-org", "bounce-clear@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	// Insert bounce
	err := testStore.InsertBounce(ctx, orgID, "cleared@example.com", "soft")
	if err != nil {
		t.Fatalf("InsertBounce failed: %v", err)
	}

	// Clear bounce
	err = testStore.ClearBounce(ctx, orgID, "cleared@example.com")
	if err != nil {
		t.Fatalf("ClearBounce failed: %v", err)
	}

	// Verify it's gone
	blocked, err := testStore.CheckBouncedRecipients(ctx, orgID, []string{"cleared@example.com"})
	if err != nil {
		t.Fatalf("CheckBouncedRecipients failed: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("expected 0 blocked after clear, got %d", len(blocked))
	}
}

func TestCanSendAsAlias(t *testing.T) {
	orgID, userID := seedOrg(t, "cansend-org", "cansend@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "cansend.test")

	ctx := context.Background()

	// Create a member user
	var memberID string
	err := testPool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, role, status) VALUES ($1, $2, $3, 'member', 'active') RETURNING id`,
		orgID, "member-cansend@test.io", "Member",
	).Scan(&memberID)
	if err != nil {
		t.Fatalf("create member failed: %v", err)
	}

	// Create alias and assign member with can_send_as=true
	aliasID := seedAlias(t, orgID, domainID, "team@cansend.test", "Team Alias")
	err = testStore.AddAliasUser(ctx, aliasID, orgID, memberID, true)
	if err != nil {
		t.Fatalf("AddAliasUser failed: %v", err)
	}

	// Member should be able to send as this alias
	canSend, err := testStore.CanSendAs(ctx, memberID, orgID, "team@cansend.test", "member")
	if err != nil {
		t.Fatalf("CanSendAs failed: %v", err)
	}
	if !canSend {
		t.Fatal("expected CanSendAs=true for assigned alias")
	}

	// Admin can always send
	canSendAdmin, err := testStore.CanSendAs(ctx, userID, orgID, "team@cansend.test", "admin")
	if err != nil {
		t.Fatalf("CanSendAs admin failed: %v", err)
	}
	if !canSendAdmin {
		t.Fatal("expected admin CanSendAs=true")
	}
}

func TestCannotSendAsUnassignedAlias(t *testing.T) {
	orgID, _ := seedOrg(t, "nosend-org", "nosend@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "nosend.test")

	ctx := context.Background()

	// Create a member user
	var memberID string
	err := testPool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, role, status) VALUES ($1, $2, $3, 'member', 'active') RETURNING id`,
		orgID, "member-nosend@test.io", "Member",
	).Scan(&memberID)
	if err != nil {
		t.Fatalf("create member failed: %v", err)
	}

	// Create alias but do NOT assign it to member
	seedAlias(t, orgID, domainID, "private@nosend.test", "Private")

	// Member should NOT be able to send as this alias
	canSend, err := testStore.CanSendAs(ctx, memberID, orgID, "private@nosend.test", "member")
	if err != nil {
		t.Fatalf("CanSendAs failed: %v", err)
	}
	if canSend {
		t.Fatal("expected CanSendAs=false for unassigned alias")
	}
}
