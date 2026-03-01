package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend_Unauthorized(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	req := httptest.NewRequest("POST", "/emails/send", nil)
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Send(no auth): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "unauthorized") {
		t.Errorf("Send(no auth): body = %q, want containing 'unauthorized'", w.Body.String())
	}
}

func TestSend_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader("{invalid"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("Send(invalid json): body = %q, want containing 'invalid request body'", w.Body.String())
	}
}

func TestSend_MissingFrom(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	body := `{"from":"","to":["bob@example.com"],"subject":"Hi"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(missing from): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "from, to, and subject are required") {
		t.Errorf("Send(missing from): body = %q", w.Body.String())
	}
}

func TestSend_MissingTo(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	body := `{"from":"alice@example.com","to":[],"subject":"Hi"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(missing to): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "from, to, and subject are required") {
		t.Errorf("Send(missing to): body = %q", w.Body.String())
	}
}

func TestSend_MissingSubject(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":""}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(missing subject): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "from, to, and subject are required") {
		t.Errorf("Send(missing subject): body = %q", w.Body.String())
	}
}

func TestSend_MissingMultipleFields(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	body := `{"from":"","to":[],"subject":""}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(missing all): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "from, to, and subject are required") {
		t.Errorf("Send(missing all): body = %q", w.Body.String())
	}
}

func TestSend_InvalidToEmail(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	body := `{"from":"alice@example.com","to":["not-an-email"],"subject":"Hi"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(invalid to): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid To address") {
		t.Errorf("Send(invalid to): body = %q", w.Body.String())
	}
}

func TestSend_SubjectTooLong(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{}
	longSubject := strings.Repeat("a", 501)
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":"` + longSubject + `"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(long subject): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "subject") {
		t.Errorf("Send(long subject): body = %q, want containing 'subject'", w.Body.String())
	}
}

func TestSend_ValidRequestShape(t *testing.T) {
	t.Parallel()
	// Valid JSON that passes all validation — will fail at canSendAs (nil DB), but we verify fields parse
	h := &EmailHandler{}
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":"Hi","html":"<p>Hello</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	// This will panic/500 on canSendAs due to nil DB, that's expected.
	// We recover to confirm it got past validation.
	func() {
		defer func() { recover() }()
		h.Send(w, req)
	}()
	// If we got a 400, validation rejected it (bad). If panic/500, it passed validation (good).
	if w.Code == http.StatusBadRequest {
		t.Errorf("Send(valid shape): got 400, validation should have passed: %s", w.Body.String())
	}
}
