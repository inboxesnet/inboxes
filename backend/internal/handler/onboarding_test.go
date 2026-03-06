package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/inboxes/backend/internal/store"
)

// mockQuerier implements store.Querier for testing Q().Exec() calls.
type mockQuerier struct {
	execFn func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (q *mockQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (q *mockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (q *mockQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if q.execFn != nil {
		return q.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func TestOnboardingStatus_NoAPIKey(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{
			HasAPIKeyFn: func(ctx context.Context, orgID string) (bool, error) {
				return false, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/onboarding/status", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("OnboardingStatus(no API key): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("OnboardingStatus(no API key): failed to decode: %v", err)
	}
	if resp["step"] != "connect" {
		t.Errorf("OnboardingStatus(no API key): got step %q, want %q", resp["step"], "connect")
	}
}

func TestOnboardingStatus_NoDomains(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{
			HasAPIKeyFn: func(ctx context.Context, orgID string) (bool, error) {
				return true, nil
			},
			CountVisibleDomainsFn: func(ctx context.Context, orgID string) (int, error) {
				return 0, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/onboarding/status", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("OnboardingStatus(no domains): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("OnboardingStatus(no domains): failed to decode: %v", err)
	}
	if resp["step"] != "domains" {
		t.Errorf("OnboardingStatus(no domains): got step %q, want %q", resp["step"], "domains")
	}
}

func TestOnboardingStatus_ActiveSync(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{
			HasAPIKeyFn: func(ctx context.Context, orgID string) (bool, error) {
				return true, nil
			},
			CountVisibleDomainsFn: func(ctx context.Context, orgID string) (int, error) {
				return 1, nil
			},
			GetActiveSyncJobFn: func(ctx context.Context, orgID string) (string, string, error) {
				return "job1", "scanning", nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/onboarding/status", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("OnboardingStatus(active sync): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("OnboardingStatus(active sync): failed to decode: %v", err)
	}
	if resp["step"] != "sync" {
		t.Errorf("OnboardingStatus(active sync): got step %q, want %q", resp["step"], "sync")
	}
	if resp["sync_in_progress"] != true {
		t.Errorf("OnboardingStatus(active sync): expected sync_in_progress=true")
	}
}

func TestOnboardingStatus_Complete(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{
			HasAPIKeyFn: func(ctx context.Context, orgID string) (bool, error) {
				return true, nil
			},
			CountVisibleDomainsFn: func(ctx context.Context, orgID string) (int, error) {
				return 1, nil
			},
			GetActiveSyncJobFn: func(ctx context.Context, orgID string) (string, string, error) {
				return "", "", fmt.Errorf("no active sync job")
			},
			CountEmailsFn: func(ctx context.Context, orgID string) (int, error) {
				return 50, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/onboarding/status", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("OnboardingStatus(complete): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("OnboardingStatus(complete): failed to decode: %v", err)
	}
	// With emails present and no active sync, step should be "addresses"
	if resp["step"] != "addresses" {
		t.Errorf("OnboardingStatus(complete): got step %q, want %q", resp["step"], "addresses")
	}
}

func TestOnboardingConnect_MissingAPIKey(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{},
	}

	body := `{"api_key":""}`
	req := httptest.NewRequest("POST", "/onboarding/connect", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Connect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("OnboardingConnect(missing key): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "api_key is required") {
		t.Errorf("OnboardingConnect(missing key): body = %q", w.Body.String())
	}
}

// ── #26: Select domains → calls SelectDomains ──

func TestOnboardingSelectDomains_Success(t *testing.T) {
	t.Parallel()

	selectCalled := false
	h := &OnboardingHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					SelectDomainsFn: func(ctx context.Context, orgID string, domainIDs []string) error {
						selectCalled = true
						if len(domainIDs) != 2 {
							t.Errorf("SelectDomains: got %d domains, want 2", len(domainIDs))
						}
						return nil
					},
				})
			},
		},
	}

	body := `{"domain_ids":["d1","d2"]}`
	req := httptest.NewRequest("POST", "/onboarding/domains", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SelectDomains(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SelectDomains: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !selectCalled {
		t.Error("SelectDomains: store.SelectDomains was not called")
	}
}

// ── #28: Status during sync shows sync step ──

func TestOnboardingStatus_SyncInProgress(t *testing.T) {
	t.Parallel()

	h := &OnboardingHandler{
		Store: &store.MockStore{
			HasAPIKeyFn: func(ctx context.Context, orgID string) (bool, error) {
				return true, nil
			},
			CountVisibleDomainsFn: func(ctx context.Context, orgID string) (int, error) {
				return 2, nil
			},
			GetActiveSyncJobFn: func(ctx context.Context, orgID string) (string, string, error) {
				return "job1", "aliases_ready", nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/onboarding/status", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("OnboardingStatus(aliases_ready): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["step"] != "addresses" {
		t.Errorf("OnboardingStatus(aliases_ready): got step %q, want %q", resp["step"], "addresses")
	}
	if resp["sync_in_progress"] != true {
		t.Errorf("OnboardingStatus(aliases_ready): expected sync_in_progress=true")
	}
}

// ── #29: Setup addresses with "individual" type ──

func TestOnboardingSetupAddresses_Individual(t *testing.T) {
	t.Parallel()

	setupCalled := 0
	h := &OnboardingHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					SetupAddressFn: func(ctx context.Context, orgID, userID, address, addrType, name string) error {
						setupCalled++
						if addrType != "individual" {
							t.Errorf("SetupAddress: type = %q, want individual", addrType)
						}
						return nil
					},
				})
			},
		},
	}

	body := `{"addresses":[{"address":"alice@example.com","type":"individual","name":"Alice"}]}`
	req := httptest.NewRequest("POST", "/onboarding/addresses", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetupAddresses(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupAddresses: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if setupCalled != 1 {
		t.Errorf("SetupAddresses: setupCalled = %d, want 1", setupCalled)
	}
}

// ── #31: Setup addresses with "skip" type ──

func TestOnboardingSetupAddresses_Skip(t *testing.T) {
	t.Parallel()

	var capturedType string
	h := &OnboardingHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					SetupAddressFn: func(ctx context.Context, orgID, userID, address, addrType, name string) error {
						capturedType = addrType
						return nil
					},
				})
			},
		},
	}

	body := `{"addresses":[{"address":"noreply@example.com","type":"skip","name":""}]}`
	req := httptest.NewRequest("POST", "/onboarding/addresses", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetupAddresses(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupAddresses(skip): got status %d, want %d", w.Code, http.StatusOK)
	}
	if capturedType != "skip" {
		t.Errorf("SetupAddresses(skip): type = %q, want skip", capturedType)
	}
}

// ── #32: Duplicate address (ON CONFLICT) is idempotent ──

func TestOnboardingSetupAddresses_DuplicateIdempotent(t *testing.T) {
	t.Parallel()

	setupCalls := 0
	h := &OnboardingHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					SetupAddressFn: func(ctx context.Context, orgID, userID, address, addrType, name string) error {
						setupCalls++
						return nil // ON CONFLICT DO NOTHING — no error on duplicate
					},
				})
			},
		},
	}

	body := `{"addresses":[{"address":"alice@example.com","type":"individual","name":"Alice"},{"address":"alice@example.com","type":"individual","name":"Alice"}]}`
	req := httptest.NewRequest("POST", "/onboarding/addresses", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetupAddresses(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupAddresses(duplicate): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if setupCalls != 2 {
		t.Errorf("SetupAddresses(duplicate): setupCalls = %d, want 2", setupCalls)
	}

	// Call again — should still succeed (idempotent)
	req2 := httptest.NewRequest("POST", "/onboarding/addresses", strings.NewReader(`{"addresses":[{"address":"alice@example.com","type":"individual","name":"Alice"}]}`))
	req2 = withClaims(req2, "user1", "org1", "admin")
	w2 := httptest.NewRecorder()

	h.SetupAddresses(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("SetupAddresses(duplicate repeat): got status %d, want %d", w2.Code, http.StatusOK)
	}
}

// ── Webhook skip enables auto-poll ──

func TestSetupWebhook_LocalhostEnablesAutoPoll(t *testing.T) {
	t.Parallel()

	var capturedOrgID string
	pollEnabled := false

	mq := &mockQuerier{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			pollEnabled = true
			if len(args) > 0 {
				capturedOrgID, _ = args[0].(string)
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	h := &OnboardingHandler{
		Store:     &store.MockStore{QFn: func() store.Querier { return mq }},
		PublicURL: "http://localhost:8080",
	}

	req := httptest.NewRequest("POST", "/onboarding/webhook", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetupWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupWebhook(localhost): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["webhook_skipped"] != true {
		t.Errorf("expected webhook_skipped=true, got %v", resp["webhook_skipped"])
	}
	if !pollEnabled {
		t.Error("expected auto-poll to be enabled via Q().Exec()")
	}
	if capturedOrgID != "org1" {
		t.Errorf("auto-poll update for org %q, want %q", capturedOrgID, "org1")
	}

	reason, _ := resp["reason"].(string)
	if !strings.Contains(reason, "Auto-polling") {
		t.Errorf("reason should mention Auto-polling, got: %s", reason)
	}
}

func TestSetupWebhook_127001EnablesAutoPoll(t *testing.T) {
	t.Parallel()

	pollEnabled := false
	mq := &mockQuerier{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			pollEnabled = true
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	h := &OnboardingHandler{
		Store:     &store.MockStore{QFn: func() store.Querier { return mq }},
		PublicURL: "http://127.0.0.1:8080",
	}

	req := httptest.NewRequest("POST", "/onboarding/webhook", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetupWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupWebhook(127.0.0.1): got status %d, want %d", w.Code, http.StatusOK)
	}
	if !pollEnabled {
		t.Error("expected auto-poll to be enabled for 127.0.0.1")
	}
}

// ── #33: Complete marks onboarding done ──

func TestOnboardingComplete_Success(t *testing.T) {
	t.Parallel()

	completeCalled := false
	h := &OnboardingHandler{
		Store: &store.MockStore{
			CompleteOnboardingFn: func(ctx context.Context, orgID string) error {
				completeCalled = true
				return nil
			},
			GetFirstDomainIDFn: func(ctx context.Context, orgID string) (string, error) {
				return "d1", nil
			},
		},
	}

	req := httptest.NewRequest("POST", "/onboarding/complete", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Complete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Complete: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !completeCalled {
		t.Error("Complete: CompleteOnboarding was not called")
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["first_domain_id"] != "d1" {
		t.Errorf("Complete: first_domain_id = %v, want d1", resp["first_domain_id"])
	}
}
