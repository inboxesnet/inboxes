//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWebhookDedupFirstTime(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-dedup1-org", "wh-dedup1@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	// First time check should return false (not a duplicate)
	exists, err := testStore.CheckWebhookDedup(ctx, orgID, "resend-email-id-001", "email.delivered")
	if err != nil {
		t.Fatalf("CheckWebhookDedup failed: %v", err)
	}
	if exists {
		t.Fatal("expected dedup check to return false for first occurrence")
	}
}

func TestWebhookDedupAfterInsert(t *testing.T) {
	orgID, userID := seedOrg(t, "wh-dedup2-org", "wh-dedup2@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "wh-dedup2.test")
	threadID := seedThread(t, orgID, userID, domainID, "Webhook Dedup Thread")

	ctx := context.Background()

	// Insert an email with a resend_email_id to trigger dedup
	toJSON, _ := json.Marshal([]string{"to@example.com"})
	ccJSON, _ := json.Marshal([]string{})
	bccJSON, _ := json.Marshal([]string{})
	refsJSON, _ := json.Marshal([]string{})

	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "from@example.com", toJSON, ccJSON, bccJSON,
		"Dedup Test", "<p>test</p>", "test", "sent", "", refsJSON)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}

	// Set the resend_email_id on the email
	resendEmailID := "resend-dedup-" + t.Name()
	_, err = testPool.Exec(ctx,
		`UPDATE emails SET resend_email_id = $1 WHERE id = $2`,
		resendEmailID, emailID)
	if err != nil {
		t.Fatalf("update resend_email_id failed: %v", err)
	}

	// Now CheckWebhookDedup should find it
	exists, err := testStore.CheckWebhookDedup(ctx, orgID, resendEmailID, "email.delivered")
	if err != nil {
		t.Fatalf("CheckWebhookDedup failed: %v", err)
	}
	if !exists {
		t.Fatal("expected dedup check to return true after insert")
	}
}

func TestUpdateEmailStatus(t *testing.T) {
	orgID, userID := seedOrg(t, "wh-status-org", "wh-status@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "wh-status.test")
	threadID := seedThread(t, orgID, userID, domainID, "Status Update Thread")

	ctx := context.Background()

	toJSON, _ := json.Marshal([]string{"to@example.com"})
	ccJSON, _ := json.Marshal([]string{})
	bccJSON, _ := json.Marshal([]string{})
	refsJSON, _ := json.Marshal([]string{})

	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "from@example.com", toJSON, ccJSON, bccJSON,
		"Status Test", "", "", "queued", "", refsJSON)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}

	// Set resend_email_id
	resendEmailID := "resend-status-" + t.Name()
	_, err = testPool.Exec(ctx,
		`UPDATE emails SET resend_email_id = $1 WHERE id = $2`,
		resendEmailID, emailID)
	if err != nil {
		t.Fatalf("set resend_email_id failed: %v", err)
	}

	// Update status
	n, err := testStore.UpdateEmailStatus(ctx, orgID, resendEmailID, "delivered")
	if err != nil {
		t.Fatalf("UpdateEmailStatus failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}

	// Verify status changed
	var status string
	err = testPool.QueryRow(ctx,
		`SELECT status FROM emails WHERE id = $1`, emailID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query status failed: %v", err)
	}
	if status != "delivered" {
		t.Fatalf("expected status 'delivered', got '%s'", status)
	}
}

func TestUpdateEmailStatusNoMatch(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-nomatch-org", "wh-nomatch@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	n, err := testStore.UpdateEmailStatus(ctx, orgID, "nonexistent-resend-id", "delivered")
	if err != nil {
		t.Fatalf("UpdateEmailStatus failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows affected for nonexistent resend ID, got %d", n)
	}
}

func TestInsertBounce(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-bounce-org", "wh-bounce@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	err := testStore.InsertBounce(ctx, orgID, "bounced-wh@example.com", "hard")
	if err != nil {
		t.Fatalf("InsertBounce failed: %v", err)
	}

	// Verify bounce exists
	blocked, err := testStore.CheckBouncedRecipients(ctx, orgID, []string{"bounced-wh@example.com"})
	if err != nil {
		t.Fatalf("CheckBouncedRecipients failed: %v", err)
	}
	if len(blocked) != 1 {
		t.Fatalf("expected 1 blocked address, got %d", len(blocked))
	}
}

func TestClearBounceWebhook(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-clr-org", "wh-clr@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	// Insert then clear
	testStore.InsertBounce(ctx, orgID, "cleared-wh@example.com", "soft")
	err := testStore.ClearBounce(ctx, orgID, "cleared-wh@example.com")
	if err != nil {
		t.Fatalf("ClearBounce failed: %v", err)
	}

	blocked, err := testStore.CheckBouncedRecipients(ctx, orgID, []string{"cleared-wh@example.com"})
	if err != nil {
		t.Fatalf("CheckBouncedRecipients failed: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("expected 0 blocked after clear, got %d", len(blocked))
	}
}

func TestGetWebhookSecretEncrypted(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-secret-org", "wh-secret@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	// Store encrypted webhook secret
	_, err := testPool.Exec(ctx,
		`UPDATE orgs SET resend_webhook_secret_encrypted = $1, resend_webhook_secret_iv = $2, resend_webhook_secret_tag = $3
		 WHERE id = $4`,
		"encrypted_secret_data", "iv_data", "tag_data", orgID)
	if err != nil {
		t.Fatalf("update webhook secret failed: %v", err)
	}

	encSecret, encIV, encTag, plainSecret, err := testStore.GetOrgWebhookSecret(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgWebhookSecret failed: %v", err)
	}
	if encSecret != "encrypted_secret_data" {
		t.Fatalf("expected encrypted secret 'encrypted_secret_data', got '%s'", encSecret)
	}
	if encIV != "iv_data" {
		t.Fatalf("expected IV 'iv_data', got '%s'", encIV)
	}
	if encTag != "tag_data" {
		t.Fatalf("expected tag 'tag_data', got '%s'", encTag)
	}
	// Plain secret should be nil since we only set encrypted
	if plainSecret != nil {
		t.Fatalf("expected nil plain secret, got '%v'", *plainSecret)
	}
}

func TestGetWebhookSecretNone(t *testing.T) {
	orgID, _ := seedOrg(t, "wh-nosecret-org", "wh-nosecret@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	ctx := context.Background()

	encSecret, encIV, encTag, plainSecret, err := testStore.GetOrgWebhookSecret(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgWebhookSecret failed: %v", err)
	}
	if encSecret != "" {
		t.Fatalf("expected empty encrypted secret, got '%s'", encSecret)
	}
	if encIV != "" {
		t.Fatalf("expected empty IV, got '%s'", encIV)
	}
	if encTag != "" {
		t.Fatalf("expected empty tag, got '%s'", encTag)
	}
	if plainSecret != nil {
		t.Fatalf("expected nil plain secret, got '%v'", *plainSecret)
	}
}
