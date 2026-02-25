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
