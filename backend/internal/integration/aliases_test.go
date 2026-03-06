//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
)

func TestCreateAndListAliases(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("aliaslist-%s", t.Name()), fmt.Sprintf("aliaslist-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("aliaslist-%s.example.com", t.Name()))

	address := fmt.Sprintf("hello@aliaslist-%s.example.com", t.Name())
	aliasID, err := testStore.CreateAlias(ctx, orgID, domainID, address, "Hello Alias")
	if err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}
	if aliasID == "" {
		t.Fatal("expected non-empty alias ID")
	}

	// List aliases and verify
	aliases, err := testStore.ListAliases(ctx, orgID, domainID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0]["id"] != aliasID {
		t.Errorf("expected alias ID %s, got %v", aliasID, aliases[0]["id"])
	}
	if aliases[0]["address"] != address {
		t.Errorf("expected address %s, got %v", address, aliases[0]["address"])
	}
	if aliases[0]["name"] != "Hello Alias" {
		t.Errorf("expected name 'Hello Alias', got %v", aliases[0]["name"])
	}
}

func TestDeleteAlias(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("aliasdel-%s", t.Name()), fmt.Sprintf("aliasdel-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("aliasdel-%s.example.com", t.Name()))
	address := fmt.Sprintf("del@aliasdel-%s.example.com", t.Name())
	aliasID, err := testStore.CreateAlias(ctx, orgID, domainID, address, "To Delete")
	if err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	// Delete alias
	rows, err := testStore.DeleteAlias(ctx, aliasID, orgID)
	if err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	// Verify no longer in list (ListAliases filters deleted_at IS NULL)
	aliases, err := testStore.ListAliases(ctx, orgID, domainID)
	if err != nil {
		t.Fatalf("ListAliases after delete: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}

	// Delete again should affect 0 rows
	rows, err = testStore.DeleteAlias(ctx, aliasID, orgID)
	if err != nil {
		t.Fatalf("DeleteAlias second time: %v", err)
	}
	if rows != 0 {
		t.Errorf("expected 0 rows affected on double delete, got %d", rows)
	}
}

func TestAddAliasUser(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("aliasuser-%s", t.Name()), fmt.Sprintf("aliasuser-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("aliasuser-%s.example.com", t.Name()))
	address := fmt.Sprintf("team@aliasuser-%s.example.com", t.Name())
	aliasID, err := testStore.CreateAlias(ctx, orgID, domainID, address, "Team Alias")
	if err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	// Add user to alias with can_send_as = true
	err = testStore.AddAliasUser(ctx, aliasID, orgID, userID, true)
	if err != nil {
		t.Fatalf("AddAliasUser: %v", err)
	}

	// Verify via ListAliases - check users sub-array
	aliases, err := testStore.ListAliases(ctx, orgID, domainID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}

	users, ok := aliases[0]["users"].([]map[string]any)
	if !ok {
		t.Fatalf("expected users to be []map[string]any, got %T", aliases[0]["users"])
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 alias user, got %d", len(users))
	}
	if users[0]["user_id"] != userID {
		t.Errorf("expected user_id %s, got %v", userID, users[0]["user_id"])
	}
	if users[0]["can_send_as"] != true {
		t.Errorf("expected can_send_as true, got %v", users[0]["can_send_as"])
	}

	// Verify CanSendAs for non-admin role too (admin always returns true, so test as member)
	// First change role to member so the alias path is exercised
	_, err = testStore.ChangeRole(ctx, userID, orgID, "member")
	if err != nil {
		t.Fatalf("ChangeRole: %v", err)
	}

	canSend, err := testStore.CanSendAs(ctx, userID, orgID, address, "member")
	if err != nil {
		t.Fatalf("CanSendAs: %v", err)
	}
	if !canSend {
		t.Error("expected CanSendAs to return true for added alias user")
	}
}

func TestSetDefaultAlias(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("aliasdefault-%s", t.Name()), fmt.Sprintf("aliasdefault-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("aliasdefault-%s.example.com", t.Name()))

	addr1 := fmt.Sprintf("a1@aliasdefault-%s.example.com", t.Name())
	addr2 := fmt.Sprintf("a2@aliasdefault-%s.example.com", t.Name())

	aliasID1, err := testStore.CreateAlias(ctx, orgID, domainID, addr1, "Alias 1")
	if err != nil {
		t.Fatalf("CreateAlias 1: %v", err)
	}
	aliasID2, err := testStore.CreateAlias(ctx, orgID, domainID, addr2, "Alias 2")
	if err != nil {
		t.Fatalf("CreateAlias 2: %v", err)
	}

	// Add user to both aliases
	if err := testStore.AddAliasUser(ctx, aliasID1, orgID, userID, true); err != nil {
		t.Fatalf("AddAliasUser 1: %v", err)
	}
	if err := testStore.AddAliasUser(ctx, aliasID2, orgID, userID, true); err != nil {
		t.Fatalf("AddAliasUser 2: %v", err)
	}

	// Set alias1 as default
	err = testStore.SetDefaultAlias(ctx, aliasID1, userID, orgID)
	if err != nil {
		t.Fatalf("SetDefaultAlias 1: %v", err)
	}

	// Verify via ListMyAliases
	myAliases, err := testStore.ListMyAliases(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("ListMyAliases: %v", err)
	}
	if len(myAliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(myAliases))
	}

	// First one should be the default (ListMyAliases orders by is_default DESC)
	if myAliases[0]["id"] != aliasID1 {
		t.Errorf("expected first alias to be %s (default), got %v", aliasID1, myAliases[0]["id"])
	}
	if myAliases[0]["is_default"] != true {
		t.Errorf("expected is_default true for alias1, got %v", myAliases[0]["is_default"])
	}

	// Now set alias2 as default, alias1 should lose default
	err = testStore.SetDefaultAlias(ctx, aliasID2, userID, orgID)
	if err != nil {
		t.Fatalf("SetDefaultAlias 2: %v", err)
	}

	myAliases, err = testStore.ListMyAliases(ctx, userID, orgID)
	if err != nil {
		t.Fatalf("ListMyAliases after re-default: %v", err)
	}

	// alias2 should now be first (default)
	if myAliases[0]["id"] != aliasID2 {
		t.Errorf("expected first alias to be %s (new default), got %v", aliasID2, myAliases[0]["id"])
	}
	if myAliases[0]["is_default"] != true {
		t.Errorf("expected is_default true for alias2, got %v", myAliases[0]["is_default"])
	}
	if myAliases[1]["is_default"] != false {
		t.Errorf("expected is_default false for alias1, got %v", myAliases[1]["is_default"])
	}
}

func TestDiscoveredAddresses(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("discovered-%s", t.Name()), fmt.Sprintf("discovered-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("discovered-%s.example.com", t.Name()))

	// Insert discovered addresses via raw SQL
	addresses := []string{
		fmt.Sprintf("alice@discovered-%s.example.com", t.Name()),
		fmt.Sprintf("bob@discovered-%s.example.com", t.Name()),
		fmt.Sprintf("carol@discovered-%s.example.com", t.Name()),
	}
	for _, addr := range addresses {
		localPart := addr[:len(addr)-len("@discovered-"+t.Name()+".example.com")]
		_, err := testPool.Exec(ctx,
			"INSERT INTO discovered_addresses (domain_id, address, local_part) VALUES ($1, $2, $3)",
			domainID, addr, localPart)
		if err != nil {
			t.Fatalf("Insert discovered address %s: %v", addr, err)
		}
	}

	// ListDiscoveredAddresses returns only type='unclaimed' addresses
	result, err := testStore.ListDiscoveredAddresses(ctx, orgID)
	if err != nil {
		t.Fatalf("ListDiscoveredAddresses: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 discovered addresses, got %d", len(result))
	}

	// Verify the returned addresses contain the ones we inserted
	returnedAddrs := make(map[string]bool)
	for _, r := range result {
		addr, ok := r["address"].(string)
		if !ok {
			t.Fatalf("expected address to be string, got %T", r["address"])
		}
		returnedAddrs[addr] = true
	}
	for _, addr := range addresses {
		if !returnedAddrs[addr] {
			t.Errorf("expected address %s in results, not found", addr)
		}
	}
}
