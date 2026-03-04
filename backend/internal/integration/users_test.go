//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestListUsers(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("listusers-%s", t.Name()), fmt.Sprintf("listusers-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	users, err := testStore.ListUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0]["id"] != userID {
		t.Errorf("expected user ID %s, got %s", userID, users[0]["id"])
	}
	if users[0]["role"] != "admin" {
		t.Errorf("expected role admin, got %v", users[0]["role"])
	}
	if users[0]["status"] != "active" {
		t.Errorf("expected status active, got %v", users[0]["status"])
	}
}

func TestInsertInvitedUser(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("inviteduser-%s", t.Name()), fmt.Sprintf("inviteduser-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	inviteEmail := fmt.Sprintf("invited-%s@test.com", t.Name())
	token := "invite-token-123"
	expiresAt := time.Now().Add(24 * time.Hour)

	invitedID, err := testStore.InsertInvitedUser(ctx, orgID, inviteEmail, "Invited User", "member", token, expiresAt)
	if err != nil {
		t.Fatalf("InsertInvitedUser: %v", err)
	}
	if invitedID == "" {
		t.Fatal("expected non-empty user ID")
	}

	// Verify user exists in DB with status "invited"
	users, err := testStore.ListUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	var found bool
	for _, u := range users {
		if u["id"] == invitedID {
			found = true
			if u["status"] != "invited" {
				t.Errorf("expected status invited, got %v", u["status"])
			}
			if u["email"] != inviteEmail {
				t.Errorf("expected email %s, got %v", inviteEmail, u["email"])
			}
			if u["role"] != "member" {
				t.Errorf("expected role member, got %v", u["role"])
			}
		}
	}
	if !found {
		t.Error("invited user not found in ListUsers")
	}
}

func TestCountActiveAdmins(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("countadmins-%s", t.Name()), fmt.Sprintf("countadmins-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	count, err := testStore.CountActiveAdmins(ctx, orgID)
	if err != nil {
		t.Fatalf("CountActiveAdmins: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 active admin, got %d", count)
	}
}

func TestChangeRole(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("changerole-%s", t.Name()), fmt.Sprintf("changerole-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Verify starts as 1 admin
	count, err := testStore.CountActiveAdmins(ctx, orgID)
	if err != nil {
		t.Fatalf("CountActiveAdmins before: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active admin before change, got %d", count)
	}

	// Change role to member
	rows, err := testStore.ChangeRole(ctx, userID, orgID, "member")
	if err != nil {
		t.Fatalf("ChangeRole: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	// Now CountActiveAdmins should be 0
	count, err = testStore.CountActiveAdmins(ctx, orgID)
	if err != nil {
		t.Fatalf("CountActiveAdmins after: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 active admins after role change, got %d", count)
	}

	// Verify role via GetUserRole
	role, err := testStore.GetUserRole(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("GetUserRole: %v", err)
	}
	if role != "member" {
		t.Errorf("expected role member, got %s", role)
	}
}

func TestDisableUser(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("disableuser-%s", t.Name()), fmt.Sprintf("disableuser-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	rows, err := testStore.DisableUser(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	// Verify user is disabled via ListUsers
	users, err := testStore.ListUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0]["status"] != "disabled" {
		t.Errorf("expected status disabled, got %v", users[0]["status"])
	}

	// CountActiveAdmins should return 0 since user is disabled
	count, err := testStore.CountActiveAdmins(ctx, orgID)
	if err != nil {
		t.Fatalf("CountActiveAdmins: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 active admins after disable, got %d", count)
	}
}

func TestGetMe(t *testing.T) {
	ctx := context.Background()
	email := fmt.Sprintf("getme-%s@test.com", t.Name())
	orgID, userID := seedOrg(t, fmt.Sprintf("getme-%s", t.Name()), email, "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	me, err := testStore.GetMe(ctx, userID)
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me["id"] != userID {
		t.Errorf("expected id %s, got %v", userID, me["id"])
	}
	if me["email"] != email {
		t.Errorf("expected email %s, got %v", email, me["email"])
	}
	if me["role"] != "admin" {
		t.Errorf("expected role admin, got %v", me["role"])
	}
	if me["status"] != "active" {
		t.Errorf("expected status active, got %v", me["status"])
	}
	if me["is_owner"] != false {
		t.Errorf("expected is_owner false, got %v", me["is_owner"])
	}
	if me["has_webhook"] != false {
		t.Errorf("expected has_webhook false, got %v", me["has_webhook"])
	}
}

func TestEnableUser(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("enableuser-%s", t.Name()), fmt.Sprintf("enableuser-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Disable the user first
	rows, err := testStore.DisableUser(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if rows != 1 {
		t.Fatalf("DisableUser: expected 1 row affected, got %d", rows)
	}

	// Verify user is disabled
	users, err := testStore.ListUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("ListUsers (after disable): %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0]["status"] != "disabled" {
		t.Fatalf("expected status 'disabled' after DisableUser, got %v", users[0]["status"])
	}

	// Re-enable the user
	rows, err = testStore.EnableUser(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("EnableUser: %v", err)
	}
	if rows != 1 {
		t.Errorf("EnableUser: expected 1 row affected, got %d", rows)
	}

	// Verify user is active again
	users, err = testStore.ListUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("ListUsers (after enable): %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0]["status"] != "active" {
		t.Errorf("expected status 'active' after EnableUser, got %v", users[0]["status"])
	}
}
