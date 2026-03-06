package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/store"
)

// ── Domain List ──

func TestDomainList_Success(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{
			ListDomainsFn: func(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error) {
				return []map[string]any{
					{"id": "d1", "domain": "example.com", "status": "verified"},
					{"id": "d2", "domain": "test.com", "status": "pending"},
				}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/domains", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("List: failed to parse response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("List: got %d domains, want 2", len(resp))
	}
}

// ── Domain List All (including hidden) ──

func TestDomainListAll_Success(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{
			ListDomainsFn: func(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error) {
				if !includeHidden {
					t.Error("ListAll: expected includeHidden=true")
				}
				return []map[string]any{
					{"id": "d1", "domain": "example.com", "status": "verified"},
					{"id": "d2", "domain": "hidden.com", "status": "verified", "hidden": true},
				}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/domains/all", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.ListAll(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListAll: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ListAll: failed to parse response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("ListAll: got %d domains, want 2", len(resp))
	}
}

// ── Domain Soft Delete ──

func TestDomainSoftDelete_Success(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{
			SoftDeleteDomainFn: func(ctx context.Context, domainID, orgID string) (int64, error) {
				return 1, nil
			},
			CascadeDeleteDomainFn: func(ctx context.Context, domainID string) error {
				return nil
			},
		},
	}
	req := httptest.NewRequest("DELETE", "/domains/d1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "d1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("SoftDelete: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestDomainSoftDelete_NotFound(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{
			SoftDeleteDomainFn: func(ctx context.Context, domainID, orgID string) (int64, error) {
				return 0, nil
			},
		},
	}
	req := httptest.NewRequest("DELETE", "/domains/d999", nil)
	req = withClaims(req, "user1", "org1", "admin")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "d999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("SoftDelete(not found): got status %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// ── Domain Reorder ──

func TestDomainReorder_Success(t *testing.T) {
	t.Parallel()
	reorderCalled := false
	h := &DomainHandler{
		Store: &store.MockStore{
			ReorderDomainsFn: func(ctx context.Context, orgID string, order []store.DomainOrder) error {
				reorderCalled = true
				if len(order) != 2 {
					t.Errorf("Reorder: expected 2 items, got %d", len(order))
				}
				return nil
			},
		},
	}
	body := `{"order":[{"id":"d1","order":0},{"id":"d2","order":1}]}`
	req := httptest.NewRequest("PUT", "/domains/reorder", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Reorder(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Reorder: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if !reorderCalled {
		t.Error("Reorder: ReorderDomains was not called")
	}
}

// ── Domain Create ──

func TestDomainCreate_MissingDomain(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{},
	}
	body := `{"domain":""}`
	req := httptest.NewRequest("POST", "/domains", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Create(missing domain): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "domain is required") {
		t.Errorf("Create(missing domain): body = %q", w.Body.String())
	}
}

// ── Domain Update Visibility ──

func TestDomainUpdateVisibility_EmptyList(t *testing.T) {
	t.Parallel()
	h := &DomainHandler{
		Store: &store.MockStore{},
	}
	body := `{"visible":[]}`
	req := httptest.NewRequest("PUT", "/domains/visibility", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.UpdateVisibility(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateVisibility(empty): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "at least one domain must remain visible") {
		t.Errorf("UpdateVisibility(empty): body = %q", w.Body.String())
	}
}

func TestDomainUpdateVisibility_Success(t *testing.T) {
	t.Parallel()
	updateCalled := false
	h := &DomainHandler{
		Store: &store.MockStore{
			UpdateDomainVisibilityFn: func(ctx context.Context, orgID string, visible []string) error {
				updateCalled = true
				if len(visible) != 2 {
					t.Errorf("UpdateVisibility: got %d visible, want 2", len(visible))
				}
				return nil
			},
		},
	}
	body := `{"visible":["d1","d2"]}`
	req := httptest.NewRequest("PUT", "/domains/visibility", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.UpdateVisibility(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("UpdateVisibility: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if !updateCalled {
		t.Error("UpdateVisibility: store.UpdateDomainVisibility was not called")
	}
}
