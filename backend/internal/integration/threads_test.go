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

func TestThreadsList(t *testing.T) {
	orgID, userID := seedOrg(t, "threads-list-org", "threads-list@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "threads-list.test")

	seedThread(t, orgID, userID, domainID, "Thread A")
	seedThread(t, orgID, userID, domainID, "Thread B")
	seedThread(t, orgID, userID, domainID, "Thread C")

	h := &handler.ThreadHandler{Store: testStore, Bus: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/threads?label=inbox", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Threads []map[string]any `json:"threads"`
		Total   int              `json:"total"`
	}
	parseJSON(t, w, &resp)
	if len(resp.Threads) != 3 {
		t.Fatalf("expected 3 threads, got %d", len(resp.Threads))
	}
	if resp.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Total)
	}
	// Verify each thread has required fields
	for _, th := range resp.Threads {
		if th["id"] == nil || th["subject"] == nil || th["domain_id"] == nil {
			t.Fatalf("thread missing required fields: %v", th)
		}
	}
}

func TestThreadsListPagination(t *testing.T) {
	orgID, userID := seedOrg(t, "threads-page-org", "threads-page@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "threads-page.test")

	for i := 0; i < 5; i++ {
		seedThread(t, orgID, userID, domainID, "Paginated Thread")
	}

	h := &handler.ThreadHandler{Store: testStore, Bus: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/threads?label=inbox&page=1&limit=2", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Threads []map[string]any `json:"threads"`
		Total   int              `json:"total"`
	}
	parseJSON(t, w, &resp)
	if len(resp.Threads) != 2 {
		t.Fatalf("expected 2 threads per page, got %d", len(resp.Threads))
	}
	if resp.Total != 5 {
		t.Fatalf("expected total=5, got %d", resp.Total)
	}
}

func TestThreadsListByFolder(t *testing.T) {
	orgID, userID := seedOrg(t, "threads-folder-org", "threads-folder@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "threads-folder.test")

	// seedThread adds "inbox" label by default
	seedThread(t, orgID, userID, domainID, "Inbox Thread")

	h := &handler.ThreadHandler{Store: testStore, Bus: nil}

	// Should appear when listing inbox
	req := httptest.NewRequest(http.MethodGet, "/api/threads?label=inbox", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.List(w, req)

	var inboxResp struct {
		Threads []map[string]any `json:"threads"`
		Total   int              `json:"total"`
	}
	parseJSON(t, w, &inboxResp)
	if inboxResp.Total != 1 {
		t.Fatalf("expected 1 thread in inbox, got %d", inboxResp.Total)
	}

	// Should NOT appear when listing sent
	req2 := httptest.NewRequest(http.MethodGet, "/api/threads?label=sent", nil)
	req2 = withClaims(req2, userID, orgID, "admin")
	w2 := httptest.NewRecorder()
	h.List(w2, req2)

	var sentResp struct {
		Threads []map[string]any `json:"threads"`
		Total   int              `json:"total"`
	}
	parseJSON(t, w2, &sentResp)
	if sentResp.Total != 0 {
		t.Fatalf("expected 0 threads in sent, got %d", sentResp.Total)
	}
}

func TestThreadGet(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-get-org", "thread-get@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-get.test")
	threadID := seedThread(t, orgID, userID, domainID, "Viewable Thread")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "sender@example.com", "Viewable Thread")

	h := &handler.ThreadHandler{Store: testStore, Bus: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/threads/"+threadID, nil)
	req = withChiParam(req, "id", threadID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Thread map[string]any `json:"thread"`
	}
	parseJSON(t, w, &resp)
	if resp.Thread["id"] != threadID {
		t.Fatalf("expected thread id %s, got %v", threadID, resp.Thread["id"])
	}
	if resp.Thread["subject"] != "Viewable Thread" {
		t.Fatalf("expected subject 'Viewable Thread', got %v", resp.Thread["subject"])
	}
	// Should contain emails
	emails, ok := resp.Thread["emails"].([]any)
	if !ok || len(emails) < 1 {
		t.Fatalf("expected at least 1 email, got %v", resp.Thread["emails"])
	}
}

func TestThreadGetNotFound(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-nf-org", "thread-nf@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	h := &handler.ThreadHandler{Store: testStore, Bus: nil}
	fakeID := "00000000-0000-0000-0000-000000000099"
	req := httptest.NewRequest(http.MethodGet, "/api/threads/"+fakeID, nil)
	req = withChiParam(req, "id", fakeID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThreadArchive(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-archive-org", "thread-archive@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-archive.test")
	threadID := seedThread(t, orgID, userID, domainID, "Archive Me")

	ctx := context.Background()
	// Verify inbox label exists before archive
	if !testStore.HasLabel(ctx, threadID, "inbox") {
		t.Fatal("expected thread to have inbox label before archive")
	}

	// Archive = remove inbox label
	err := testStore.RemoveLabel(ctx, threadID, "inbox")
	if err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	// Verify inbox label is removed
	if testStore.HasLabel(ctx, threadID, "inbox") {
		t.Fatal("expected inbox label to be removed after archive")
	}

	// Verify thread appears in archive listing (no inbox, no trash/spam)
	threads, total, err := testStore.ListThreads(ctx, orgID, "archive", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads archive failed: %v", err)
	}
	if total != 1 || len(threads) != 1 {
		t.Fatalf("expected 1 archived thread, got total=%d, len=%d", total, len(threads))
	}
}

func TestThreadTrash(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-trash-org", "thread-trash@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-trash.test")
	threadID := seedThread(t, orgID, userID, domainID, "Trash Me")

	ctx := context.Background()

	// Trash the thread: add trash label + set expiry
	err := testStore.AddLabel(ctx, threadID, orgID, "trash")
	if err != nil {
		t.Fatalf("AddLabel trash failed: %v", err)
	}
	err = testStore.SetTrashExpiry(ctx, []string{threadID}, orgID)
	if err != nil {
		t.Fatalf("SetTrashExpiry failed: %v", err)
	}

	if !testStore.HasLabel(ctx, threadID, "trash") {
		t.Fatal("expected thread to have trash label")
	}

	// Verify thread appears in trash listing
	threads, total, err := testStore.ListThreads(ctx, orgID, "trash", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads trash failed: %v", err)
	}
	if total != 1 || len(threads) != 1 {
		t.Fatalf("expected 1 trashed thread, got total=%d, len=%d", total, len(threads))
	}
	// Verify trash_expires_at is set
	if threads[0]["trash_expires_at"] == nil {
		t.Fatal("expected trash_expires_at to be set")
	}
}

func TestThreadStar(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-star-org", "thread-star@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-star.test")
	threadID := seedThread(t, orgID, userID, domainID, "Star Me")

	ctx := context.Background()

	err := testStore.AddLabel(ctx, threadID, orgID, "starred")
	if err != nil {
		t.Fatalf("AddLabel starred failed: %v", err)
	}

	if !testStore.HasLabel(ctx, threadID, "starred") {
		t.Fatal("expected starred label")
	}

	labels := testStore.GetLabels(ctx, threadID)
	found := false
	for _, l := range labels {
		if l == "starred" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'starred' in labels, got %v", labels)
	}
}

func TestThreadUnstar(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-unstar-org", "thread-unstar@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-unstar.test")
	threadID := seedThread(t, orgID, userID, domainID, "Unstar Me")

	ctx := context.Background()

	// Star then unstar
	testStore.AddLabel(ctx, threadID, orgID, "starred")
	if !testStore.HasLabel(ctx, threadID, "starred") {
		t.Fatal("expected starred label after star")
	}

	testStore.RemoveLabel(ctx, threadID, "starred")
	if testStore.HasLabel(ctx, threadID, "starred") {
		t.Fatal("expected starred label to be removed after unstar")
	}
}

func TestThreadMarkRead(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-read-org", "thread-read@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-read.test")
	threadID := seedThread(t, orgID, userID, domainID, "Read Me")

	ctx := context.Background()

	// Set unread_count=1
	_, err := testStore.UpdateThreadUnread(ctx, threadID, orgID, 1)
	if err != nil {
		t.Fatalf("UpdateThreadUnread failed: %v", err)
	}

	// Mark read
	n, err := testStore.UpdateThreadUnread(ctx, threadID, orgID, 0)
	if err != nil {
		t.Fatalf("mark read failed: %v", err)
	}
	if n == 0 {
		t.Fatal("expected 1 row affected")
	}

	// Verify
	th, err := testStore.GetThread(ctx, threadID, orgID)
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}
	if count, ok := th["unread_count"].(int); !ok || count != 0 {
		t.Fatalf("expected unread_count=0, got %v", th["unread_count"])
	}
}

func TestThreadMarkUnread(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-unread-org", "thread-unread@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-unread.test")
	threadID := seedThread(t, orgID, userID, domainID, "Unread Me")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "sender@example.com", "Unread Me")

	ctx := context.Background()

	// Mark all emails as read first
	testStore.MarkAllEmailsRead(ctx, threadID, orgID)

	// Mark latest email unread
	err := testStore.MarkLatestEmailUnread(ctx, threadID, orgID)
	if err != nil {
		t.Fatalf("MarkLatestEmailUnread failed: %v", err)
	}

	// Verify the latest email is unread
	emails, err := testStore.GetThreadEmails(ctx, threadID, orgID)
	if err != nil {
		t.Fatalf("GetThreadEmails failed: %v", err)
	}
	if len(emails) == 0 {
		t.Fatal("expected at least 1 email")
	}
	lastEmail := emails[len(emails)-1]
	if isRead, ok := lastEmail["is_read"].(bool); !ok || isRead {
		t.Fatalf("expected latest email is_read=false, got %v", lastEmail["is_read"])
	}
}

func TestThreadMoveToSent(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-sent-org", "thread-sent@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-sent.test")
	threadID := seedThread(t, orgID, userID, domainID, "Move to Sent")

	ctx := context.Background()

	// Remove inbox, add sent
	testStore.RemoveLabel(ctx, threadID, "inbox")
	testStore.AddLabel(ctx, threadID, orgID, "sent")

	if testStore.HasLabel(ctx, threadID, "inbox") {
		t.Fatal("expected inbox label to be removed")
	}
	if !testStore.HasLabel(ctx, threadID, "sent") {
		t.Fatal("expected sent label to be added")
	}

	// Verify it shows up in sent listing
	threads, total, err := testStore.ListThreads(ctx, orgID, "sent", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads sent failed: %v", err)
	}
	if total != 1 || len(threads) != 1 {
		t.Fatalf("expected 1 sent thread, got total=%d, len=%d", total, len(threads))
	}
}

func TestThreadBulkArchive(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-bulkarc-org", "thread-bulkarc@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-bulkarc.test")

	tid1 := seedThread(t, orgID, userID, domainID, "Bulk Archive 1")
	tid2 := seedThread(t, orgID, userID, domainID, "Bulk Archive 2")
	tid3 := seedThread(t, orgID, userID, domainID, "Bulk Archive 3")
	threadIDs := []string{tid1, tid2, tid3}

	ctx := context.Background()
	err := testStore.BulkRemoveLabel(ctx, threadIDs, "inbox")
	if err != nil {
		t.Fatalf("BulkRemoveLabel failed: %v", err)
	}

	for _, tid := range threadIDs {
		if testStore.HasLabel(ctx, tid, "inbox") {
			t.Fatalf("thread %s still has inbox label after bulk archive", tid)
		}
	}

	// All should be in archive now
	threads, total, err := testStore.ListThreads(ctx, orgID, "archive", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads archive failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 archived threads, got %d", total)
	}
	if len(threads) != 3 {
		t.Fatalf("expected 3 threads returned, got %d", len(threads))
	}
}

func TestThreadBulkTrash(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-bulktrash-org", "thread-bulktrash@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-bulktrash.test")

	tid1 := seedThread(t, orgID, userID, domainID, "Bulk Trash 1")
	tid2 := seedThread(t, orgID, userID, domainID, "Bulk Trash 2")
	threadIDs := []string{tid1, tid2}

	ctx := context.Background()
	err := testStore.BulkAddLabel(ctx, threadIDs, orgID, "trash")
	if err != nil {
		t.Fatalf("BulkAddLabel trash failed: %v", err)
	}
	err = testStore.SetTrashExpiry(ctx, threadIDs, orgID)
	if err != nil {
		t.Fatalf("SetTrashExpiry failed: %v", err)
	}

	for _, tid := range threadIDs {
		if !testStore.HasLabel(ctx, tid, "trash") {
			t.Fatalf("thread %s missing trash label after bulk trash", tid)
		}
	}
}

func TestThreadSoftDelete(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-del-org", "thread-del@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-del.test")
	threadID := seedThread(t, orgID, userID, domainID, "Delete Me")

	ctx := context.Background()

	n, err := testStore.SoftDeleteThread(ctx, threadID, orgID)
	if err != nil {
		t.Fatalf("SoftDeleteThread failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}

	// Thread should no longer appear in inbox
	_, err = testStore.GetThread(ctx, threadID, orgID)
	// The thread still exists, but listing should exclude it due to deleted_at
	threads, total, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads failed: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 threads after soft delete, got %d", total)
	}
	_ = threads
}

func TestThreadMemberVisibility(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-vis-org", "thread-vis@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-vis.test")

	// Create a member user
	ctx := context.Background()
	var memberID string
	err := testPool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, role, status) VALUES ($1, $2, $3, 'member', 'active') RETURNING id`,
		orgID, "member-vis@test.io", "Member",
	).Scan(&memberID)
	if err != nil {
		t.Fatalf("create member failed: %v", err)
	}

	// Create alias and assign it to the member
	aliasID := seedAlias(t, orgID, domainID, "team-vis@thread-vis.test", "Team")
	err = testStore.AddAliasUser(ctx, aliasID, orgID, memberID, true)
	if err != nil {
		t.Fatalf("AddAliasUser failed: %v", err)
	}

	// Thread visible to admin, but member needs alias:* label on the thread
	threadID := seedThread(t, orgID, userID, domainID, "Visible Thread")
	testStore.AddLabel(ctx, threadID, orgID, "alias:team-vis@thread-vis.test")

	// Thread without alias label - not visible to member
	threadID2 := seedThread(t, orgID, userID, domainID, "Hidden Thread")
	_ = threadID2

	// Member should only see threads with their alias label
	aliasAddrs := []string{"team-vis@thread-vis.test"}
	threads, total, err := testStore.ListThreads(ctx, orgID, "inbox", "", "member", aliasAddrs, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads as member failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 visible thread for member, got %d", total)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread returned, got %d", len(threads))
	}
}

func TestThreadSearch(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-search-org", "thread-search@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-search.test")
	threadID := seedThread(t, orgID, userID, domainID, "Invoice Payment Due")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "billing@example.com", "Invoice Payment Due")

	ctx := context.Background()

	results, err := testStore.SearchEmails(ctx, orgID, "invoice", "", "admin", nil)
	if err != nil {
		t.Fatalf("SearchEmails failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result for 'invoice'")
	}

	found := false
	for _, r := range results {
		if r["id"] == threadID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected thread %s in search results", threadID)
	}
}

func TestThreadLabels(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-labels-org", "thread-labels@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-labels.test")
	threadID := seedThread(t, orgID, userID, domainID, "Label Test Thread")

	ctx := context.Background()

	// Add custom label
	err := testStore.AddLabel(ctx, threadID, orgID, "important")
	if err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}

	labels := testStore.GetLabels(ctx, threadID)
	hasImportant := false
	for _, l := range labels {
		if l == "important" {
			hasImportant = true
		}
	}
	if !hasImportant {
		t.Fatalf("expected 'important' in labels, got %v", labels)
	}

	// Remove custom label
	err = testStore.RemoveLabel(ctx, threadID, "important")
	if err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	labels = testStore.GetLabels(ctx, threadID)
	for _, l := range labels {
		if l == "important" {
			t.Fatalf("expected 'important' to be removed, still present in %v", labels)
		}
	}
}

// marshalJSON is a test helper for json.Marshal.
func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return b
}

func TestThreadUntrash(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-untrash-org", "thread-untrash@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-untrash.test")
	threadID := seedThread(t, orgID, userID, domainID, "Untrash Me")

	ctx := context.Background()

	// Trash the thread: add trash label + set expiry
	err := testStore.AddLabel(ctx, threadID, orgID, "trash")
	if err != nil {
		t.Fatalf("AddLabel trash failed: %v", err)
	}
	err = testStore.SetTrashExpiry(ctx, []string{threadID}, orgID)
	if err != nil {
		t.Fatalf("SetTrashExpiry failed: %v", err)
	}

	// Verify it's in trash
	trashThreads, trashTotal, err := testStore.ListThreads(ctx, orgID, "trash", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads trash failed: %v", err)
	}
	if trashTotal != 1 || len(trashThreads) != 1 {
		t.Fatalf("expected 1 trashed thread, got total=%d, len=%d", trashTotal, len(trashThreads))
	}

	// Untrash: remove trash label, add inbox label back
	err = testStore.RemoveLabel(ctx, threadID, "trash")
	if err != nil {
		t.Fatalf("RemoveLabel trash failed: %v", err)
	}
	err = testStore.AddLabel(ctx, threadID, orgID, "inbox")
	if err != nil {
		t.Fatalf("AddLabel inbox failed: %v", err)
	}

	// Verify it appears in inbox listing
	inboxThreads, inboxTotal, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox failed: %v", err)
	}
	if inboxTotal != 1 || len(inboxThreads) != 1 {
		t.Fatalf("expected 1 inbox thread after untrash, got total=%d, len=%d", inboxTotal, len(inboxThreads))
	}

	// Verify it's no longer in trash listing
	trashThreads2, trashTotal2, err := testStore.ListThreads(ctx, orgID, "trash", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads trash after untrash failed: %v", err)
	}
	if trashTotal2 != 0 || len(trashThreads2) != 0 {
		t.Fatalf("expected 0 trashed threads after untrash, got total=%d, len=%d", trashTotal2, len(trashThreads2))
	}
}

func TestThreadSpamToInbox(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-spam-org", "thread-spam@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-spam.test")
	threadID := seedThread(t, orgID, userID, domainID, "Spam Then Inbox")

	ctx := context.Background()

	// Mark as spam: add spam label
	err := testStore.AddLabel(ctx, threadID, orgID, "spam")
	if err != nil {
		t.Fatalf("AddLabel spam failed: %v", err)
	}

	// Verify it's in spam listing
	spamThreads, spamTotal, err := testStore.ListThreads(ctx, orgID, "spam", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads spam failed: %v", err)
	}
	if spamTotal != 1 || len(spamThreads) != 1 {
		t.Fatalf("expected 1 spam thread, got total=%d, len=%d", spamTotal, len(spamThreads))
	}

	// Verify it does NOT appear in inbox (spam excludes from inbox)
	inboxThreads, inboxTotal, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox while spam failed: %v", err)
	}
	if inboxTotal != 0 {
		t.Fatalf("expected 0 inbox threads while spam, got total=%d", inboxTotal)
	}
	_ = inboxThreads

	// Move back to inbox: remove spam, add inbox
	err = testStore.RemoveLabel(ctx, threadID, "spam")
	if err != nil {
		t.Fatalf("RemoveLabel spam failed: %v", err)
	}
	err = testStore.AddLabel(ctx, threadID, orgID, "inbox")
	if err != nil {
		t.Fatalf("AddLabel inbox failed: %v", err)
	}

	// Verify it's in inbox
	inboxThreads2, inboxTotal2, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox after unspam failed: %v", err)
	}
	if inboxTotal2 != 1 || len(inboxThreads2) != 1 {
		t.Fatalf("expected 1 inbox thread after unspam, got total=%d, len=%d", inboxTotal2, len(inboxThreads2))
	}

	// Verify it's no longer in spam
	spamThreads2, spamTotal2, err := testStore.ListThreads(ctx, orgID, "spam", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads spam after unspam failed: %v", err)
	}
	if spamTotal2 != 0 || len(spamThreads2) != 0 {
		t.Fatalf("expected 0 spam threads after unspam, got total=%d, len=%d", spamTotal2, len(spamThreads2))
	}
}

func TestThreadMuteFlag(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-mute-org", "thread-mute@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-mute.test")
	threadID := seedThread(t, orgID, userID, domainID, "Mute Me")

	ctx := context.Background()

	// Initially not muted
	if testStore.HasLabel(ctx, threadID, "muted") {
		t.Fatal("expected thread not to have muted label initially")
	}

	// Mute the thread by adding "muted" label
	err := testStore.AddLabel(ctx, threadID, orgID, "muted")
	if err != nil {
		t.Fatalf("AddLabel muted failed: %v", err)
	}

	// Verify muted label is present
	if !testStore.HasLabel(ctx, threadID, "muted") {
		t.Fatal("expected thread to have muted label after mute")
	}

	// Verify muted appears in GetLabels
	labels := testStore.GetLabels(ctx, threadID)
	foundMuted := false
	for _, l := range labels {
		if l == "muted" {
			foundMuted = true
			break
		}
	}
	if !foundMuted {
		t.Fatalf("expected 'muted' in labels, got %v", labels)
	}

	// Unmute the thread by removing "muted" label
	err = testStore.RemoveLabel(ctx, threadID, "muted")
	if err != nil {
		t.Fatalf("RemoveLabel muted failed: %v", err)
	}

	// Verify muted label is removed
	if testStore.HasLabel(ctx, threadID, "muted") {
		t.Fatal("expected muted label to be removed after unmute")
	}

	// Verify muted is gone from GetLabels
	labels2 := testStore.GetLabels(ctx, threadID)
	for _, l := range labels2 {
		if l == "muted" {
			t.Fatalf("expected 'muted' to be removed from labels, still present in %v", labels2)
		}
	}
}

func TestThreadBulkTrashAndUndo(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-bulkundo-org", "thread-bulkundo@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-bulkundo.test")

	tid1 := seedThread(t, orgID, userID, domainID, "Bulk Undo 1")
	tid2 := seedThread(t, orgID, userID, domainID, "Bulk Undo 2")
	tid3 := seedThread(t, orgID, userID, domainID, "Bulk Undo 3")
	threadIDs := []string{tid1, tid2, tid3}

	ctx := context.Background()

	// Verify all 3 are in inbox initially
	inboxThreads, inboxTotal, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox initial failed: %v", err)
	}
	if inboxTotal != 3 || len(inboxThreads) != 3 {
		t.Fatalf("expected 3 inbox threads initially, got total=%d, len=%d", inboxTotal, len(inboxThreads))
	}

	// Bulk trash: add trash label + set expiry
	err = testStore.BulkAddLabel(ctx, threadIDs, orgID, "trash")
	if err != nil {
		t.Fatalf("BulkAddLabel trash failed: %v", err)
	}
	err = testStore.SetTrashExpiry(ctx, threadIDs, orgID)
	if err != nil {
		t.Fatalf("SetTrashExpiry failed: %v", err)
	}

	// Verify all 3 are in trash
	trashThreads, trashTotal, err := testStore.ListThreads(ctx, orgID, "trash", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads trash failed: %v", err)
	}
	if trashTotal != 3 || len(trashThreads) != 3 {
		t.Fatalf("expected 3 trashed threads, got total=%d, len=%d", trashTotal, len(trashThreads))
	}

	// Verify none appear in inbox (trash excludes from inbox)
	inboxThreads2, inboxTotal2, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox while trashed failed: %v", err)
	}
	if inboxTotal2 != 0 {
		t.Fatalf("expected 0 inbox threads while trashed, got total=%d", inboxTotal2)
	}
	_ = inboxThreads2

	// Undo: remove trash label from all
	err = testStore.BulkRemoveLabel(ctx, threadIDs, "trash")
	if err != nil {
		t.Fatalf("BulkRemoveLabel trash failed: %v", err)
	}

	// Verify all 3 are back in inbox
	inboxThreads3, inboxTotal3, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox after undo failed: %v", err)
	}
	if inboxTotal3 != 3 || len(inboxThreads3) != 3 {
		t.Fatalf("expected 3 inbox threads after undo, got total=%d, len=%d", inboxTotal3, len(inboxThreads3))
	}

	// Verify trash is empty
	trashThreads2, trashTotal2, err := testStore.ListThreads(ctx, orgID, "trash", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads trash after undo failed: %v", err)
	}
	if trashTotal2 != 0 || len(trashThreads2) != 0 {
		t.Fatalf("expected 0 trashed threads after undo, got total=%d, len=%d", trashTotal2, len(trashThreads2))
	}
}

func TestThreadSearchMemberAliasScoped(t *testing.T) {
	orgID, userID := seedOrg(t, "thread-srchscope-org", "thread-srchscope@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "thread-srchscope.test")

	// Create a member user
	ctx := context.Background()
	var memberID string
	err := testPool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, role, status) VALUES ($1, $2, $3, 'member', 'active') RETURNING id`,
		orgID, "member-srchscope@test.io", "Scoped Member",
	).Scan(&memberID)
	if err != nil {
		t.Fatalf("create member failed: %v", err)
	}

	// Create alias and assign it to the member
	aliasID := seedAlias(t, orgID, domainID, "team-srch@thread-srchscope.test", "Team Search")
	err = testStore.AddAliasUser(ctx, aliasID, orgID, memberID, true)
	if err != nil {
		t.Fatalf("AddAliasUser failed: %v", err)
	}

	// Thread with alias label (should be visible to member)
	threadVisible := seedThread(t, orgID, userID, domainID, "Visible Invoice Report")
	testStore.AddLabel(ctx, threadVisible, orgID, "alias:team-srch@thread-srchscope.test")
	seedEmail(t, orgID, userID, domainID, threadVisible, "inbound", "client@example.com", "Visible Invoice Report")

	// Thread without alias label (should NOT be visible to member)
	threadHidden := seedThread(t, orgID, userID, domainID, "Hidden Invoice Summary")
	seedEmail(t, orgID, userID, domainID, threadHidden, "inbound", "other@example.com", "Hidden Invoice Summary")

	// Search as member with alias scope - should only find the visible thread
	aliasAddrs := []string{"team-srch@thread-srchscope.test"}
	results, err := testStore.SearchEmails(ctx, orgID, "invoice", "", "member", aliasAddrs)
	if err != nil {
		t.Fatalf("SearchEmails as member failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 search result for member, got %d", len(results))
	}

	// Verify the returned result is the visible thread
	foundVisible := false
	for _, r := range results {
		if r["id"] == threadVisible {
			foundVisible = true
		}
		if r["id"] == threadHidden {
			t.Fatal("member should not see thread without alias label in search results")
		}
	}
	if !foundVisible {
		t.Fatalf("expected visible thread %s in search results, got %v", threadVisible, results)
	}

	// Admin should see both threads
	adminResults, err := testStore.SearchEmails(ctx, orgID, "invoice", "", "admin", nil)
	if err != nil {
		t.Fatalf("SearchEmails as admin failed: %v", err)
	}
	if len(adminResults) != 2 {
		t.Fatalf("expected 2 search results for admin, got %d", len(adminResults))
	}
}
