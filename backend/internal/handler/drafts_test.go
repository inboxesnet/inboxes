package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Create ──
// Auth enforcement is handled by AuthMiddleware in the router.

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

func TestDraftCreate_SubjectTooLong(t *testing.T) {
	t.Parallel()
	h := &DraftHandler{}
	longSubject := strings.Repeat("a", 501)
	body := `{"domain_id":"d1","subject":"` + longSubject + `","kind":"compose"}`
	req := httptest.NewRequest("POST", "/drafts", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("DraftCreate(long subject): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "subject") {
		t.Errorf("DraftCreate(long subject): body = %q, want containing 'subject'", w.Body.String())
	}
}

func TestDraftCreate_DefaultKind(t *testing.T) {
	t.Parallel()
	// Verify that missing kind doesn't cause a 400 — it defaults to "compose"
	h := &DraftHandler{}
	body := `{"domain_id":"d1","subject":"Test"}`
	req := httptest.NewRequest("POST", "/drafts", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	// Will fail at DB insert (nil DB), but should NOT fail at validation
	func() {
		defer func() { recover() }()
		h.Create(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("DraftCreate(no kind): got 400, validation should pass: %s", w.Body.String())
	}
}

// Auth enforcement for List, Delete, Send is handled by AuthMiddleware in the router.
