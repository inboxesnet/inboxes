//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/handler"
)

// seedSecondAdmin creates a second admin user in the org (via invite + role change).
func seedSecondAdmin(t *testing.T, orgID, email string) string {
	t.Helper()
	ctx := context.Background()
	token := "token-" + email
	userID, err := testStore.InsertInvitedUser(ctx, orgID, email, "Admin2", "admin", token, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Activate the user by setting status and password
	testPool.Exec(ctx, "UPDATE users SET status = 'active', password_hash = '$2a$04$x' WHERE id = $1", userID)
	return userID
}

func TestRBAC_ChangeOwnerRole(t *testing.T) {
	ctx := context.Background()
	orgID, ownerID := seedOrg(t, fmt.Sprintf("rbac-owner-%s", t.Name()), fmt.Sprintf("rbac-owner-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Mark user as owner
	testPool.Exec(ctx, "UPDATE users SET is_owner = true WHERE id = $1", ownerID)

	// Create a second admin who tries to change the owner's role
	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-admin2-%s@test.com", t.Name()))

	h := &handler.UserHandler{Store: testStore}
	body := jsonBody(map[string]string{"role": "member"})
	req := httptest.NewRequest("PATCH", "/users/"+ownerID+"/role", body)
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, admin2ID, orgID, "admin")
	req = withChiParam(req, "id", ownerID)
	w := httptest.NewRecorder()

	h.ChangeRole(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" || w.Body.String() == "{}" {
		t.Error("expected error message in body")
	}
}

func TestRBAC_DisableAdminWith2Admins(t *testing.T) {
	orgID, adminID := seedOrg(t, fmt.Sprintf("rbac-dis2-%s", t.Name()), fmt.Sprintf("rbac-dis2-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dis2-b-%s@test.com", t.Name()))

	h := &handler.UserHandler{Store: testStore, RDB: testRDB}
	req := httptest.NewRequest("DELETE", "/users/"+admin2ID, nil)
	req = withClaims(req, adminID, orgID, "admin")
	req = withChiParam(req, "id", admin2ID)
	w := httptest.NewRecorder()

	h.Disable(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_DisableAdminWith3Admins(t *testing.T) {
	orgID, adminID := seedOrg(t, fmt.Sprintf("rbac-dis3-%s", t.Name()), fmt.Sprintf("rbac-dis3-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dis3-b-%s@test.com", t.Name()))
	_ = seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dis3-c-%s@test.com", t.Name()))

	h := &handler.UserHandler{Store: testStore, RDB: testRDB}
	req := httptest.NewRequest("DELETE", "/users/"+admin2ID, nil)
	req = withClaims(req, adminID, orgID, "admin")
	req = withChiParam(req, "id", admin2ID)
	w := httptest.NewRecorder()

	h.Disable(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_DemoteAdminWith2Admins(t *testing.T) {
	orgID, adminID := seedOrg(t, fmt.Sprintf("rbac-dem2-%s", t.Name()), fmt.Sprintf("rbac-dem2-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dem2-b-%s@test.com", t.Name()))

	h := &handler.UserHandler{Store: testStore}
	body := jsonBody(map[string]string{"role": "member"})
	req := httptest.NewRequest("PATCH", "/users/"+admin2ID+"/role", body)
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, adminID, orgID, "admin")
	req = withChiParam(req, "id", admin2ID)
	w := httptest.NewRecorder()

	h.ChangeRole(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_DemoteAdminWith3Admins(t *testing.T) {
	orgID, adminID := seedOrg(t, fmt.Sprintf("rbac-dem3-%s", t.Name()), fmt.Sprintf("rbac-dem3-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dem3-b-%s@test.com", t.Name()))
	_ = seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-dem3-c-%s@test.com", t.Name()))

	h := &handler.UserHandler{Store: testStore}
	body := jsonBody(map[string]string{"role": "member"})
	req := httptest.NewRequest("PATCH", "/users/"+admin2ID+"/role", body)
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, adminID, orgID, "admin")
	req = withChiParam(req, "id", admin2ID)
	w := httptest.NewRecorder()

	h.ChangeRole(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_DeleteOrgNonOwner(t *testing.T) {
	orgID, _ := seedOrg(t, fmt.Sprintf("rbac-del-%s", t.Name()), fmt.Sprintf("rbac-del-%s@test.com", t.Name()), "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Create a non-owner admin
	admin2ID := seedSecondAdmin(t, orgID, fmt.Sprintf("rbac-del-b-%s@test.com", t.Name()))

	h := &handler.OrgHandler{Store: testStore, RDB: testRDB}
	req := httptest.NewRequest("DELETE", "/org", nil)
	req = withClaims(req, admin2ID, orgID, "admin")
	w := httptest.NewRecorder()

	h.Delete(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403; body: %s", w.Code, w.Body.String())
	}
}
