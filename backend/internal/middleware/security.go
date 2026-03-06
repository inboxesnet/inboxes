package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds standard security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src * data: blob:; connect-src 'self' wss:; frame-ancestors 'none'")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ValidateContentType rejects POST/PATCH/PUT requests whose Content-Type
// is present but not application/json or multipart/form-data.
// Requests with no Content-Type header are allowed through (webhooks may omit it).
func ValidateContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			ct := r.Header.Get("Content-Type")
			if ct != "" {
				ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
				if ct != "application/json" && ct != "multipart/form-data" {
					http.Error(w, `{"error":"unsupported content type"}`, http.StatusUnsupportedMediaType)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
