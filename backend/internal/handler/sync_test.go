package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inboxes/backend/internal/middleware"
)

func TestStartSync_RequiresAdmin_MemberGets403(t *testing.T) {
	t.Parallel()
	h := &SyncHandler{}
	req := httptest.NewRequest("POST", "/api/sync", nil)
	claims := &middleware.Claims{UserID: "u1", OrgID: "org1", Role: "member"}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.StartSync(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("StartSync(member): got status %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStartSync_NoClaims_Gets401(t *testing.T) {
	t.Parallel()
	h := &SyncHandler{}
	req := httptest.NewRequest("POST", "/api/sync", nil)
	w := httptest.NewRecorder()
	h.StartSync(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("StartSync(no claims): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGetSync_RequiresAuth(t *testing.T) {
	t.Parallel()
	h := &SyncHandler{}
	req := httptest.NewRequest("GET", "/api/sync/123", nil)
	w := httptest.NewRecorder()
	h.GetSync(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetSync(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
