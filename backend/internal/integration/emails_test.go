//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inboxes/backend/internal/handler"
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

	results, _, err := testStore.SearchEmails(ctx, orgID, "payment", "", "", "admin", nil, 1, 50)
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

	results, _, err := testStore.SearchEmails(ctx, orgID, "xyzzynonexistent99", "", "", "admin", nil, 1, 50)
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

func TestCancelSend_UndoWithinWindow(t *testing.T) {
	orgID, userID := seedOrg(t, "undo-org", "undo@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "undo.test")
	threadID := seedThread(t, orgID, userID, domainID, "Undo Me")

	ctx := context.Background()
	toJSON := []byte(`["dest@example.com"]`)
	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "me@undo.test", toJSON, []byte(`[]`), []byte(`[]`),
		"Undo Me", "<p>body</p>", "body", "queued", "", nil)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}
	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("CreateEmailJob failed: %v", err)
	}

	h := &handler.EmailHandler{Store: testStore, RDB: testRDB, Bus: testBus()}
	req := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailID+"/cancel-send", nil)
	req = withChiParam(req, "id", emailID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.CancelSend(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel-send: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Status  string `json:"status"`
		DraftID string `json:"draft_id"`
	}
	parseJSON(t, w, &resp)
	if resp.Status != "cancelled" {
		t.Errorf("expected status cancelled, got %q", resp.Status)
	}
	if resp.DraftID == "" {
		t.Error("expected a recovery draft id for a direct send")
	}

	// The job must be cancelled and the email row gone.
	var jobStatus string
	if err := testPool.QueryRow(ctx, "SELECT status FROM email_jobs WHERE id = $1", jobID).Scan(&jobStatus); err != nil {
		t.Fatalf("job lookup: %v", err)
	}
	if jobStatus != "cancelled" {
		t.Errorf("expected job cancelled, got %q", jobStatus)
	}
	var emailCount int
	testPool.QueryRow(ctx, "SELECT count(*) FROM emails WHERE id = $1", emailID).Scan(&emailCount)
	if emailCount != 0 {
		t.Error("expected queued email deleted after undo")
	}

	// The draft must carry the original content.
	var draftSubject string
	if err := testPool.QueryRow(ctx, "SELECT subject FROM drafts WHERE id = $1", resp.DraftID).Scan(&draftSubject); err != nil {
		t.Fatalf("draft lookup: %v", err)
	}
	if draftSubject != "Undo Me" {
		t.Errorf("expected draft subject 'Undo Me', got %q", draftSubject)
	}
}

func TestCancelSend_TooLateWhenRunning(t *testing.T) {
	orgID, userID := seedOrg(t, "undo-late-org", "undo-late@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "undo-late.test")
	threadID := seedThread(t, orgID, userID, domainID, "Too Late")

	ctx := context.Background()
	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "me@undo-late.test", []byte(`["dest@example.com"]`), []byte(`[]`), []byte(`[]`),
		"Too Late", "<p>body</p>", "body", "queued", "", nil)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}
	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("CreateEmailJob failed: %v", err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE email_jobs SET status='running' WHERE id = $1", jobID); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	h := &handler.EmailHandler{Store: testStore, RDB: testRDB, Bus: testBus()}
	req := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailID+"/cancel-send", nil)
	req = withChiParam(req, "id", emailID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.CancelSend(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a running job, got %d: %s", w.Code, w.Body.String())
	}

	// The email must survive.
	var emailCount int
	testPool.QueryRow(ctx, "SELECT count(*) FROM emails WHERE id = $1", emailID).Scan(&emailCount)
	if emailCount != 1 {
		t.Error("email must not be deleted when undo is too late")
	}
}

func TestCancelSend_OnlySenderCanUndo(t *testing.T) {
	orgID, userID := seedOrg(t, "undo-authz-org", "undo-authz@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "undo-authz.test")
	threadID := seedThread(t, orgID, userID, domainID, "Not Yours")

	ctx := context.Background()
	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "me@undo-authz.test", []byte(`["dest@example.com"]`), []byte(`[]`), []byte(`[]`),
		"Not Yours", "<p>body</p>", "body", "queued", "", nil)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}
	if _, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, []byte(`{}`), nil); err != nil {
		t.Fatalf("CreateEmailJob failed: %v", err)
	}

	h := &handler.EmailHandler{Store: testStore, RDB: testRDB, Bus: testBus()}
	req := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailID+"/cancel-send", nil)
	req = withChiParam(req, "id", emailID)
	req = withClaims(req, "other-user-id", orgID, "admin")
	w := httptest.NewRecorder()
	h.CancelSend(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a different user, got %d: %s", w.Code, w.Body.String())
	}
}
