package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Note: Tests that involve actual rate limit checks require a live Redis client.
// These tests validate the request parsing and body handling logic that occurs
// BEFORE the Redis rate check — specifically the fail-open paths.

func TestRateLimitByBodyField_InvalidJSON_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString("not json")
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called for invalid JSON")
	}
}

func TestRateLimitByBodyField_MissingField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"name":"test"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is missing")
	}
}

func TestRateLimitByBodyField_EmptyField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"email":""}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is empty")
	}
}

func TestRateLimitByBodyField_BodyPreservedAfterRead(t *testing.T) {
	t.Parallel()
	var bodyContent string
	// Use a body that won't match the field so we go to next handler without hitting Redis
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodyContent = string(data)
	}))
	original := `{"name":"test"}`
	body := bytes.NewBufferString(original)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if bodyContent != original {
		t.Errorf("body not preserved: got %q, want %q", bodyContent, original)
	}
}

func TestRateLimitByBodyField_BodyPreservedWithMatchingField(t *testing.T) {
	t.Parallel()
	// When field IS present and body is parsed, the body should still be replaced
	// for downstream handlers. We test this by reading body in a failing-open scenario.
	var bodyContent string
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodyContent = string(data)
	}))
	// Empty field passes through without hitting Redis
	original := `{"email":""}`
	body := bytes.NewBufferString(original)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if bodyContent != original {
		t.Errorf("body not preserved: got %q, want %q", bodyContent, original)
	}
}

func TestRateLimitByBodyField_NonStringField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"email":123}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is non-string")
	}
}

func TestRateLimitByBodyField_NullField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"email":null}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is null")
	}
}

func TestRateLimitByBodyField_BoolField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"email":true}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is boolean")
	}
}

func TestRateLimitByBodyField_EmptyBody_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called for empty body")
	}
}

func TestRateLimitByBodyField_ArrayField_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	handler := RateLimitByBodyField(nil, "email", 5, 60)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	body := bytes.NewBufferString(`{"email":["a@b.com"]}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler should be called when field is array (not string)")
	}
}
