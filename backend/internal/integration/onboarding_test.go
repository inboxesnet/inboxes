//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/store"
)

// newOnboardingHandler creates an OnboardingHandler backed by real test infrastructure.
func newOnboardingHandler() *handler.OnboardingHandler {
	return &handler.OnboardingHandler{
		Store:     testStore,
		EncSvc:    testEncSvc,
		PublicURL: "http://localhost:8080",
	}
}

func TestOnboarding_StatusConnectStep(t *testing.T) {
	// Fresh org with no API key should return step "connect".
	orgID, userID := seedOrg(t, "Onb Connect Org", "integ-onb-connect@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	h := newOnboardingHandler()

	req := httptest.NewRequest("GET", "/api/onboarding/status", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["step"] != "connect" {
		t.Errorf("Status: step = %v, want 'connect'", resp["step"])
	}
}

func TestOnboarding_StatusDomainsStep(t *testing.T) {
	// Org with API key but no visible domains should return step "domains".
	orgID, userID := seedOrg(t, "Onb Domains Org", "integ-onb-domains@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Store an encrypted API key to move past the "connect" step.
	ctx := context.Background()
	ct, iv, tag, err := testEncSvc.Encrypt("re_test_fake_key")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.StoreEncryptedAPIKey(ctx, orgID, ct, iv, tag); err != nil {
		t.Fatal(err)
	}

	h := newOnboardingHandler()

	req := httptest.NewRequest("GET", "/api/onboarding/status", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status: got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["step"] != "domains" {
		t.Errorf("Status: step = %v, want 'domains'", resp["step"])
	}
}

func TestOnboarding_StatusSyncStep(t *testing.T) {
	// Org with API key + visible domain but no emails should return step "sync".
	orgID, userID := seedOrg(t, "Onb Sync Org", "integ-onb-sync@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()
	ct, iv, tag, err := testEncSvc.Encrypt("re_test_fake_key_2")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.StoreEncryptedAPIKey(ctx, orgID, ct, iv, tag); err != nil {
		t.Fatal(err)
	}

	// Create a visible domain (hidden=false is the default).
	seedDomain(t, orgID, "integ-sync.example.com")

	h := newOnboardingHandler()

	req := httptest.NewRequest("GET", "/api/onboarding/status", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status: got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["step"] != "sync" {
		t.Errorf("Status: step = %v, want 'sync'", resp["step"])
	}
}

func TestOnboarding_StatusAddressesStep(t *testing.T) {
	// Org with API key + domain + emails should return step "addresses".
	orgID, userID := seedOrg(t, "Onb Addr Org", "integ-onb-addr@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()
	ct, iv, tag, err := testEncSvc.Encrypt("re_test_fake_key_3")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.StoreEncryptedAPIKey(ctx, orgID, ct, iv, tag); err != nil {
		t.Fatal(err)
	}

	domainID := seedDomain(t, orgID, "integ-addr.example.com")
	threadID := seedThread(t, orgID, userID, domainID, "Test Thread")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "sender@integ-addr.example.com", "Test Email")

	h := newOnboardingHandler()

	req := httptest.NewRequest("GET", "/api/onboarding/status", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status: got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["step"] != "addresses" {
		t.Errorf("Status: step = %v, want 'addresses'", resp["step"])
	}
}

func TestOnboarding_CompleteAndVerify(t *testing.T) {
	// Test the complete onboarding endpoint and verify status persists.
	orgID, userID := seedOrg(t, "Onb Complete Org", "integ-onb-complete@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	h := newOnboardingHandler()

	// Complete onboarding
	req := httptest.NewRequest("POST", "/api/onboarding/complete", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Complete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Complete: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["message"] != "onboarding completed" {
		t.Errorf("Complete: message = %v, want 'onboarding completed'", resp["message"])
	}

	// Verify the onboarding_completed flag in the database
	ctx := context.Background()
	completed, err := testStore.GetOnboardingCompleted(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOnboardingCompleted: %v", err)
	}
	if !completed {
		t.Error("GetOnboardingCompleted: want true, got false")
	}
}

func TestOnboarding_ConnectMissingAPIKey(t *testing.T) {
	// POST /onboarding/connect with an empty api_key should return 400.
	orgID, userID := seedOrg(t, "Onb NoKey Org", "integ-onb-nokey@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	h := newOnboardingHandler()

	body := `{"api_key":""}`
	req := httptest.NewRequest("POST", "/api/onboarding/connect", strings.NewReader(body))
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Connect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Connect(empty key): got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "api_key") {
		t.Errorf("Connect(empty key): error = %q, want to contain 'api_key'", errMsg)
	}
}

func TestOnboarding_ConnectInvalidJSON(t *testing.T) {
	// POST /onboarding/connect with invalid JSON should return 400.
	orgID, userID := seedOrg(t, "Onb BadJSON Org", "integ-onb-badjson@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	h := newOnboardingHandler()

	req := httptest.NewRequest("POST", "/api/onboarding/connect", strings.NewReader("{bad"))
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Connect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Connect(bad JSON): got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestOnboarding_StatusRequiresAuth(t *testing.T) {
	// Calling Status without auth claims should panic (nil pointer dereference on claims).
	// This verifies that the handler relies on middleware-provided claims.
	h := newOnboardingHandler()

	req := httptest.NewRequest("GET", "/api/onboarding/status", nil)
	w := httptest.NewRecorder()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("Status(no claims): expected panic, got none")
		}
	}()
	h.Status(w, req)
}

func TestOnboarding_SetupAddressIndividual(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, "Onb SetupAddr Org", "integ-onb-setupaddr@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Store an encrypted API key to move past the "connect" step.
	ct, iv, tag, err := testEncSvc.Encrypt("re_test_fake_key_setup")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.StoreEncryptedAPIKey(ctx, orgID, ct, iv, tag); err != nil {
		t.Fatal(err)
	}

	domainName := "integ-setupaddr.example.com"
	domainID := seedDomain(t, orgID, domainName)
	threadID := seedThread(t, orgID, userID, domainID, "Setup Addr Thread")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "alice@"+domainName, "Setup Addr Email")

	// Use WithTx to call SetupAddress with type "individual"
	address := "alice@" + domainName
	err = testStore.WithTx(ctx, func(txStore store.Store) error {
		return txStore.SetupAddress(ctx, orgID, userID, address, "individual", "Alice")
	})
	if err != nil {
		t.Fatalf("SetupAddress(individual): %v", err)
	}

	// Verify an alias was created by listing aliases for the domain
	aliases, err := testStore.ListAliases(ctx, orgID, domainID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) == 0 {
		t.Fatal("expected at least 1 alias after SetupAddress(individual), got 0")
	}

	// Find the alias matching our address
	found := false
	for _, a := range aliases {
		if a["address"] == address {
			found = true
			if a["name"] != "Alice" {
				t.Errorf("expected alias name 'Alice', got %v", a["name"])
			}
			break
		}
	}
	if !found {
		t.Errorf("expected alias with address %s, not found in %d aliases", address, len(aliases))
	}
}

func TestOnboarding_SetupAddressSkip(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, "Onb SetupSkip Org", "integ-onb-setupskip@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Store an encrypted API key
	ct, iv, tag, err := testEncSvc.Encrypt("re_test_fake_key_skip")
	if err != nil {
		t.Fatal(err)
	}
	if err := testStore.StoreEncryptedAPIKey(ctx, orgID, ct, iv, tag); err != nil {
		t.Fatal(err)
	}

	domainName := "integ-setupskip.example.com"
	domainID := seedDomain(t, orgID, domainName)
	threadID := seedThread(t, orgID, userID, domainID, "Setup Skip Thread")
	seedEmail(t, orgID, userID, domainID, threadID, "inbound", "bob@"+domainName, "Setup Skip Email")

	// Call SetupAddress with type "skip" -- should not error
	address := "bob@" + domainName
	err = testStore.WithTx(ctx, func(txStore store.Store) error {
		return txStore.SetupAddress(ctx, orgID, userID, address, "skip", "")
	})
	if err != nil {
		t.Fatalf("SetupAddress(skip): %v", err)
	}
}
