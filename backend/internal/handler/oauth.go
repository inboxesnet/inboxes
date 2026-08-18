package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/mcp"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

// OAuthHandler implements a minimal OAuth 2.1 authorization server for MCP
// clients: dynamic client registration (RFC 7591), authorization code with
// mandatory PKCE S256, refresh tokens, and the discovery metadata MCP
// clients use to find it (RFC 8414 / RFC 9728). Public clients only.
type OAuthHandler struct {
	Store     store.Store
	AppURL    string
	PublicURL string
}

const (
	accessTokenTTL  = 30 * 24 * time.Hour
	refreshTokenTTL = 90 * 24 * time.Hour
	authCodeTTL     = 10 * time.Minute
)

// isLoopbackHost reports whether a URL host is a loopback address. It accepts
// an optional port, e.g. "127.0.0.1:53123" or "[::1]:5000".
func isLoopbackHost(host string) bool {
	h := host
	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// validateRedirectURI enforces the redirect policy for public MCP clients.
// It permits only a loopback http(s) address (RFC 8252 native-app pattern) or
// a reverse-domain private-use scheme. It rejects any remote http(s) host, so
// an attacker cannot register a client that sends the authorization code to a
// server they control.
func validateRedirectURI(u string) error {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("invalid redirect_uri: %s", u)
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		if !isLoopbackHost(parsed.Host) {
			return fmt.Errorf("redirect_uri must be a loopback address (127.0.0.1, ::1, or localhost)")
		}
		return nil
	case "javascript", "data", "vbscript", "file":
		return fmt.Errorf("invalid redirect_uri scheme")
	default:
		// Private-use URI scheme for a native app, e.g. com.example.app:/cb.
		// RFC 8252 requires a dotted, reverse-domain scheme. Such a scheme
		// opens a local app, not a remote server.
		if !strings.Contains(scheme, ".") {
			return fmt.Errorf("redirect_uri must be a loopback http(s) address or a reverse-domain app scheme")
		}
		return nil
	}
}

// ProtectedResourceMetadata serves /.well-known/oauth-protected-resource.
func (h *OAuthHandler) ProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"resource":                 h.PublicURL + "/mcp",
		"authorization_servers":    []string{h.PublicURL},
		"bearer_methods_supported": []string{"header"},
	})
}

// AuthorizationServerMetadata serves /.well-known/oauth-authorization-server.
// The authorization endpoint is a frontend page (login + consent live there);
// everything else is served by this backend.
func (h *OAuthHandler) AuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issuer":                                h.PublicURL,
		"authorization_endpoint":                h.AppURL + "/oauth/authorize",
		"token_endpoint":                        h.PublicURL + "/api/oauth/token",
		"registration_endpoint":                 h.PublicURL + "/api/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"inboxes"},
	})
}

// Register handles dynamic client registration. Public, rate-limited.
func (h *OAuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := readJSON(r, &req); err != nil || len(req.RedirectURIs) == 0 {
		writeError(w, http.StatusBadRequest, "redirect_uris is required")
		return
	}
	if len(req.RedirectURIs) > 10 {
		writeError(w, http.StatusBadRequest, "too many redirect_uris")
		return
	}
	for _, u := range req.RedirectURIs {
		if err := validateRedirectURI(u); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(req.ClientName) > 200 {
		req.ClientName = req.ClientName[:200]
	}

	urisJSON, _ := json.Marshal(req.RedirectURIs)
	var clientID string
	if err := h.Store.Q().QueryRow(r.Context(),
		`INSERT INTO oauth_clients (name, redirect_uris) VALUES ($1, $2) RETURNING id`,
		req.ClientName, urisJSON,
	).Scan(&clientID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register client")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"client_id":                  clientID,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	})
}

// ClientInfo returns a client's display name so the consent page can show
// who is asking. Public, read-only, rate-limited.
func (h *OAuthHandler) ClientInfo(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		writeError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	var name string
	if err := h.Store.Q().QueryRow(r.Context(),
		`SELECT name FROM oauth_clients WHERE id = $1`, clientID,
	).Scan(&name); err != nil {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"client_id": clientID, "name": name})
}

// Approve is called by the logged-in consent page. It validates the request
// and returns the redirect URL carrying a fresh authorization code.
func (h *OAuthHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		ClientID            string `json:"client_id"`
		RedirectURI         string `json:"redirect_uri"`
		State               string `json:"state"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ClientID == "" || req.RedirectURI == "" || req.CodeChallenge == "" {
		writeError(w, http.StatusBadRequest, "client_id, redirect_uri, and code_challenge are required")
		return
	}
	if req.CodeChallengeMethod != "S256" {
		writeError(w, http.StatusBadRequest, "code_challenge_method must be S256")
		return
	}

	// The redirect URI must match one the client registered.
	var urisJSON []byte
	if err := h.Store.Q().QueryRow(r.Context(),
		`SELECT redirect_uris FROM oauth_clients WHERE id = $1`, req.ClientID,
	).Scan(&urisJSON); err != nil {
		writeError(w, http.StatusBadRequest, "unknown client")
		return
	}
	var uris []string
	json.Unmarshal(urisJSON, &uris)
	allowed := false
	for _, u := range uris {
		if u == req.RedirectURI {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, http.StatusBadRequest, "redirect_uri not registered for this client")
		return
	}
	// Re-check the policy at grant time. This blocks a remote redirect on any
	// client that a caller registered before the registration rule existed.
	if err := validateRedirectURI(req.RedirectURI); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	code, err := mcp.NewToken("inbx_c")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create code")
		return
	}
	if _, err := h.Store.Q().Exec(r.Context(),
		`INSERT INTO oauth_codes (code_hash, client_id, user_id, org_id, redirect_uri, code_challenge, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		mcp.HashToken(code), req.ClientID, claims.UserID, claims.OrgID,
		req.RedirectURI, req.CodeChallenge, time.Now().Add(authCodeTTL),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store code")
		return
	}

	redirect, _ := url.Parse(req.RedirectURI)
	q := redirect.Query()
	q.Set("code", code)
	if req.State != "" {
		q.Set("state", req.State)
	}
	redirect.RawQuery = q.Encode()

	writeJSON(w, http.StatusOK, map[string]interface{}{"redirect_to": redirect.String()})
}

// Token is the OAuth token endpoint. It accepts form-encoded bodies (the
// OAuth standard) and JSON. Public, rate-limited, PKCE-verified.
func (h *OAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	params := map[string]string{}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if ct == "application/json" {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.tokenError(w, "invalid_request", "invalid JSON body")
			return
		}
		params = body
	} else {
		if err := r.ParseForm(); err != nil {
			h.tokenError(w, "invalid_request", "invalid form body")
			return
		}
		for k := range r.PostForm {
			params[k] = r.PostForm.Get(k)
		}
	}

	switch params["grant_type"] {
	case "authorization_code":
		h.tokenFromCode(w, r, params)
	case "refresh_token":
		h.tokenFromRefresh(w, r, params)
	default:
		h.tokenError(w, "unsupported_grant_type", "use authorization_code or refresh_token")
	}
}

func (h *OAuthHandler) tokenFromCode(w http.ResponseWriter, r *http.Request, p map[string]string) {
	code, verifier := p["code"], p["code_verifier"]
	if code == "" || verifier == "" {
		h.tokenError(w, "invalid_request", "code and code_verifier are required")
		return
	}

	ctx := r.Context()
	var clientID, userID, orgID, redirectURI, challenge string
	var expiresAt time.Time
	err := h.Store.Q().QueryRow(ctx,
		`DELETE FROM oauth_codes WHERE code_hash = $1
		 RETURNING client_id, user_id, org_id, redirect_uri, code_challenge, expires_at`,
		mcp.HashToken(code),
	).Scan(&clientID, &userID, &orgID, &redirectURI, &challenge, &expiresAt)
	if err != nil {
		h.tokenError(w, "invalid_grant", "unknown or already-used code")
		return
	}
	if time.Now().After(expiresAt) {
		h.tokenError(w, "invalid_grant", "code expired")
		return
	}
	// OAuth 2.1 requires a public client to send client_id on the code grant,
	// and the redirect_uri must match the one the code was issued with. Both are
	// mandatory here, not just checked when present.
	if p["client_id"] == "" || p["client_id"] != clientID {
		h.tokenError(w, "invalid_grant", "client_id mismatch")
		return
	}
	if p["redirect_uri"] == "" || p["redirect_uri"] != redirectURI {
		h.tokenError(w, "invalid_grant", "redirect_uri mismatch")
		return
	}

	// PKCE S256: challenge == b64url(sha256(verifier))
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		h.tokenError(w, "invalid_grant", "PKCE verification failed")
		return
	}

	var clientName string
	h.Store.Q().QueryRow(ctx, `SELECT name FROM oauth_clients WHERE id = $1`, clientID).Scan(&clientName)
	if clientName == "" {
		clientName = "OAuth client"
	}

	access, err := mcp.NewToken("inbx_at")
	if err != nil {
		h.tokenError(w, "server_error", "failed to create token")
		return
	}
	refresh, err := mcp.NewToken("inbx_rt")
	if err != nil {
		h.tokenError(w, "server_error", "failed to create token")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`INSERT INTO agent_tokens (org_id, user_id, kind, name, token_hash, refresh_hash, expires_at)
		 VALUES ($1, $2, 'oauth', $3, $4, $5, $6)`,
		orgID, userID, clientName, mcp.HashToken(access), mcp.HashToken(refresh),
		time.Now().Add(accessTokenTTL),
	); err != nil {
		h.tokenError(w, "server_error", "failed to store token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  access,
		"token_type":    "bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         "inboxes",
	})
}

func (h *OAuthHandler) tokenFromRefresh(w http.ResponseWriter, r *http.Request, p map[string]string) {
	refresh := p["refresh_token"]
	if refresh == "" {
		h.tokenError(w, "invalid_request", "refresh_token is required")
		return
	}

	ctx := r.Context()
	refreshHash := mcp.HashToken(refresh)

	// Reuse detection: a token that matches a superseded (rotated-away) hash is a
	// replay. It means the token leaked. Revoke that token so neither the thief
	// nor the victim can use the chain again, per OAuth 2.1 §4.14.2.
	var reuseID string
	if err := h.Store.Q().QueryRow(ctx,
		`SELECT id FROM agent_tokens WHERE prev_refresh_hash = $1 AND revoked_at IS NULL`,
		refreshHash,
	).Scan(&reuseID); err == nil && reuseID != "" {
		h.Store.Q().Exec(ctx, `UPDATE agent_tokens SET revoked_at = now() WHERE id = $1`, reuseID)
		h.tokenError(w, "invalid_grant", "refresh token reuse detected")
		return
	}

	var tokenID string
	var createdAt time.Time
	var revokedAt *time.Time
	err := h.Store.Q().QueryRow(ctx,
		`SELECT id, created_at, revoked_at FROM agent_tokens WHERE refresh_hash = $1`,
		refreshHash,
	).Scan(&tokenID, &createdAt, &revokedAt)
	if err != nil {
		h.tokenError(w, "invalid_grant", "unknown refresh token")
		return
	}
	if revokedAt != nil {
		h.tokenError(w, "invalid_grant", "token revoked")
		return
	}
	if time.Since(createdAt) > refreshTokenTTL {
		h.tokenError(w, "invalid_grant", "refresh token expired")
		return
	}

	access, err := mcp.NewToken("inbx_at")
	if err != nil {
		h.tokenError(w, "server_error", "failed to create token")
		return
	}
	// Rotate the refresh token too. OAuth 2.1 requires rotation for public
	// clients, so a leaked refresh token is good only until its next use.
	newRefresh, err := mcp.NewToken("inbx_rt")
	if err != nil {
		h.tokenError(w, "server_error", "failed to create token")
		return
	}
	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE agent_tokens SET token_hash = $1, refresh_hash = $2, prev_refresh_hash = $3, expires_at = $4 WHERE id = $5`,
		mcp.HashToken(access), mcp.HashToken(newRefresh), refreshHash, time.Now().Add(accessTokenTTL), tokenID,
	); err != nil {
		h.tokenError(w, "server_error", "failed to rotate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  access,
		"token_type":    "bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": newRefresh,
		"scope":         "inboxes",
	})
}

func (h *OAuthHandler) tokenError(w http.ResponseWriter, code, desc string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"error":             code,
		"error_description": desc,
	})
}
