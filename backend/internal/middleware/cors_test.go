package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Allow-Origin: got %q, want %q", got, "https://app.example.com")
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin for disallowed origin: got %q, want empty", got)
	}
}

func TestCORSMiddleware_PreflightOptions(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight: got status %d, want 200 or 204", w.Code)
	}
}

func TestCORSMiddleware_AllowedMethods(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	// go-chi/cors returns the requested method if it's in the allowed list
	for _, m := range []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"} {
		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", m)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		methods := w.Header().Get("Access-Control-Allow-Methods")
		if !strings.Contains(methods, m) {
			t.Errorf("Allow-Methods missing %q for request method %q: got %q", m, m, methods)
		}
	}
}

func TestCORSMiddleware_AllowedHeaders(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization, X-Requested-With")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	headers := w.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Content-Type", "Authorization", "X-Requested-With"} {
		if !strings.Contains(strings.ToLower(headers), strings.ToLower(h)) {
			t.Errorf("Allow-Headers missing %q: got %q", h, headers)
		}
	}
}

func TestCORSMiddleware_AllowCredentials(t *testing.T) {
	t.Parallel()
	handler := CORSMiddleware("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials: got %q, want %q", got, "true")
	}
}
