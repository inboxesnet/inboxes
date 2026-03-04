//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGetBillingInfoDefault(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("billinginfo-%s", t.Name()), fmt.Sprintf("billinginfo-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	if info["plan"] != "free" {
		t.Errorf("expected plan 'free', got %v", info["plan"])
	}
	// These are typed nil pointers (*string / *time.Time), so check via type assertion
	if v, ok := info["stripe_customer_id"].(*string); ok && v != nil {
		t.Errorf("expected nil stripe_customer_id, got %v", v)
	}
	if v, ok := info["stripe_subscription_id"].(*string); ok && v != nil {
		t.Errorf("expected nil stripe_subscription_id, got %v", v)
	}
	if v, ok := info["plan_expires_at"].(*time.Time); ok && v != nil {
		t.Errorf("expected nil plan_expires_at, got %v", v)
	}
}

func TestSetStripeCustomerID(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("stripecust-%s", t.Name()), fmt.Sprintf("stripecust-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	customerID := "cus_test_abc123"

	err := testStore.SetStripeCustomerID(ctx, orgID, customerID)
	if err != nil {
		t.Fatalf("SetStripeCustomerID: %v", err)
	}

	// Verify via GetStripeCustomerID
	got, err := testStore.GetStripeCustomerID(ctx, orgID)
	if err != nil {
		t.Fatalf("GetStripeCustomerID: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil customer ID")
	}
	if *got != customerID {
		t.Errorf("expected customer ID %q, got %q", customerID, *got)
	}

	// Also verify via GetBillingInfo
	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	custID, ok := info["stripe_customer_id"].(*string)
	if !ok || custID == nil {
		t.Fatalf("expected *string stripe_customer_id, got %T", info["stripe_customer_id"])
	}
	if *custID != customerID {
		t.Errorf("expected stripe_customer_id %q in billing info, got %q", customerID, *custID)
	}
}

func TestUpdateOrgPlan(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("orgplan-%s", t.Name()), fmt.Sprintf("orgplan-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	subID := "sub_test_xyz789"

	err := testStore.UpdateOrgPlan(ctx, orgID, "pro", subID)
	if err != nil {
		t.Fatalf("UpdateOrgPlan: %v", err)
	}

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	if info["plan"] != "pro" {
		t.Errorf("expected plan 'pro', got %v", info["plan"])
	}

	gotSubID, ok := info["stripe_subscription_id"].(*string)
	if !ok || gotSubID == nil {
		t.Fatalf("expected *string stripe_subscription_id, got %T", info["stripe_subscription_id"])
	}
	if *gotSubID != subID {
		t.Errorf("expected subscription ID %q, got %q", subID, *gotSubID)
	}
}

func TestSetPlanCancelledWithFutureExpiry(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("graceF-%s", t.Name()), fmt.Sprintf("graceF-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Set plan to pro first, then cancel with 7-day grace
	_ = testStore.UpdateOrgPlan(ctx, orgID, "pro", "sub_123")
	expiry := time.Now().Add(7 * 24 * time.Hour)
	testPool.Exec(ctx, "UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1, stripe_subscription_id = NULL WHERE id = $2", expiry, orgID)

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	if info["plan"] != "cancelled" {
		t.Errorf("expected plan 'cancelled', got %v", info["plan"])
	}
	expiresAt, ok := info["plan_expires_at"].(*time.Time)
	if !ok || expiresAt == nil {
		t.Fatal("expected non-nil plan_expires_at")
	}
	if expiresAt.Before(time.Now()) {
		t.Error("expected plan_expires_at to be in the future")
	}
}

func TestSetPlanCancelledWithPastExpiry(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("graceP-%s", t.Name()), fmt.Sprintf("graceP-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Set a past expiry (grace period elapsed)
	pastExpiry := time.Now().Add(-24 * time.Hour)
	testPool.Exec(ctx, "UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1 WHERE id = $2", pastExpiry, orgID)

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	// Store still says "cancelled" — the handler converts to "free" based on expiry
	if info["plan"] != "cancelled" {
		t.Errorf("expected plan 'cancelled', got %v", info["plan"])
	}
	expiresAt, ok := info["plan_expires_at"].(*time.Time)
	if !ok || expiresAt == nil {
		t.Fatal("expected non-nil plan_expires_at")
	}
	if !expiresAt.Before(time.Now()) {
		t.Error("expected plan_expires_at to be in the past")
	}
}

func TestSetPlanPastDueWithExpiry(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("pastdue-%s", t.Name()), fmt.Sprintf("pastdue-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	expiry := time.Now().Add(14 * 24 * time.Hour)
	testPool.Exec(ctx, "UPDATE orgs SET plan = 'past_due', plan_expires_at = $1 WHERE id = $2", expiry, orgID)

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	if info["plan"] != "past_due" {
		t.Errorf("expected plan 'past_due', got %v", info["plan"])
	}
	expiresAt, ok := info["plan_expires_at"].(*time.Time)
	if !ok || expiresAt == nil {
		t.Fatal("expected non-nil plan_expires_at")
	}
	if expiresAt.Before(time.Now()) {
		t.Error("expected plan_expires_at to be in the future")
	}
}

func TestSetPlanProClearsExpiry(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("proclr-%s", t.Name()), fmt.Sprintf("proclr-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// First set cancelled with expiry, then upgrade to pro
	testPool.Exec(ctx, "UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1 WHERE id = $2", time.Now().Add(7*24*time.Hour), orgID)

	// Now set to pro and clear expiry
	testPool.Exec(ctx, "UPDATE orgs SET plan = 'pro', plan_expires_at = NULL, stripe_subscription_id = 'sub_new' WHERE id = $1", orgID)

	info, err := testStore.GetBillingInfo(ctx, orgID)
	if err != nil {
		t.Fatalf("GetBillingInfo: %v", err)
	}
	if info["plan"] != "pro" {
		t.Errorf("expected plan 'pro', got %v", info["plan"])
	}
	expiresAt, ok := info["plan_expires_at"].(*time.Time)
	if ok && expiresAt != nil {
		t.Errorf("expected nil plan_expires_at, got %v", expiresAt)
	}
}

func TestInsertStripeEventIdempotency(t *testing.T) {
	ctx := context.Background()

	// No org needed for stripe_events, but clean up the event after test
	eventID := fmt.Sprintf("evt_test_%s", t.Name())
	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM stripe_events WHERE event_id = $1", eventID)
	})

	// First insert should succeed (return true = inserted)
	inserted, err := testStore.InsertStripeEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("InsertStripeEvent first: %v", err)
	}
	if !inserted {
		t.Error("expected first InsertStripeEvent to return true (inserted)")
	}

	// Second insert with same ID should return false (duplicate, ON CONFLICT DO NOTHING)
	inserted, err = testStore.InsertStripeEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("InsertStripeEvent second: %v", err)
	}
	if inserted {
		t.Error("expected second InsertStripeEvent to return false (duplicate)")
	}
}
