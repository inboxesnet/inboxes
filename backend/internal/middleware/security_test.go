package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_XContentTypeOptions(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
	}
}

func TestSecurityHeaders_XFrameOptions(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want %q", got, "DENY")
	}
}

func TestSecurityHeaders_ReferrerPolicy(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy: got %q, want %q", got, "strict-origin-when-cross-origin")
	}
}

func TestSecurityHeaders_CacheControl(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control: got %q, want %q", got, "no-store")
	}
}

func TestSecurityHeaders_CSP(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src", "frame-ancestors", "script-src"} {
		if !contains(csp, want) {
			t.Errorf("CSP missing %q: got %q", want, csp)
		}
	}
}

func TestSecurityHeaders_HSTS_HTTPS(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("HSTS header absent for X-Forwarded-Proto: https")
	}
}

func TestSecurityHeaders_HSTS_HTTP(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS present for plain HTTP: got %q", got)
	}
}

func TestSecurityHeaders_HSTS_TLS(t *testing.T) {
	t.Parallel()
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("HSTS header absent for TLS connection")
	}
}

func TestSecurityHeaders_NextHandlerCalled(t *testing.T) {
	t.Parallel()
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("next handler was not called")
	}
}

func TestValidateContentType_ApplicationJSON(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for application/json")
	}
}

func TestValidateContentType_MultipartFormData(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for multipart/form-data")
	}
}

func TestValidateContentType_TextHTML_Rejected(t *testing.T) {
	t.Parallel()
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for text/html")
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/html: got status %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestValidateContentType_NoContentType_Allowed(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for missing Content-Type (webhook compat)")
	}
}

func TestValidateContentType_GET_Ignored(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Content-Type", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for GET with text/html")
	}
}

func TestValidateContentType_PATCH_Validated(t *testing.T) {
	t.Parallel()
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("PATCH", "/", nil)
	req.Header.Set("Content-Type", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("PATCH text/html: got status %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestValidateContentType_PUT_Validated(t *testing.T) {
	t.Parallel()
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("PUT", "/", nil)
	req.Header.Set("Content-Type", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("PUT text/html: got status %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestValidateContentType_DELETE_Validated(t *testing.T) {
	t.Parallel()
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("DELETE", "/", nil)
	req.Header.Set("Content-Type", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("DELETE text/html: got status %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestValidateContentType_ContentTypeWithCharset(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for application/json; charset=utf-8")
	}
}

func TestValidateContentType_CaseInsensitive(t *testing.T) {
	t.Parallel()
	called := false
	handler := ValidateContentType(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "Application/JSON")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for Application/JSON (case-insensitive)")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
