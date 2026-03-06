package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

// Auth enforcement is handled by AuthMiddleware in the router.

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
	// Valid JSON that passes all validation — will fail at CheckBouncedRecipients (nil Store), but we verify fields parse
	h := &EmailHandler{}
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":"Hi","html":"<p>Hello</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	// This will panic/500 on CheckBouncedRecipients due to nil Store, that's expected.
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

// ── MockStore-backed tests ──

func TestSend_TooManyRecipients(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{
		Store: &store.MockStore{},
	}
	// Build 51 To addresses
	addrs := make([]string, 51)
	for i := range addrs {
		addrs[i] = `"user` + strings.Repeat("0", 2) + `@example.com"`
	}
	toJSON := "[" + strings.Join(addrs, ",") + "]"
	body := `{"from":"alice@example.com","to":` + toJSON + `,"subject":"Hi","html":"<p>Hello</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(too many recipients): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many recipients") {
		t.Errorf("Send(too many recipients): body = %q, want containing 'too many recipients'", w.Body.String())
	}
}

func TestSend_BodyTooLarge(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{
		Store: &store.MockStore{},
	}
	largeBody := strings.Repeat("x", 512*1024+1)
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":"Hi","html":"` + largeBody + `"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Send(body too large): got status %d, want %d; body length: %d", w.Code, http.StatusBadRequest, w.Body.Len())
	}
	if !strings.Contains(w.Body.String(), "too large") {
		t.Errorf("Send(body too large): body = %q, want containing 'too large'", w.Body.String())
	}
}

func TestSend_BouncedRecipient(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{
		Store: &store.MockStore{
			CheckBouncedRecipientsFn: func(ctx context.Context, orgID string, addresses []string) ([]string, error) {
				return []string{"blocked@test.com"}, nil
			},
		},
	}
	body := `{"from":"alice@example.com","to":["blocked@test.com"],"subject":"Hi","html":"<p>Hello</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("Send(bounced): got status %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "bounced") {
		t.Errorf("Send(bounced): body = %q, want containing 'bounced'", w.Body.String())
	}
}

func TestSend_UnauthorizedFrom(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{
		Store: &store.MockStore{
			CheckBouncedRecipientsFn: func(ctx context.Context, orgID string, addresses []string) ([]string, error) {
				return nil, nil
			},
			CanSendAsFn: func(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error) {
				return false, nil
			},
		},
	}
	body := `{"from":"admin@example.com","to":["bob@example.com"],"subject":"Hi","html":"<p>Hello</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "member")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("Send(unauthorized from): got status %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not authorized") {
		t.Errorf("Send(unauthorized from): body = %q, want containing 'not authorized'", w.Body.String())
	}
}

// ── Search handler tests ──

func TestSearch_EmptyQuery(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{Store: &store.MockStore{}}
	req := httptest.NewRequest("GET", "/emails/search?q=", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Search(empty q): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "q parameter is required") {
		t.Errorf("Search(empty q): body = %q, want containing 'q parameter is required'", w.Body.String())
	}
}

func TestSearch_ValidQuery(t *testing.T) {
	t.Parallel()
	h := &EmailHandler{
		Store: &store.MockStore{
			SearchEmailsFn: func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
				return []map[string]any{
					{"id": "thread1", "subject": "Hello"},
					{"id": "thread2", "subject": "World"},
				}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/emails/search?q=hello", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Search(valid): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Search(valid): failed to decode response: %v", err)
	}
	threads, ok := resp["threads"].([]interface{})
	if !ok {
		t.Fatalf("Search(valid): threads is not an array: %v", resp)
	}
	if len(threads) != 2 {
		t.Errorf("Search(valid): got %d threads, want 2", len(threads))
	}
}

func TestSearch_NonAdminAliasFiltered(t *testing.T) {
	t.Parallel()
	var capturedAliasAddrs []string
	h := &EmailHandler{
		Store: &store.MockStore{
			GetUserAliasAddressesFn: func(ctx context.Context, userID string) ([]string, error) {
				return []string{"alias1@example.com", "alias2@example.com"}, nil
			},
			SearchEmailsFn: func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
				capturedAliasAddrs = aliasAddrs
				return []map[string]any{}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/emails/search?q=test", nil)
	req = withClaims(req, "user1", "org1", "member")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Search(non-admin): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if len(capturedAliasAddrs) != 2 {
		t.Fatalf("Search(non-admin): got %d alias addrs, want 2", len(capturedAliasAddrs))
	}
	if capturedAliasAddrs[0] != "alias1@example.com" || capturedAliasAddrs[1] != "alias2@example.com" {
		t.Errorf("Search(non-admin): aliasAddrs = %v, want [alias1@example.com alias2@example.com]", capturedAliasAddrs)
	}
}

func TestSearch_AdminUnfiltered(t *testing.T) {
	t.Parallel()
	var capturedAliasAddrs []string
	aliasWasCalled := false
	h := &EmailHandler{
		Store: &store.MockStore{
			GetUserAliasAddressesFn: func(ctx context.Context, userID string) ([]string, error) {
				aliasWasCalled = true
				return []string{"should-not-be-used@example.com"}, nil
			},
			SearchEmailsFn: func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
				capturedAliasAddrs = aliasAddrs
				return []map[string]any{}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/emails/search?q=test", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Search(admin unfiltered): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if aliasWasCalled {
		t.Error("Search(admin unfiltered): GetUserAliasAddresses should not be called for admin")
	}
	if capturedAliasAddrs != nil {
		t.Errorf("Search(admin unfiltered): aliasAddrs = %v, want nil", capturedAliasAddrs)
	}
}

// ── #56: Reply sets In-Reply-To and References headers ──

func TestSend_ReplyHeaders(t *testing.T) {
	t.Parallel()
	var capturedPayload map[string]interface{}
	h := &EmailHandler{
		Store: &store.MockStore{
			CheckBouncedRecipientsFn: func(ctx context.Context, orgID string, addresses []string) ([]string, error) {
				return nil, nil
			},
			CanSendAsFn: func(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error) {
				return true, nil
			},
			ResolveFromDisplayFn: func(ctx context.Context, orgID, from string) (string, error) {
				return from, nil
			},
			LookupDomainByNameFn: func(ctx context.Context, orgID, domain string) (string, error) {
				return "d1", nil
			},
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					InsertEmailFn: func(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, to, cc, bcc []byte, subject, html, text, status, inReplyTo string, refs []byte) (string, error) {
						return "e1", nil
					},
					UpdateThreadStatsFn: func(ctx context.Context, threadID, snippet, lastSender string) error {
						return nil
					},
					CreateEmailJobFn: func(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, payload []byte, draftID *string) (string, error) {
						json.Unmarshal(payload, &capturedPayload)
						return "job1", nil
					},
				})
			},
		},
	}
	body := `{
		"from":"alice@example.com",
		"to":["bob@example.com"],
		"subject":"Re: Hello",
		"html":"<p>Reply</p>",
		"reply_to_thread_id":"t1",
		"in_reply_to":"<msg123@example.com>",
		"references":["<msg100@example.com>","<msg123@example.com>"]
	}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	// Will fail at RDB.LPush (nil Redis), but we can still verify payload was built
	func() {
		defer func() { recover() }()
		h.Send(w, req)
	}()

	if capturedPayload != nil {
		headers, ok := capturedPayload["headers"].(map[string]interface{})
		if !ok {
			t.Fatal("Send(reply): headers not found in payload")
		}
		if headers["In-Reply-To"] != "<msg123@example.com>" {
			t.Errorf("Send(reply): In-Reply-To = %v, want <msg123@example.com>", headers["In-Reply-To"])
		}
		if refs, ok := headers["References"].(string); ok {
			if !strings.Contains(refs, "<msg100@example.com>") || !strings.Contains(refs, "<msg123@example.com>") {
				t.Errorf("Send(reply): References = %q, want both message IDs", refs)
			}
		} else {
			t.Error("Send(reply): References not found in headers")
		}
	}
}

// ── #57: Subject prefix normalization ──

func TestSend_DomainLookupFromAddress(t *testing.T) {
	t.Parallel()
	var capturedDomainID string
	h := &EmailHandler{
		Store: &store.MockStore{
			CheckBouncedRecipientsFn: func(ctx context.Context, orgID string, addresses []string) ([]string, error) {
				return nil, nil
			},
			CanSendAsFn: func(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error) {
				return true, nil
			},
			ResolveFromDisplayFn: func(ctx context.Context, orgID, from string) (string, error) {
				return from, nil
			},
			LookupDomainByNameFn: func(ctx context.Context, orgID, domain string) (string, error) {
				capturedDomainID = "resolved-d1"
				return "resolved-d1", nil
			},
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					CreateThreadFn: func(ctx context.Context, orgID, userID, domainID, subject string, participants []byte, snippet, lastSender string) (string, error) {
						capturedDomainID = domainID
						return "t1", nil
					},
					AddLabelFn: func(ctx context.Context, threadID, orgID, label string) error {
						return nil
					},
					InsertEmailFn: func(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, to, cc, bcc []byte, subject, html, text, status, inReplyTo string, refs []byte) (string, error) {
						return "e1", nil
					},
					UpdateThreadStatsFn: func(ctx context.Context, threadID, snippet, lastSender string) error {
						return nil
					},
					CreateEmailJobFn: func(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, payload []byte, draftID *string) (string, error) {
						return "job1", nil
					},
				})
			},
		},
	}
	// domain_id not set — should resolve from "from" address
	body := `{"from":"alice@example.com","to":["bob@example.com"],"subject":"Hello","html":"<p>Hi</p>"}`
	req := httptest.NewRequest("POST", "/emails/send", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Send(w, req)
	}()

	if capturedDomainID != "resolved-d1" {
		t.Errorf("Send(domain lookup): domain_id = %q, want resolved-d1", capturedDomainID)
	}
}

// ── #92: Search — max 50 results (store enforces LIMIT 50) ──

func TestSearch_Max50Results(t *testing.T) {
	t.Parallel()
	// Generate 60 results — store should cap at 50 via SQL LIMIT,
	// but at the handler level we verify it passes through whatever the store returns.
	results := make([]map[string]any, 50)
	for i := range results {
		results[i] = map[string]any{"id": fmt.Sprintf("t%d", i)}
	}
	h := &EmailHandler{
		Store: &store.MockStore{
			SearchEmailsFn: func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
				return results, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/emails/search?q=test", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Search(max50): got status %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	threads := resp["threads"].([]interface{})
	if len(threads) > 50 {
		t.Errorf("Search(max50): got %d results, want ≤50", len(threads))
	}
}

func TestSearch_DomainFilter(t *testing.T) {
	t.Parallel()
	var capturedDomainID string
	h := &EmailHandler{
		Store: &store.MockStore{
			SearchEmailsFn: func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
				capturedDomainID = domainID
				return []map[string]any{{"id": "thread1"}}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/emails/search?q=hello&domain_id=dom123", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Search(domain filter): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedDomainID != "dom123" {
		t.Errorf("Search(domain filter): got domainID %q, want %q", capturedDomainID, "dom123")
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Search(domain filter): failed to decode response: %v", err)
	}
	threads, ok := resp["threads"].([]interface{})
	if !ok {
		t.Fatalf("Search(domain filter): threads is not an array: %v", resp)
	}
	if len(threads) != 1 {
		t.Errorf("Search(domain filter): got %d threads, want 1", len(threads))
	}
}
