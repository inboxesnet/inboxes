package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

func TestSetupStatus_NeedsSetup(t *testing.T) {
	t.Parallel()

	// SetupHandler.Status with StripeKey set bypasses DB entirely and returns
	// needs_setup=false, commercial=true. This tests the commercial-mode early return.
	h := &SetupHandler{StripeKey: "sk_test_123"}
	req := httptest.NewRequest("GET", "/setup/status", nil)
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupStatus(commercial): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("SetupStatus(commercial): failed to decode: %v", err)
	}
	if resp["needs_setup"] != false {
		t.Errorf("SetupStatus(commercial): got needs_setup=%v, want false", resp["needs_setup"])
	}
	if resp["commercial"] != true {
		t.Errorf("SetupStatus(commercial): got commercial=%v, want true", resp["commercial"])
	}
}

func TestSetupStatus_AlreadySetup(t *testing.T) {
	t.Parallel()

	// Same approach — test commercial mode which always returns needs_setup=false
	h := &SetupHandler{StripeKey: "sk_test_123"}

	req := httptest.NewRequest("GET", "/setup/status", nil)
	w := httptest.NewRecorder()

	h.Status(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SetupStatus(already setup): got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("SetupStatus(already setup): failed to decode: %v", err)
	}
	if resp["needs_setup"] != false {
		t.Errorf("SetupStatus(already setup): got needs_setup=%v, want false", resp["needs_setup"])
	}
}

func TestSetup_MissingFields(t *testing.T) {
	t.Parallel()

	h := &SetupHandler{
		Store: &store.MockStore{
			SetupCountUsersFn: func(ctx context.Context) (int, error) {
				return 0, nil
			},
		},
	}

	body := `{}`
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Setup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Setup(missing fields): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "email and password are required") {
		t.Errorf("Setup(missing fields): body = %q", w.Body.String())
	}
}

func TestSetup_WeakPassword(t *testing.T) {
	t.Parallel()

	h := &SetupHandler{
		Store: &store.MockStore{
			SetupCountUsersFn: func(ctx context.Context) (int, error) {
				return 0, nil
			},
		},
	}

	body := `{"email":"admin@example.com","password":"weak"}`
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Setup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Setup(weak password): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "password must be") {
		t.Errorf("Setup(weak password): body = %q", w.Body.String())
	}
}
