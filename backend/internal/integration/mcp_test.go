//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/mcp"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/router"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/ws"
)

const mcpTestSecret = "mcp-test-secret"

// newFullRouter builds the real chi router with the MCP server mounted —
// the same wiring main.go uses.
func newFullRouter(t *testing.T) (http.Handler, *mcp.Server) {
	t.Helper()
	resendSvc := service.NewResendService(testEncSvc, testPool, "", "")
	bus := event.NewBus(testPool, testRDB)
	wsHub := ws.NewHub(testRDB, testStore, 3, time.Minute)
	limiterMap := queue.NewOrgLimiterMap(testPool, 2)
	mcpSrv := mcp.NewServer(testPool, mcpTestSecret, "http://api.test")

	r := router.New(testPool, testRDB, testEncSvc, resendSvc, bus, wsHub, limiterMap, router.Config{
		Secret:    mcpTestSecret,
		AppURL:    "http://app.test",
		PublicURL: "http://api.test",
		AppCtx:    context.Background(),
		MCP:       mcpSrv,
	})
	return r, mcpSrv
}

// webRequest performs an authenticated browser-style request against the router.
func webRequest(t *testing.T, h http.Handler, userID, orgID, role, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	token, _, err := middleware.GenerateToken(mcpTestSecret, userID, orgID, role)
	if err != nil {
		t.Fatal(err)
	}
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, jsonBody(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// mcpCall sends one JSON-RPC message to /mcp with a bearer token.
func mcpCall(t *testing.T, h http.Handler, bearer, method string, params any) map[string]any {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		msg["params"] = params
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonBody(msg))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mcp %s: HTTP %d: %s", method, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	parseJSON(t, rec, &resp)
	return resp
}

// toolResultText extracts the text payload of a tools/call result.
func toolResultText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatalf("no result in response: %v", resp)
	}
	isErr, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content in tool result: %v", result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text, isErr
}

func createAgentKey(t *testing.T, h http.Handler, userID, orgID, role string) string {
	t.Helper()
	rec := webRequest(t, h, userID, orgID, role, http.MethodPost, "/api/agent-keys", map[string]string{"name": "test key"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key: HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Key string `json:"key"`
	}
	parseJSON(t, rec, &out)
	if !strings.HasPrefix(out.Key, "inbx_k_") {
		t.Fatalf("unexpected key format: %q", out.Key)
	}
	return out.Key
}

// oauthAccessToken runs the full consent flow and returns an OAuth access
// token (kind='oauth') for the given user, using a loopback redirect.
func oauthAccessToken(t *testing.T, h http.Handler, userID, orgID, role string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/register", jsonBody(map[string]any{
		"client_name":   "Test CLI",
		"redirect_uris": []string{"http://127.0.0.1:0/callback"},
	}))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	var client struct {
		ClientID string `json:"client_id"`
	}
	parseJSON(t, rec, &client)

	verifier := "test-verifier-abc123"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	rec = webRequest(t, h, userID, orgID, role, http.MethodPost, "/api/oauth/approve", map[string]any{
		"client_id":             client.ClientID,
		"redirect_uri":          "http://127.0.0.1:0/callback",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	var approved struct {
		RedirectTo string `json:"redirect_to"`
	}
	parseJSON(t, rec, &approved)
	redirectURL, _ := url.Parse(approved.RedirectTo)
	code := redirectURL.Query().Get("code")

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("client_id", client.ClientID)
	form.Set("redirect_uri", "http://127.0.0.1:0/callback")
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	parseJSON(t, rec, &tok)
	if tok.AccessToken == "" {
		t.Fatalf("no access token: %s", rec.Body.String())
	}
	return tok.AccessToken
}

func TestMCPAuthRequired(t *testing.T) {
	h, _ := newFullRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonBody(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "oauth-protected-resource") {
		t.Errorf("expected WWW-Authenticate resource metadata, got %q", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestMCPInitializeAndToolListing(t *testing.T) {
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("mcp-%s", t.Name()), fmt.Sprintf("mcp-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	memberID := seedUser(t, orgID, fmt.Sprintf("mcp-member-%s@test.com", t.Name()), "member")

	adminKey := createAgentKey(t, h, adminID, orgID, "admin")
	memberKey := createAgentKey(t, h, memberID, orgID, "member")

	// initialize
	resp := mcpCall(t, h, adminKey, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	result, _ := resp["result"].(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("unexpected protocol version: %v", result["protocolVersion"])
	}
	if instr, _ := result["instructions"].(string); !strings.Contains(instr, "drafts") {
		t.Errorf("instructions missing draft policy")
	}

	// tools/list role filtering
	listTools := func(key string) []string {
		resp := mcpCall(t, h, key, "tools/list", nil)
		result, _ := resp["result"].(map[string]any)
		tools, _ := result["tools"].([]any)
		names := []string{}
		for _, tl := range tools {
			m, _ := tl.(map[string]any)
			names = append(names, m["name"].(string))
		}
		return names
	}
	adminTools := listTools(adminKey)
	memberTools := listTools(memberKey)

	has := func(list []string, name string) bool {
		for _, n := range list {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has(adminTools, "invite_user") || !has(adminTools, "create_draft") {
		t.Errorf("admin tool list incomplete: %v", adminTools)
	}
	if has(memberTools, "invite_user") {
		t.Errorf("member must not see admin tools: %v", memberTools)
	}
	if !has(memberTools, "create_draft") {
		t.Errorf("member tool list incomplete: %v", memberTools)
	}
}

func TestMCPDraftLifecycleAndSendToggle(t *testing.T) {
	ctx := context.Background()
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("mcpdraft-%s", t.Name()), fmt.Sprintf("mcpdraft-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainName := fmt.Sprintf("mcpdraft-%s.example.com", strings.ToLower(t.Name()))
	seedDomain(t, orgID, domainName)

	key := createAgentKey(t, h, adminID, orgID, "admin")

	// create_draft resolves the domain by name and stores the body.
	resp := mcpCall(t, h, key, "tools/call", map[string]any{
		"name": "create_draft",
		"arguments": map[string]any{
			"domain":       domainName,
			"from_address": "hello@" + domainName,
			"to":           []string{"dest@example.com"},
			"subject":      "MCP test draft",
			"body_text":    "Written by an agent for human review.",
		},
	})
	text, isErr := toolResultText(t, resp)
	if isErr {
		t.Fatalf("create_draft failed: %s", text)
	}
	var created struct {
		DraftID string `json:"draft_id"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil || created.DraftID == "" {
		t.Fatalf("no draft_id in result: %s", text)
	}

	// The draft is visible through the normal API as the same user.
	rec := webRequest(t, h, adminID, orgID, "admin", http.MethodGet, "/api/drafts", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), created.DraftID) {
		t.Fatalf("draft not listed via web API: %d %s", rec.Code, rec.Body.String())
	}

	// update_draft
	resp = mcpCall(t, h, key, "tools/call", map[string]any{
		"name": "update_draft",
		"arguments": map[string]any{
			"draft_id": created.DraftID,
			"subject":  "MCP test draft v2",
		},
	})
	if text, isErr := toolResultText(t, resp); isErr {
		t.Fatalf("update_draft failed: %s", text)
	}

	// send_draft with the org toggle off must refuse.
	resp = mcpCall(t, h, key, "tools/call", map[string]any{
		"name":      "send_draft",
		"arguments": map[string]any{"draft_id": created.DraftID},
	})
	text, isErr = toolResultText(t, resp)
	if !isErr || !strings.Contains(text, "disabled") {
		t.Fatalf("expected send to be blocked by toggle, got isErr=%v text=%s", isErr, text)
	}

	// Flip the org switch and send for real: the email row and job appear.
	if _, err := testPool.Exec(ctx, "UPDATE orgs SET agent_send_enabled = true WHERE id = $1", orgID); err != nil {
		t.Fatal(err)
	}
	resp = mcpCall(t, h, key, "tools/call", map[string]any{
		"name":      "send_draft",
		"arguments": map[string]any{"draft_id": created.DraftID},
	})
	text, isErr = toolResultText(t, resp)
	if isErr {
		t.Fatalf("send_draft failed with toggle on: %s", text)
	}
	var emailCount int
	testPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM emails WHERE org_id = $1 AND direction = 'outbound'", orgID,
	).Scan(&emailCount)
	if emailCount != 1 {
		t.Errorf("expected 1 outbound email, got %d", emailCount)
	}
}

func TestMCPMemberVisibilityInheritance(t *testing.T) {
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("mcpvis-%s", t.Name()), fmt.Sprintf("mcpvis-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, fmt.Sprintf("mcpvis-%s.example.com", strings.ToLower(t.Name())))
	memberID := seedUser(t, orgID, fmt.Sprintf("mcpvis-member-%s@test.com", t.Name()), "member")

	// A thread labeled for someone else's alias must be invisible to the member.
	threadID := seedThread(t, orgID, adminID, domainID, "Private admin thread")
	ctx := context.Background()
	testStore.AddLabel(ctx, threadID, orgID, "alias:ceo@example.com")

	memberKey := createAgentKey(t, h, memberID, orgID, "member")
	resp := mcpCall(t, h, memberKey, "tools/call", map[string]any{
		"name":      "get_thread",
		"arguments": map[string]any{"thread_id": threadID},
	})
	text, isErr := toolResultText(t, resp)
	if !isErr {
		t.Fatalf("member read a thread outside their aliases via MCP: %s", text)
	}
}

func TestMCPModifyThread(t *testing.T) {
	ctx := context.Background()
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("mcpmod-%s", t.Name()), fmt.Sprintf("mcpmod-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, fmt.Sprintf("mcpmod-%s.example.com", strings.ToLower(t.Name())))
	threadID := seedThread(t, orgID, adminID, domainID, "Modify me")

	key := createAgentKey(t, h, adminID, orgID, "admin")

	act := func(action string, extra map[string]any) (string, bool) {
		argsMap := map[string]any{"thread_id": threadID, "action": action}
		for k, v := range extra {
			argsMap[k] = v
		}
		resp := mcpCall(t, h, key, "tools/call", map[string]any{
			"name": "modify_thread", "arguments": argsMap,
		})
		return toolResultText(t, resp)
	}

	hasLabel := func(label string) bool {
		return testStore.HasLabel(ctx, threadID, orgID, label)
	}

	if text, isErr := act("archive", nil); isErr {
		t.Fatalf("archive: %s", text)
	}
	if hasLabel("inbox") {
		t.Error("archive did not remove inbox label")
	}

	if text, isErr := act("restore", nil); isErr {
		t.Fatalf("restore: %s", text)
	}
	if !hasLabel("inbox") {
		t.Error("restore did not re-add inbox label")
	}

	if text, isErr := act("star", nil); isErr {
		t.Fatalf("star: %s", text)
	}
	if !hasLabel("starred") {
		t.Error("star did not add label")
	}

	if text, isErr := act("mute", nil); isErr {
		t.Fatalf("mute: %s", text)
	}
	if !hasLabel("muted") {
		t.Error("mute did not add label")
	}
	// Idempotent: mute again must not toggle off.
	act("mute", nil)
	if !hasLabel("muted") {
		t.Error("second mute toggled the label off — must be idempotent")
	}

	if text, isErr := act("snooze", map[string]any{"snooze_until": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)}); isErr {
		t.Fatalf("snooze: %s", text)
	}
	var snoozedUntil *time.Time
	testPool.QueryRow(ctx, "SELECT snoozed_until FROM threads WHERE id = $1", threadID).Scan(&snoozedUntil)
	if snoozedUntil == nil {
		t.Error("snooze did not set snoozed_until")
	}

	if text, isErr := act("spam", nil); isErr {
		t.Fatalf("spam: %s", text)
	}
	if !hasLabel("spam") {
		t.Error("spam did not add label")
	}

	// A member without alias access must be blocked from modifying it.
	memberID := seedUser(t, orgID, fmt.Sprintf("mcpmod-m-%s@test.com", t.Name()), "member")
	testStore.AddLabel(ctx, threadID, orgID, "alias:someoneelse@example.com")
	memberKey := createAgentKey(t, h, memberID, orgID, "member")
	resp := mcpCall(t, h, memberKey, "tools/call", map[string]any{
		"name": "modify_thread", "arguments": map[string]any{"thread_id": threadID, "action": "trash"},
	})
	if text, isErr := toolResultText(t, resp); !isErr {
		t.Fatalf("member modified a thread outside their aliases: %s", text)
	}
}

func TestMCPRevokedKeyRejected(t *testing.T) {
	ctx := context.Background()
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("mcprev-%s", t.Name()), fmt.Sprintf("mcprev-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	key := createAgentKey(t, h, adminID, orgID, "admin")
	if _, err := testPool.Exec(ctx,
		"UPDATE agent_tokens SET revoked_at = now() WHERE org_id = $1", orgID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", jsonBody(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "ping",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key must get 401, got %d", rec.Code)
	}
}

func TestOAuthFullFlow(t *testing.T) {
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("oauth-%s", t.Name()), fmt.Sprintf("oauth-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Discovery metadata is public.
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "authorization_endpoint") {
		t.Fatalf("AS metadata: %d %s", rec.Code, rec.Body.String())
	}

	// 1. Dynamic client registration
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/register", jsonBody(map[string]any{
		"client_name":   "Test MCP Client",
		"redirect_uris": []string{"http://localhost:33418/callback"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	var client struct {
		ClientID string `json:"client_id"`
	}
	parseJSON(t, rec, &client)

	// 2. Consent: the logged-in user approves; PKCE S256
	verifier := "test-verifier-string-with-enough-entropy-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	rec = webRequest(t, h, adminID, orgID, "admin", http.MethodPost, "/api/oauth/approve", map[string]any{
		"client_id":             client.ClientID,
		"redirect_uri":          "http://localhost:33418/callback",
		"state":                 "xyz",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	var approved struct {
		RedirectTo string `json:"redirect_to"`
	}
	parseJSON(t, rec, &approved)
	redirectURL, err := url.Parse(approved.RedirectTo)
	if err != nil {
		t.Fatal(err)
	}
	code := redirectURL.Query().Get("code")
	if code == "" || redirectURL.Query().Get("state") != "xyz" {
		t.Fatalf("bad redirect: %s", approved.RedirectTo)
	}

	// 3. Token exchange, form-encoded like real OAuth clients
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("client_id", client.ClientID)
	form.Set("redirect_uri", "http://localhost:33418/callback")
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token: %d %s", rec.Code, rec.Body.String())
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	parseJSON(t, rec, &tok)
	if !strings.HasPrefix(tok.AccessToken, "inbx_at_") {
		t.Fatalf("unexpected access token: %q", tok.AccessToken)
	}

	// 4. The access token works on /mcp
	resp := mcpCall(t, h, tok.AccessToken, "tools/list", nil)
	if resp["result"] == nil {
		t.Fatalf("tools/list with OAuth token failed: %v", resp)
	}

	// 5. A code is single-use
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("authorization code was accepted twice")
	}

	// 6. Refresh rotates the access token
	form = url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", tok.RefreshToken)
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	var tok2 struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	parseJSON(t, rec, &tok2)
	if tok2.AccessToken == tok.AccessToken {
		t.Fatal("refresh must rotate the access token")
	}
	if mcpCall(t, h, tok2.AccessToken, "ping", nil)["result"] == nil {
		t.Fatal("rotated token does not work")
	}

	// 7. Reuse detection: replaying the old (rotated-away) refresh token is a
	// replay. The server must reject it and revoke the whole chain.
	form = url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", tok.RefreshToken)
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("a superseded refresh token must be rejected")
	}

	// 8. The reuse revokes the chain, so the current refresh token also stops.
	form = url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", tok2.RefreshToken)
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("refresh chain must be revoked after a reuse is detected")
	}
}

func TestAgentKeyExchange(t *testing.T) {
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("exch-%s", t.Name()), fmt.Sprintf("exch-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// An OAuth access token from the consent flow exchanges for a durable key.
	accessToken := oauthAccessToken(t, h, adminID, orgID, "admin")
	req := httptest.NewRequest(http.MethodPost, "/api/agent-keys/exchange", jsonBody(map[string]string{"name": "cli-test"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("exchange: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	parseJSON(t, rec, &out)
	if !strings.HasPrefix(out.Key, "inbx_k_") || out.Key == accessToken {
		t.Fatalf("bad exchanged key: %q", out.Key)
	}
	if out.Name != "cli-test" {
		t.Errorf("name not stored: %q", out.Name)
	}

	// The exchanged key works on /mcp.
	if mcpCall(t, h, out.Key, "ping", nil)["result"] == nil {
		t.Fatal("exchanged key rejected by /mcp")
	}

	// A plain API key cannot be exchanged — only OAuth tokens can. This stops
	// one key from cloning itself into keys that survive its revocation.
	apiKey := createAgentKey(t, h, adminID, orgID, "admin")
	req = httptest.NewRequest(http.MethodPost, "/api/agent-keys/exchange", jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for API-key exchange, got %d %s", rec.Code, rec.Body.String())
	}

	// Garbage bearers get 401.
	req = httptest.NewRequest(http.MethodPost, "/api/agent-keys/exchange", jsonBody(map[string]string{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer inbx_at_garbage")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad bearer, got %d", rec.Code)
	}
}

func TestOAuthRemoteRedirectRejected(t *testing.T) {
	h, _ := newFullRouter(t)

	// A remote http(s) redirect must be refused at registration.
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/register", jsonBody(map[string]any{
		"client_name":   "Evil Client",
		"redirect_uris": []string{"https://evil.tld/cb"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for remote redirect, got %d %s", rec.Code, rec.Body.String())
	}

	// A loopback redirect must still register.
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/register", jsonBody(map[string]any{
		"client_name":   "Good Client",
		"redirect_uris": []string{"http://127.0.0.1:52001/callback"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for loopback redirect, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestOAuthPKCEMismatchRejected(t *testing.T) {
	h, _ := newFullRouter(t)
	orgID, adminID := seedOrg(t, fmt.Sprintf("pkce-%s", t.Name()), fmt.Sprintf("pkce-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	req := httptest.NewRequest(http.MethodPost, "/api/oauth/register", jsonBody(map[string]any{
		"client_name":   "PKCE Client",
		"redirect_uris": []string{"http://localhost:1/cb"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var client struct {
		ClientID string `json:"client_id"`
	}
	parseJSON(t, rec, &client)

	sum := sha256.Sum256([]byte("right-verifier"))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	rec = webRequest(t, h, adminID, orgID, "admin", http.MethodPost, "/api/oauth/approve", map[string]any{
		"client_id":             client.ClientID,
		"redirect_uri":          "http://localhost:1/cb",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	var approved struct {
		RedirectTo string `json:"redirect_to"`
	}
	parseJSON(t, rec, &approved)
	redirectURL, _ := url.Parse(approved.RedirectTo)
	code := redirectURL.Query().Get("code")

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", "wrong-verifier")
	req = httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("token issued despite PKCE mismatch")
	}
}
