package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/store"
)

// ── Thread List ──

func TestThreadList_Admin_Success(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			ListThreadsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
				return []map[string]any{
					{"id": "t1", "subject": "Thread 1"},
					{"id": "t2", "subject": "Thread 2"},
				}, 2, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads?label=inbox", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List: got status %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("List: failed to parse response: %v", err)
	}
	threads, ok := resp["threads"].([]interface{})
	if !ok {
		t.Fatalf("List: threads is not an array: %T", resp["threads"])
	}
	if len(threads) != 2 {
		t.Errorf("List: got %d threads, want 2", len(threads))
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("List: got total %v, want 2", resp["total"])
	}
}

func TestThreadList_MemberFiltered(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetUserAliasAddressesFn: func(ctx context.Context, userID string) ([]string, error) {
				return []string{"user@test.com"}, nil
			},
			ListThreadsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
				// Verify alias addresses were passed through for member
				if len(aliasAddrs) != 1 || aliasAddrs[0] != "user@test.com" {
					t.Errorf("ListThreads: expected aliasAddrs [user@test.com], got %v", aliasAddrs)
				}
				return []map[string]any{
					{"id": "t1", "subject": "Filtered Thread"},
				}, 1, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads?label=inbox", nil)
	req = withClaims(req, "user1", "org1", "member")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List(member): got status %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("List(member): failed to parse response: %v", err)
	}
	threads := resp["threads"].([]interface{})
	if len(threads) != 1 {
		t.Errorf("List(member): got %d threads, want 1", len(threads))
	}
}

func TestThreadList_EmptyResult(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			ListThreadsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
				return []map[string]any{}, 0, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads?label=inbox", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List(empty): got status %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("List(empty): failed to parse response: %v", err)
	}
	threads := resp["threads"].([]interface{})
	if len(threads) != 0 {
		t.Errorf("List(empty): got %d threads, want 0", len(threads))
	}
}

// ── Thread Get ──

func TestThreadGet_Success(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": "t1", "subject": "Test Thread"}, nil
			},
			GetThreadEmailsFn: func(ctx context.Context, threadID, orgID string) ([]map[string]any, error) {
				return []map[string]any{
					{"id": "e1", "from": "alice@test.com"},
				}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads/t1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Get: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Get: failed to parse response: %v", err)
	}
	thread := resp["thread"].(map[string]interface{})
	if thread["id"] != "t1" {
		t.Errorf("Get: thread id = %v, want t1", thread["id"])
	}
	emails := thread["emails"].([]interface{})
	if len(emails) != 1 {
		t.Errorf("Get: got %d emails, want 1", len(emails))
	}
}

func TestThreadGet_NotFound(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return nil, errors.New("not found")
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads/t999", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Get(not found): got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ── Thread Archive ──

func TestThreadArchive_Success(t *testing.T) {
	t.Parallel()
	removeLabelCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			RemoveLabelFn: func(ctx context.Context, threadID, label string) error {
				if label == "inbox" {
					removeLabelCalled = true
				}
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/archive", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Archive(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Archive: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !removeLabelCalled {
		t.Error("Archive: RemoveLabel('inbox') was not called")
	}
}

// ── Thread Trash ──

func TestThreadTrash_Success(t *testing.T) {
	t.Parallel()
	addLabelCalled := false
	trashExpiryCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
				if label == "trash" {
					addLabelCalled = true
				}
				return nil
			},
			SetTrashExpiryFn: func(ctx context.Context, threadIDs []string, orgID string) error {
				trashExpiryCalled = true
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/trash", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Trash(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Trash: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !addLabelCalled {
		t.Error("Trash: AddLabel('trash') was not called")
	}
	if !trashExpiryCalled {
		t.Error("Trash: SetTrashExpiry was not called")
	}
}

// ── Thread Star / Unstar ──

func TestThreadStar_Success(t *testing.T) {
	t.Parallel()
	addLabelCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return false // not currently starred
			},
			AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
				if label == "starred" {
					addLabelCalled = true
				}
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/star", strings.NewReader(`{"starred": true}`))
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Star(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Star: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !addLabelCalled {
		t.Error("Star: AddLabel('starred') was not called")
	}
}

func TestThreadUnstar_Success(t *testing.T) {
	t.Parallel()
	removeLabelCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return true // currently starred
			},
			RemoveLabelFn: func(ctx context.Context, threadID, label string) error {
				if label == "starred" {
					removeLabelCalled = true
				}
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/star", strings.NewReader(`{"starred": false}`))
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Star(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Unstar: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !removeLabelCalled {
		t.Error("Unstar: RemoveLabel('starred') was not called")
	}
}

// ── Bulk Archive ──

// ── #36: List threads with pagination ──

func TestThreadList_Pagination(t *testing.T) {
	t.Parallel()
	var capturedPage, capturedLimit int
	h := &ThreadHandler{
		Store: &store.MockStore{
			ListThreadsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
				capturedPage = page
				capturedLimit = limit
				return []map[string]any{{"id": "t1"}}, 100, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads?label=inbox&page=3&limit=25", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List(pagination): got status %d, want %d", w.Code, http.StatusOK)
	}
	if capturedPage != 3 {
		t.Errorf("List(pagination): page = %d, want 3", capturedPage)
	}
	if capturedLimit != 25 {
		t.Errorf("List(pagination): limit = %d, want 25", capturedLimit)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["page"].(float64) != 3 {
		t.Errorf("List(pagination): response page = %v, want 3", resp["page"])
	}
	if resp["total"].(float64) != 100 {
		t.Errorf("List(pagination): response total = %v, want 100", resp["total"])
	}
}

// ── #37: List threads with folder/label filtering ──

func TestThreadList_FolderFiltering(t *testing.T) {
	t.Parallel()
	var capturedLabel string
	h := &ThreadHandler{
		Store: &store.MockStore{
			ListThreadsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
				capturedLabel = label
				return []map[string]any{}, 0, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("GET", "/threads?label=trash", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List(folder): got status %d, want %d", w.Code, http.StatusOK)
	}
	if capturedLabel != "trash" {
		t.Errorf("List(folder): label = %q, want trash", capturedLabel)
	}
}

// ── #40: Bulk untrash → move to inbox ──

func TestBulkAction_Untrash(t *testing.T) {
	t.Parallel()
	addLabelCalled := false
	removeLabelCalls := map[string]bool{}
	h := &ThreadHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					BulkAddLabelFn: func(ctx context.Context, threadIDs []string, orgID, label string) error {
						if label == "inbox" {
							addLabelCalled = true
						}
						return nil
					},
					BulkRemoveLabelFn: func(ctx context.Context, threadIDs []string, label string) error {
						removeLabelCalls[label] = true
						return nil
					},
					QFn: func() store.Querier { return &store.MockQuerier{} },
				})
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1"],"action":"move","label":"inbox"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(untrash): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !addLabelCalled {
		t.Error("BulkAction(untrash): BulkAddLabel('inbox') was not called")
	}
	if !removeLabelCalls["trash"] {
		t.Error("BulkAction(untrash): BulkRemoveLabel('trash') was not called")
	}
	if !removeLabelCalls["spam"] {
		t.Error("BulkAction(untrash): BulkRemoveLabel('spam') was not called")
	}
}

// ── #41: Bulk spam ──

func TestBulkAction_Spam(t *testing.T) {
	t.Parallel()
	bulkAddCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkAddLabelFn: func(ctx context.Context, threadIDs []string, orgID, label string) error {
				if label == "spam" && len(threadIDs) == 1 {
					bulkAddCalled = true
				}
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1"],"action":"spam"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(spam): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !bulkAddCalled {
		t.Error("BulkAction(spam): BulkAddLabel('spam') was not called")
	}
}

// ── #43: Mute/unmute toggle ──

func TestThread_Mute_Toggle(t *testing.T) {
	t.Parallel()
	addLabelCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return false // not currently muted
			},
			AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
				if label == "muted" {
					addLabelCalled = true
				}
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/mute", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Mute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Mute: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !addLabelCalled {
		t.Error("Mute: AddLabel('muted') was not called")
	}
}

func TestThread_Unmute_Toggle(t *testing.T) {
	t.Parallel()
	removeLabelCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return true // currently muted
			},
			RemoveLabelFn: func(ctx context.Context, threadID, label string) error {
				if label == "muted" {
					removeLabelCalled = true
				}
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/mute", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Mute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Unmute: got status %d, want %d", w.Code, http.StatusOK)
	}
	if !removeLabelCalled {
		t.Error("Unmute: RemoveLabel('muted') was not called")
	}
}

// ── #44: Mark read ──

func TestThread_MarkRead(t *testing.T) {
	t.Parallel()
	markReadCalled := false
	markEmailsReadCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			UpdateThreadUnreadFn: func(ctx context.Context, threadID, orgID string, unread int) (int64, error) {
				markReadCalled = true
				if unread != 0 {
					t.Errorf("MarkRead: unread = %d, want 0", unread)
				}
				return 1, nil
			},
			MarkAllEmailsReadFn: func(ctx context.Context, threadID, orgID string) error {
				markEmailsReadCalled = true
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/read", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.MarkRead(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("MarkRead: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !markReadCalled {
		t.Error("MarkRead: UpdateThreadUnread was not called")
	}
	if !markEmailsReadCalled {
		t.Error("MarkRead: MarkAllEmailsRead was not called")
	}
}

// ── #44b: Mark unread ──

func TestThread_MarkUnread(t *testing.T) {
	t.Parallel()
	markUnreadCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			UpdateThreadUnreadFn: func(ctx context.Context, threadID, orgID string, unread int) (int64, error) {
				markUnreadCalled = true
				if unread != 1 {
					t.Errorf("MarkUnread: unread = %d, want 1", unread)
				}
				return 1, nil
			},
			MarkLatestEmailUnreadFn: func(ctx context.Context, threadID, orgID string) error {
				return nil
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/unread", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.MarkUnread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("MarkUnread: got status %d, want %d", w.Code, http.StatusOK)
	}
	if !markUnreadCalled {
		t.Error("MarkUnread: UpdateThreadUnread was not called")
	}
}

// ── #45: Permanent delete only from trash ──

func TestThread_PermanentDelete_OnlyFromTrash(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return false // not in trash
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("DELETE", "/threads/t1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Delete(not in trash): got status %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "not in trash") {
		t.Errorf("Delete(not in trash): body = %q", w.Body.String())
	}
}

func TestThread_PermanentDelete_FromTrash_Success(t *testing.T) {
	t.Parallel()
	removeAllCalled := false
	softDeleteCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			HasLabelFn: func(ctx context.Context, threadID, label string) bool {
				return label == "trash" // is in trash
			},
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					RemoveAllLabelsFn: func(ctx context.Context, threadID string) error {
						removeAllCalled = true
						return nil
					},
					SoftDeleteThreadFn: func(ctx context.Context, threadID, orgID string) (int64, error) {
						softDeleteCalled = true
						return 1, nil
					},
				})
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("DELETE", "/threads/t1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Delete(from trash): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !removeAllCalled {
		t.Error("Delete: RemoveAllLabels was not called")
	}
	if !softDeleteCalled {
		t.Error("Delete: SoftDeleteThread was not called")
	}
}

// ── #47: Bulk select all pages mode ──

func TestBulkAction_SelectAll(t *testing.T) {
	t.Parallel()
	var resolvedIDs []string
	h := &ThreadHandler{
		Store: &store.MockStore{
			ResolveFilteredThreadIDsFn: func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string) ([]string, error) {
				return []string{"t1", "t2", "t3"}, nil
			},
			BulkRemoveLabelFn: func(ctx context.Context, threadIDs []string, label string) error {
				resolvedIDs = threadIDs
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"action":"archive","select_all":true,"filter_label":"inbox"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(select_all): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if len(resolvedIDs) != 3 {
		t.Errorf("BulkAction(select_all): resolved %d IDs, want 3", len(resolvedIDs))
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["affected"].(float64) != 3 {
		t.Errorf("BulkAction(select_all): affected = %v, want 3", resp["affected"])
	}
}

// ── #48: Bulk move to folder ──

func TestBulkAction_MoveToTrash(t *testing.T) {
	t.Parallel()
	trashLabelAdded := false
	expirySet := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					BulkAddLabelFn: func(ctx context.Context, threadIDs []string, orgID, label string) error {
						if label == "trash" {
							trashLabelAdded = true
						}
						return nil
					},
					SetTrashExpiryFn: func(ctx context.Context, threadIDs []string, orgID string) error {
						expirySet = true
						return nil
					},
				})
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1","t2"],"action":"trash"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(trash): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !trashLabelAdded {
		t.Error("BulkAction(trash): BulkAddLabel('trash') was not called")
	}
	if !expirySet {
		t.Error("BulkAction(trash): SetTrashExpiry was not called")
	}
}

// ── #49: Bulk apply/remove custom labels ──

func TestBulkAction_ApplyCustomLabel(t *testing.T) {
	t.Parallel()
	var capturedLabel string
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkAddLabelFn: func(ctx context.Context, threadIDs []string, orgID, label string) error {
				capturedLabel = label
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1"],"action":"label","label":"important"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(label): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedLabel != "important" {
		t.Errorf("BulkAction(label): label = %q, want important", capturedLabel)
	}
}

func TestBulkAction_RemoveCustomLabel(t *testing.T) {
	t.Parallel()
	var capturedLabel string
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkRemoveLabelFn: func(ctx context.Context, threadIDs []string, label string) error {
				capturedLabel = label
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1"],"action":"unlabel","label":"important"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(unlabel): got status %d, want %d", w.Code, http.StatusOK)
	}
	if capturedLabel != "important" {
		t.Errorf("BulkAction(unlabel): label = %q, want important", capturedLabel)
	}
}

// ── #43b: Bulk mute/unmute ──

func TestBulkAction_Mute(t *testing.T) {
	t.Parallel()
	muteCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkAddLabelFn: func(ctx context.Context, threadIDs []string, orgID, label string) error {
				if label == "muted" {
					muteCalled = true
				}
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1"],"action":"mute"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(mute): got status %d, want %d", w.Code, http.StatusOK)
	}
	if !muteCalled {
		t.Error("BulkAction(mute): BulkAddLabel('muted') was not called")
	}
}

// ── Bulk read/unread ──

func TestBulkAction_MarkRead(t *testing.T) {
	t.Parallel()
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkUpdateUnreadFn: func(ctx context.Context, threadIDs []string, orgID string, unread int) (int64, error) {
				if unread != 0 {
					t.Errorf("BulkAction(read): unread = %d, want 0", unread)
				}
				return int64(len(threadIDs)), nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1","t2"],"action":"read"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkAction(read): got status %d, want %d", w.Code, http.StatusOK)
	}
}

// ── Spam handler tests ──

func TestThread_Spam_AddSpam(t *testing.T) {
	t.Parallel()
	spamAdded := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
						if label == "spam" {
							spamAdded = true
						}
						return nil
					},
				})
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/spam", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Spam(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Spam: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !spamAdded {
		t.Error("Spam: AddLabel('spam') was not called")
	}
}

func TestThread_Spam_NotSpam(t *testing.T) {
	t.Parallel()
	spamRemoved := false
	inboxAdded := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			GetThreadDomainIDFn: func(ctx context.Context, threadID, orgID string) (string, error) {
				return "d1", nil
			},
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					RemoveLabelFn: func(ctx context.Context, threadID, label string) error {
						if label == "spam" {
							spamRemoved = true
						}
						return nil
					},
					AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
						if label == "inbox" {
							inboxAdded = true
						}
						return nil
					},
				})
			},
			FetchThreadSummaryFn: func(ctx context.Context, threadID, orgID string) (map[string]any, error) {
				return map[string]any{"id": threadID}, nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	req := httptest.NewRequest("POST", "/threads/t1/spam", strings.NewReader(`{"action":"not_spam"}`))
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Spam(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Spam(not_spam): got status %d, want %d", w.Code, http.StatusOK)
	}
	if !spamRemoved {
		t.Error("Spam(not_spam): RemoveLabel('spam') was not called")
	}
	if !inboxAdded {
		t.Error("Spam(not_spam): AddLabel('inbox') was not called")
	}
}

func TestBulkArchive_Success(t *testing.T) {
	t.Parallel()
	bulkRemoveCalled := false
	h := &ThreadHandler{
		Store: &store.MockStore{
			BulkRemoveLabelFn: func(ctx context.Context, threadIDs []string, label string) error {
				if label == "inbox" && len(threadIDs) == 2 {
					bulkRemoveCalled = true
				}
				return nil
			},
		},
		Bus: event.NewBus(nil, nil),
	}
	body := `{"thread_ids":["t1","t2"],"action":"archive"}`
	req := httptest.NewRequest("POST", "/threads/bulk", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.BulkAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BulkArchive: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !bulkRemoveCalled {
		t.Error("BulkArchive: BulkRemoveLabel('inbox') was not called")
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("BulkArchive: failed to parse response: %v", err)
	}
	if resp["affected"].(float64) != 2 {
		t.Errorf("BulkArchive: affected = %v, want 2", resp["affected"])
	}
}
