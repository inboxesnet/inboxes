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

func TestAliasList_Success(t *testing.T) {
	t.Parallel()

	h := &AliasHandler{
		Store: &store.MockStore{
			ListAliasesFn: func(ctx context.Context, orgID, domainID string) ([]map[string]any, error) {
				return []map[string]any{
					{"id": "a1", "address": "alice@example.com"},
					{"id": "a2", "address": "bob@example.com"},
				}, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/aliases?domain_id=d1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("AliasList: got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("AliasList: failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("AliasList: got %d aliases, want 2", len(resp))
	}
}

func TestAliasCreate_Success(t *testing.T) {
	t.Parallel()

	h := &AliasHandler{
		Store: &store.MockStore{
			CreateAliasFn: func(ctx context.Context, orgID, domainID, address, name string) (string, error) {
				return "alias1", nil
			},
		},
	}

	body := `{"address":"test@example.com","domain_id":"d1","name":"Test Alias"}`
	req := httptest.NewRequest("POST", "/aliases", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("AliasCreate: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("AliasCreate: failed to decode response: %v", err)
	}
	if resp["id"] != "alias1" {
		t.Errorf("AliasCreate: got id %q, want %q", resp["id"], "alias1")
	}
}

func TestAliasCreate_MissingAddress(t *testing.T) {
	t.Parallel()

	h := &AliasHandler{
		Store: &store.MockStore{},
	}

	body := `{"domain_id":"d1","name":"No Address"}`
	req := httptest.NewRequest("POST", "/aliases", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("AliasCreate(missing address): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "address and domain_id are required") {
		t.Errorf("AliasCreate(missing address): body = %q", w.Body.String())
	}
}

func TestAliasDelete_Success(t *testing.T) {
	t.Parallel()

	h := &AliasHandler{
		Store: &store.MockStore{
			DeleteAliasFn: func(ctx context.Context, aliasID, orgID string) (int64, error) {
				return 1, nil
			},
		},
	}

	req := httptest.NewRequest("DELETE", "/aliases/a1", nil)
	req = withClaims(req, "user1", "org1", "admin")
	req = req.WithContext(newChiRouteContext("id", "a1"))
	// Re-apply claims after context swap
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("AliasDelete: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestAliasSetDefault_Success(t *testing.T) {
	t.Parallel()

	h := &AliasHandler{
		Store: &store.MockStore{
			SetDefaultAliasFn: func(ctx context.Context, aliasID, userID, orgID string) error {
				return nil
			},
		},
	}

	req := httptest.NewRequest("PUT", "/aliases/a1/default", nil)
	req = req.WithContext(newChiRouteContext("id", "a1"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.SetDefault(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("AliasSetDefault: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestAliasAddUser_Success(t *testing.T) {
	t.Parallel()
	addCalled := false
	h := &AliasHandler{
		Store: &store.MockStore{
			CheckAliasOrgFn: func(ctx context.Context, aliasID, orgID string) (int, error) {
				return 1, nil
			},
			CheckUserOrgFn: func(ctx context.Context, userID, orgID string) (bool, error) {
				return true, nil
			},
			AddAliasUserFn: func(ctx context.Context, aliasID, orgID, userID string, canSendAs bool) error {
				addCalled = true
				if !canSendAs {
					t.Error("AddAliasUser: expected canSendAs=true")
				}
				return nil
			},
		},
	}
	body := `{"user_id":"u2","can_send_as":true}`
	req := httptest.NewRequest("POST", "/aliases/a1/users", strings.NewReader(body))
	req = req.WithContext(newChiRouteContext("id", "a1"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.AddUser(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("AddUser: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if !addCalled {
		t.Error("AddUser: store.AddAliasUser was not called")
	}
}

func TestAliasDiscoveredAddresses_Success(t *testing.T) {
	t.Parallel()
	h := &AliasHandler{
		Store: &store.MockStore{
			ListDiscoveredAddressesFn: func(ctx context.Context, orgID string) ([]map[string]any, error) {
				return []map[string]any{
					{"address": "unknown@example.com", "count": 5},
				}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/aliases/discovered", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.DiscoveredAddresses(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("DiscoveredAddresses: got status %d, want %d", w.Code, http.StatusOK)
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("DiscoveredAddresses: decode failed: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("DiscoveredAddresses: got %d, want 1", len(resp))
	}
}
