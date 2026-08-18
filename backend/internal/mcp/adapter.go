package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/inboxes/backend/internal/middleware"
)

// userRateLimitIP maps a user ID to a stable, valid synthetic IPv6 address.
// The in-process router keys its per-IP rate limits on the client IP, so each
// user needs a distinct, parseable address. A non-IP string (the old value)
// is rejected by the router's RealIP step, which made every user share one
// bucket. A SHA-256 of the user ID gives 16 bytes = one unique IPv6 address.
func userRateLimitIP(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return net.IP(sum[:16]).String()
}

// callAPI drives the real API router in-process as the token's user. Every
// tool call goes through the same middleware and handlers as the web app,
// so the MCP layer adds zero new authorization logic.
func (s *Server) callAPI(id *TokenIdentity, method, path string, body interface{}) (int, json.RawMessage, error) {
	var reqBody *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	// The auth middleware requires this header on state-changing methods.
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	// Give each user a distinct, valid rate-limit identity; otherwise every
	// internal call shares httptest's fixed RemoteAddr and all agents drain
	// one shared IP bucket.
	req.Header.Set("X-Real-IP", userRateLimitIP(id.UserID))

	// Mint a short-lived session token for this user. The middleware then
	// applies its full checks: revocation, live status, role re-read.
	token, _, err := middleware.GenerateToken(s.Secret, id.UserID, id.OrgID, id.Role)
	if err != nil {
		return 0, nil, fmt.Errorf("mint session: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: "token", Value: token})

	rec := httptest.NewRecorder()
	s.api.ServeHTTP(rec, req)

	respBody := rec.Body.Bytes()
	if !json.Valid(respBody) {
		respBody, _ = json.Marshal(map[string]string{"body": strings.TrimSpace(rec.Body.String())})
	}
	return rec.Code, respBody, nil
}

// apiError extracts the "error" field from an API response body, with a
// fallback to the raw body.
func apiError(status int, body json.RawMessage) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return fmt.Sprintf("API error (HTTP %d): %s", status, parsed.Error)
	}
	return fmt.Sprintf("API error (HTTP %d): %s", status, string(body))
}
