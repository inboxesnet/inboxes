//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inboxes/backend/internal/handler"
)

// failedThreadIDs returns the thread IDs the Failed view shows for an admin.
func failedThreadIDs(t *testing.T, orgID string) []string {
	t.Helper()
	threads, _, err := testStore.ListThreads(context.Background(), orgID, "failed", "", "admin", nil, 1, 100)
	if err != nil {
		t.Fatalf("ListThreads(failed): %v", err)
	}
	ids := make([]string, 0, len(threads))
	for _, th := range threads {
		ids = append(ids, th["id"].(string))
	}
	return ids
}

func TestDismissFailedEmail(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("dismiss-%s", t.Name()), fmt.Sprintf("dismiss-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, fmt.Sprintf("dismiss-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Bounced send")

	// Two failed outbound emails in one thread: dismissing one must not
	// remove the thread from the Failed view.
	emailA := seedEmail(t, orgID, userID, domainID, threadID, "outbound", "me@x.com", "Bounced send")
	emailB := seedEmail(t, orgID, userID, domainID, threadID, "outbound", "me@x.com", "Bounced send 2")
	for _, id := range []string{emailA, emailB} {
		if _, err := testPool.Exec(ctx, "UPDATE emails SET status = 'bounced' WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
	}

	if ids := failedThreadIDs(t, orgID); len(ids) != 1 || ids[0] != threadID {
		t.Fatalf("expected thread in Failed view, got %v", ids)
	}

	h := &handler.EmailHandler{Store: testStore, RDB: testRDB, Bus: testBus()}
	dismiss := func(emailID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailID+"/dismiss", nil)
		req = withClaims(req, userID, orgID, "admin")
		req = withChiParam(req, "id", emailID)
		w := httptest.NewRecorder()
		h.Dismiss(w, req)
		return w
	}

	if w := dismiss(emailA); w.Code != http.StatusOK {
		t.Fatalf("dismiss A: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ids := failedThreadIDs(t, orgID); len(ids) != 1 {
		t.Fatalf("one failed email remains — thread must stay in Failed view, got %v", ids)
	}

	if w := dismiss(emailB); w.Code != http.StatusOK {
		t.Fatalf("dismiss B: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ids := failedThreadIDs(t, orgID); len(ids) != 0 {
		t.Fatalf("all failed emails dismissed — Failed view must be empty, got %v", ids)
	}

	// The sidebar count follows the same rule.
	counts, err := testStore.GetLabelCounts(ctx, orgID, domainID, userID, "admin", nil)
	if err != nil {
		t.Fatalf("GetLabelCounts: %v", err)
	}
	if got := counts["failed"].(int); got != 0 {
		t.Fatalf("expected failed count 0, got %d", got)
	}

	// A delivered email cannot be dismissed.
	emailC := seedEmail(t, orgID, userID, domainID, threadID, "outbound", "me@x.com", "Delivered fine")
	if w := dismiss(emailC); w.Code != http.StatusConflict {
		t.Fatalf("dismiss delivered: expected 409, got %d", w.Code)
	}

	// A member who is not the sender cannot dismiss.
	memberID := seedUser(t, orgID, fmt.Sprintf("member-%s@test.com", t.Name()), "member")
	req := httptest.NewRequest(http.MethodPost, "/api/emails/"+emailA+"/dismiss", nil)
	req = withClaims(req, memberID, orgID, "member")
	req = withChiParam(req, "id", emailA)
	w := httptest.NewRecorder()
	h.Dismiss(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member dismiss: expected 403, got %d", w.Code)
	}
}

func TestBulkDismiss(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("bulkdis-%s", t.Name()), fmt.Sprintf("bulkdis-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, fmt.Sprintf("bulkdis-%s.example.com", t.Name()))

	threadA := seedThread(t, orgID, userID, domainID, "Bounce A")
	threadB := seedThread(t, orgID, userID, domainID, "Bounce B")
	emailA := seedEmail(t, orgID, userID, domainID, threadA, "outbound", "me@x.com", "Bounce A")
	emailB := seedEmail(t, orgID, userID, domainID, threadB, "outbound", "me@x.com", "Bounce B")
	for _, id := range []string{emailA, emailB} {
		if _, err := testPool.Exec(ctx, "UPDATE emails SET status = 'failed' WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
	}

	th := &handler.ThreadHandler{Store: testStore, Bus: testBus()}
	bulkDismiss := func(actorID, role string) map[string]any {
		req := httptest.NewRequest(http.MethodPatch, "/api/threads/bulk", jsonBody(map[string]any{
			"thread_ids": []string{threadA, threadB},
			"action":     "dismiss",
		}))
		req = withClaims(req, actorID, orgID, role)
		w := httptest.NewRecorder()
		th.BulkAction(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("bulk dismiss: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		parseJSON(t, w, &resp)
		return resp
	}

	// A member who did not send the emails dismisses nothing.
	memberID := seedUser(t, orgID, fmt.Sprintf("member-%s@test.com", t.Name()), "member")
	resp := bulkDismiss(memberID, "member")
	if got := int(resp["affected"].(float64)); got != 0 {
		t.Fatalf("member bulk dismiss: expected affected 0, got %d", got)
	}
	if ids := failedThreadIDs(t, orgID); len(ids) != 2 {
		t.Fatalf("member bulk dismiss must not change the Failed view, got %v", ids)
	}

	// The admin sender dismisses both threads at once.
	resp = bulkDismiss(userID, "admin")
	if got := int(resp["affected"].(float64)); got != 2 {
		t.Fatalf("admin bulk dismiss: expected affected 2, got %d", got)
	}
	if ids := failedThreadIDs(t, orgID); len(ids) != 0 {
		t.Fatalf("expected empty Failed view after bulk dismiss, got %v", ids)
	}
}

func TestNotificationsBell(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("bell-%s", t.Name()), fmt.Sprintf("bell-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	insertEvent := func(eventType, payload string) int64 {
		var id int64
		if err := testPool.QueryRow(ctx,
			`INSERT INTO events (event_type, org_id, payload) VALUES ($1, $2, $3::jsonb) RETURNING id`,
			eventType, orgID, payload,
		).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	insertEvent("email.received", `{}`)                                                  // transient — must not appear
	insertEvent("email.status_updated", `{"status":"delivered"}`)                        // healthy — must not appear
	dnsID := insertEvent("domain.dns_degraded", `{"domain":"x.com","degraded":["SPF"]}`) // warning
	lastID := insertEvent("email.status_updated", `{"status":"bounced"}`)                // warning
	insertEvent("email.status_updated", `{"status":"bounced","dismissed":true}`)         // dismiss — must not appear

	h := &handler.EventHandler{Store: testStore}
	getNotifications := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
		req = withClaims(req, userID, orgID, "admin")
		w := httptest.NewRecorder()
		h.Notifications(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("notifications: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		parseJSON(t, w, &resp)
		return resp
	}

	resp := getNotifications()
	events := resp["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 warning events, got %d: %v", len(events), events)
	}
	first := events[0].(map[string]any)
	if first["event"].(string) != "email.status_updated" {
		t.Fatalf("expected newest event first, got %v", first["event"])
	}
	if int(resp["unread_count"].(float64)) != 2 {
		t.Fatalf("expected unread 2, got %v", resp["unread_count"])
	}

	// Mark all read; the cursor never moves backward.
	markRead := func(lastID int64) int {
		req := httptest.NewRequest(http.MethodPost, "/api/notifications/read", jsonBody(map[string]int64{"last_id": lastID}))
		req = withClaims(req, userID, orgID, "admin")
		w := httptest.NewRecorder()
		h.MarkNotificationsRead(w, req)
		return w.Code
	}
	if code := markRead(lastID); code != http.StatusNoContent {
		t.Fatalf("mark read: expected 204, got %d", code)
	}
	if code := markRead(dnsID); code != http.StatusNoContent {
		t.Fatalf("mark read backward: expected 204, got %d", code)
	}

	resp = getNotifications()
	if int(resp["unread_count"].(float64)) != 0 {
		t.Fatalf("expected unread 0 after mark read, got %v", resp["unread_count"])
	}
	if int64(resp["read_cursor"].(float64)) != lastID {
		t.Fatalf("cursor moved backward: expected %d, got %v", lastID, resp["read_cursor"])
	}
	for _, e := range resp["events"].([]any) {
		if !e.(map[string]any)["read"].(bool) {
			t.Fatalf("expected every event read, got %v", e)
		}
	}
}
