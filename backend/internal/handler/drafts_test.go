package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Create ──

func TestDraftCreate_Unauthorized(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("POST", "/drafts", nil)
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("DraftCreate(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDraftCreate_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("POST", "/drafts", strings.NewReader("{bad"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DraftCreate(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("DraftCreate(invalid json): body = %q", w.Body.String())
	}
}

func TestDraftCreate_MissingDomainID(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	body := `{"subject":"Test","from_address":"test@example.com"}`
	req := httptest.NewRequest("POST", "/drafts", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DraftCreate(missing domain_id): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "domain_id is required") {
		t.Errorf("DraftCreate(missing domain_id): body = %q", w.Body.String())
	}
}

// ── Update ──

func TestDraftUpdate_Unauthorized(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("PATCH", "/drafts/123", nil)
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("DraftUpdate(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDraftUpdate_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("PATCH", "/drafts/123", strings.NewReader("{bad"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DraftUpdate(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("DraftUpdate(invalid json): body = %q", w.Body.String())
	}
}

// ── Delete ──

func TestDraftDelete_Unauthorized(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("DELETE", "/drafts/123", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("DraftDelete(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ── Send ──

func TestDraftSend_Unauthorized(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	req := httptest.NewRequest("POST", "/drafts/123/send", nil)
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("DraftSend(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
