//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
)

// resendCapture records /emails posts made by the rules engine.
type resendCapture struct {
	mu    sync.Mutex
	sends []map[string]interface{}
}

func (c *resendCapture) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/emails" {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			json.Unmarshal(body, &payload)
			c.mu.Lock()
			c.sends = append(c.sends, payload)
			c.mu.Unlock()
			w.Write([]byte(`{"id": "sent-1"}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
}

func (c *resendCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sends)
}

func (c *resendCapture) last() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sends) == 0 {
		return nil
	}
	return c.sends[len(c.sends)-1]
}

// seedOrgAPIKey stores an encrypted Resend key so the rules engine can send.
func seedOrgAPIKey(t *testing.T, orgID string) {
	t.Helper()
	enc, iv, tag, err := testEncSvc.Encrypt("re_test_key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2, resend_api_key_tag = $3 WHERE id = $4`,
		enc, iv, tag, orgID); err != nil {
		t.Fatal(err)
	}
}

func rulesWorker() *queue.EmailWorker {
	resendSvc := service.NewResendService(testEncSvc, testPool, "", "")
	limiter := queue.NewOrgLimiterMap(testPool, 10)
	return queue.NewEmailWorker(testStore, testRDB, resendSvc, event.NewBus(testPool, testRDB), limiter, "")
}

func TestInboundRules_ForwardAndAutoReply(t *testing.T) {
	orgID, userID := seedOrg(t, "rules-org", "rules@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "rules.test")
	aliasID := seedAlias(t, orgID, domainID, "support@rules.test", "Support")
	seedOrgAPIKey(t, orgID)

	capture := &resendCapture{}
	srv := capture.server()
	t.Cleanup(srv.Close)
	restore := service.SetResendBaseURLForTest(srv.URL)
	t.Cleanup(restore)

	ctx := context.Background()
	if _, err := testPool.Exec(ctx,
		`INSERT INTO forwarding_rules (org_id, user_id, alias_id, forward_to) VALUES ($1, $2, $3, $4)`,
		orgID, userID, aliasID, "backup@elsewhere.com"); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO auto_replies (org_id, user_id, alias_id, subject, body_html) VALUES ($1, $2, $3, 'Out of office', '<p>Back Monday</p>')`,
		orgID, userID, aliasID); err != nil {
		t.Fatalf("insert auto-reply: %v", err)
	}

	w := rulesWorker()
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"support@rules.test"},
		"customer@example.com", "Need help", "<p>question</p>", "question", false, false, false)

	if capture.count() != 2 {
		t.Fatalf("expected 2 sends (forward + auto-reply), got %d", capture.count())
	}

	// Repeat from the same sender: the forward fires again, the auto-reply
	// is rate-limited to one per 24 hours.
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"support@rules.test"},
		"customer@example.com", "Need help again", "<p>more</p>", "more", false, false, false)
	if capture.count() != 3 {
		t.Fatalf("expected 3 sends after repeat (auto-reply rate-limited), got %d", capture.count())
	}

	// Spam, bounces, and automated senders trigger nothing.
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"support@rules.test"},
		"customer2@example.com", "spam", "x", "x", true, false, false)
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"support@rules.test"},
		"noreply@shop.com", "auto", "x", "x", false, false, false)
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"support@rules.test"},
		"customer3@example.com", "auto-submitted", "x", "x", false, false, true)
	if capture.count() != 3 {
		t.Fatalf("expected no sends for spam/automated senders, got %d", capture.count())
	}
}

func TestInboundRules_PolicyDisablesForwarding(t *testing.T) {
	orgID, userID := seedOrg(t, "rules-policy-org", "rules-policy@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "rules-policy.test")
	aliasID := seedAlias(t, orgID, domainID, "sales@rules-policy.test", "Sales")
	seedOrgAPIKey(t, orgID)

	capture := &resendCapture{}
	srv := capture.server()
	t.Cleanup(srv.Close)
	restore := service.SetResendBaseURLForTest(srv.URL)
	t.Cleanup(restore)

	ctx := context.Background()
	testPool.Exec(ctx, `UPDATE orgs SET forwarding_enabled = false, auto_reply_enabled = false WHERE id = $1`, orgID)
	testPool.Exec(ctx,
		`INSERT INTO forwarding_rules (org_id, user_id, alias_id, forward_to) VALUES ($1, $2, $3, 'x@y.com')`,
		orgID, userID, aliasID)

	w := rulesWorker()
	w.ApplyInboundRulesForTest(ctx, orgID, []string{"sales@rules-policy.test"},
		"someone@example.com", "Hi", "<p>hi</p>", "hi", false, false, false)
	if capture.count() != 0 {
		t.Fatalf("expected no sends with policy off, got %d", capture.count())
	}
}

func TestForwardingRuleAuthz(t *testing.T) {
	orgID, adminID := seedOrg(t, "rules-authz-org", "rules-authz@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "rules-authz.test")
	aliasID := seedAlias(t, orgID, domainID, "team@rules-authz.test", "Team")

	// A member with no alias assignment cannot create a rule for it.
	memberID := seedUser(t, orgID, "member-authz@test.io", "member")

	h := &handler.RuleHandler{Store: testStore}
	req := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "me@elsewhere.com"}))
	req = withClaims(req, memberID, orgID, "member")
	w := httptest.NewRecorder()
	h.CreateForwardingRule(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unassigned member, got %d: %s", w.Code, w.Body.String())
	}

	// Assign the alias: now the member can create the rule.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO alias_users (alias_id, user_id, can_send_as) VALUES ($1, $2, true)`,
		aliasID, memberID); err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "me@elsewhere.com"}))
	req2 = withClaims(req2, memberID, orgID, "member")
	w2 := httptest.NewRecorder()
	h.CreateForwardingRule(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for assigned member, got %d: %s", w2.Code, w2.Body.String())
	}

	// The admin can create rules for any alias without an assignment.
	req3 := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "admin@elsewhere.com"}))
	req3 = withClaims(req3, adminID, orgID, "admin")
	w3 := httptest.NewRecorder()
	h.CreateForwardingRule(w3, req3)
	if w3.Code != http.StatusCreated {
		t.Fatalf("expected 201 for admin, got %d: %s", w3.Code, w3.Body.String())
	}

	// Self-forward is rejected.
	req4 := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "team@rules-authz.test"}))
	req4 = withClaims(req4, adminID, orgID, "admin")
	w4 := httptest.NewRecorder()
	h.CreateForwardingRule(w4, req4)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-forward, got %d: %s", w4.Code, w4.Body.String())
	}
}

func TestForwardingRuleExternalPolicy(t *testing.T) {
	orgID, adminID := seedOrg(t, "rules-ext-org", "rules-ext@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "rules-ext.test")
	aliasID := seedAlias(t, orgID, domainID, "ops@rules-ext.test", "Ops")

	ctx := context.Background()
	testPool.Exec(ctx, `UPDATE orgs SET external_forwarding_allowed = false WHERE id = $1`, orgID)

	h := &handler.RuleHandler{Store: testStore}
	// External target blocked.
	req := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "out@gmail.com"}))
	req = withClaims(req, adminID, orgID, "admin")
	w := httptest.NewRecorder()
	h.CreateForwardingRule(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for external target, got %d: %s", w.Code, w.Body.String())
	}

	// Same-domain target allowed.
	req2 := httptest.NewRequest(http.MethodPost, "/api/forwarding-rules",
		jsonBody(map[string]string{"alias_id": aliasID, "forward_to": "other@rules-ext.test"}))
	req2 = withClaims(req2, adminID, orgID, "admin")
	w2 := httptest.NewRecorder()
	h.CreateForwardingRule(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for internal target, got %d: %s", w2.Code, w2.Body.String())
	}
}
