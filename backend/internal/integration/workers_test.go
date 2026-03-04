//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestCreateAndGetEmailJob(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("jobcreate-%s", t.Name()), fmt.Sprintf("jobcreate-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("jobcreate-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Job Test Thread")
	emailID := seedEmail(t, orgID, userID, domainID, threadID, "outbound", fmt.Sprintf("jobcreate-%s@test.com", t.Name()), "Job Test Subject")

	payload := json.RawMessage(`{"to":["test@example.com"],"subject":"test"}`)

	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, payload, nil)
	if err != nil {
		t.Fatalf("CreateEmailJob: %v", err)
	}
	if jobID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// GetEmailJob
	job, err := testStore.GetEmailJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetEmailJob: %v", err)
	}
	if job == nil {
		t.Fatal("expected non-nil job")
	}
	if job["id"] != jobID {
		t.Errorf("expected job ID %s, got %v", jobID, job["id"])
	}
	if job["org_id"] != orgID {
		t.Errorf("expected org_id %s, got %v", orgID, job["org_id"])
	}
	if job["user_id"] != userID {
		t.Errorf("expected user_id %s, got %v", userID, job["user_id"])
	}
	if job["job_type"] != "send" {
		t.Errorf("expected job_type 'send', got %v", job["job_type"])
	}
	if job["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", job["status"])
	}

	retryCount, ok := job["retry_count"].(int32)
	if !ok {
		t.Errorf("expected retry_count to be int32, got %T (%v)", job["retry_count"], job["retry_count"])
	} else if retryCount != 0 {
		t.Errorf("expected retry_count 0, got %d", retryCount)
	}
}

func TestUpdateEmailJobStatus(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("jobstatus-%s", t.Name()), fmt.Sprintf("jobstatus-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("jobstatus-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Status Test Thread")
	emailID := seedEmail(t, orgID, userID, domainID, threadID, "outbound", fmt.Sprintf("jobstatus-%s@test.com", t.Name()), "Status Test Subject")

	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, nil, nil)
	if err != nil {
		t.Fatalf("CreateEmailJob: %v", err)
	}

	// Update to running
	err = testStore.UpdateEmailJobStatus(ctx, jobID, "running", "")
	if err != nil {
		t.Fatalf("UpdateEmailJobStatus to running: %v", err)
	}

	job, err := testStore.GetEmailJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetEmailJob after running: %v", err)
	}
	if job["status"] != "running" {
		t.Errorf("expected status 'running', got %v", job["status"])
	}

	// Update to failed with error message
	err = testStore.UpdateEmailJobStatus(ctx, jobID, "failed", "connection timeout")
	if err != nil {
		t.Fatalf("UpdateEmailJobStatus to failed: %v", err)
	}

	job, err = testStore.GetEmailJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetEmailJob after failed: %v", err)
	}
	if job["status"] != "failed" {
		t.Errorf("expected status 'failed', got %v", job["status"])
	}
	if job["error_message"] != "connection timeout" {
		t.Errorf("expected error_message 'connection timeout', got %v", job["error_message"])
	}
}

func TestIncrementJobAttempts(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("jobretry-%s", t.Name()), fmt.Sprintf("jobretry-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("jobretry-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Retry Test Thread")
	emailID := seedEmail(t, orgID, userID, domainID, threadID, "outbound", fmt.Sprintf("jobretry-%s@test.com", t.Name()), "Retry Test Subject")

	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, nil, nil)
	if err != nil {
		t.Fatalf("CreateEmailJob: %v", err)
	}

	// Increment twice
	for i := 0; i < 2; i++ {
		err = testStore.IncrementJobAttempts(ctx, jobID)
		if err != nil {
			t.Fatalf("IncrementJobAttempts (attempt %d): %v", i+1, err)
		}
	}

	job, err := testStore.GetEmailJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetEmailJob: %v", err)
	}

	retryCount, ok := job["retry_count"].(int32)
	if !ok {
		t.Fatalf("expected retry_count to be int32, got %T (%v)", job["retry_count"], job["retry_count"])
	}
	if retryCount != 2 {
		t.Errorf("expected retry_count 2, got %d", retryCount)
	}
}

func TestStaleJobDetection(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("jobstale-%s", t.Name()), fmt.Sprintf("jobstale-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("jobstale-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Stale Test Thread")
	emailID := seedEmail(t, orgID, userID, domainID, threadID, "outbound", fmt.Sprintf("jobstale-%s@test.com", t.Name()), "Stale Test Subject")

	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, nil, nil)
	if err != nil {
		t.Fatalf("CreateEmailJob: %v", err)
	}

	// Set status to running
	err = testStore.UpdateEmailJobStatus(ctx, jobID, "running", "")
	if err != nil {
		t.Fatalf("UpdateEmailJobStatus: %v", err)
	}

	// Set heartbeat to 10 minutes in the past
	_, err = testPool.Exec(ctx,
		"UPDATE email_jobs SET heartbeat_at = now() - interval '10 minutes' WHERE id = $1", jobID)
	if err != nil {
		t.Fatalf("Set old heartbeat: %v", err)
	}

	// GetStaleJobs with 5-minute timeout should find it
	staleIDs, err := testStore.GetStaleJobs(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleJobs: %v", err)
	}

	found := false
	for _, id := range staleIDs {
		if id == jobID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected job %s in stale jobs, got %v", jobID, staleIDs)
	}

	// GetStaleJobs with 15-minute timeout should NOT find it
	staleIDs, err = testStore.GetStaleJobs(ctx, 15*time.Minute)
	if err != nil {
		t.Fatalf("GetStaleJobs with 15m timeout: %v", err)
	}
	for _, id := range staleIDs {
		if id == jobID {
			t.Errorf("did not expect job %s with 15m timeout", jobID)
		}
	}
}

func TestPollableOrgs(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("pollable-%s", t.Name()), fmt.Sprintf("pollable-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Enable auto_poll and set a fake encrypted API key
	_, err := testPool.Exec(ctx,
		`UPDATE orgs SET auto_poll_enabled = true, auto_poll_interval = 60,
		 resend_api_key_encrypted = 'fakeciphertext', resend_api_key_iv = 'fakeiv', resend_api_key_tag = 'faketag',
		 last_polled_at = NULL
		 WHERE id = $1`, orgID)
	if err != nil {
		t.Fatalf("Enable auto poll: %v", err)
	}

	orgs, err := testStore.GetPollableOrgs(ctx)
	if err != nil {
		t.Fatalf("GetPollableOrgs: %v", err)
	}

	found := false
	for _, o := range orgs {
		if o["id"] == orgID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected org %s in pollable orgs, got %d orgs", orgID, len(orgs))
	}

	// Now set last_polled_at to just now -- should NOT be pollable (interval is 60s)
	_, err = testPool.Exec(ctx, "UPDATE orgs SET last_polled_at = now() WHERE id = $1", orgID)
	if err != nil {
		t.Fatalf("Set last_polled_at: %v", err)
	}

	orgs, err = testStore.GetPollableOrgs(ctx)
	if err != nil {
		t.Fatalf("GetPollableOrgs after recent poll: %v", err)
	}

	for _, o := range orgs {
		if o["id"] == orgID {
			t.Error("expected org NOT to be pollable after recent poll")
		}
	}
}

func TestPurgeExpiredTrash(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("purgetrash-%s", t.Name()), fmt.Sprintf("purgetrash-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("purgetrash-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Trash Purge Thread")

	// Add "trash" label to the thread
	if err := testStore.AddLabel(ctx, threadID, orgID, "trash"); err != nil {
		t.Fatalf("AddLabel(trash): %v", err)
	}

	// Set trash expiry (this sets trash_expires_at = now() + 30 days on the threads table)
	if err := testStore.SetTrashExpiry(ctx, []string{threadID}, orgID); err != nil {
		t.Fatalf("SetTrashExpiry: %v", err)
	}

	// Override trash_expires_at to the past so PurgeExpiredTrash picks it up
	_, err := testPool.Exec(ctx,
		"UPDATE threads SET trash_expires_at = now() - interval '1 day' WHERE id = $1", threadID)
	if err != nil {
		t.Fatalf("Set past trash_expires_at: %v", err)
	}

	// Run purge
	affected, err := testStore.PurgeExpiredTrash(ctx)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if affected < 1 {
		t.Errorf("expected >= 1 rows affected, got %d", affected)
	}

	// Verify thread is soft-deleted (deleted_at IS NOT NULL)
	var deletedAt *time.Time
	err = testPool.QueryRow(ctx, "SELECT deleted_at FROM threads WHERE id = $1", threadID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("Query deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Error("expected thread to be soft-deleted (deleted_at set), but it is NULL")
	}

	// Also verify thread no longer appears in inbox listing
	threads, _, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	for _, th := range threads {
		if th["id"] == threadID {
			t.Error("expected purged thread not to appear in inbox listing")
		}
	}
}

func TestPurgeExpiredTrash_FutureUntouched(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("purgefuture-%s", t.Name()), fmt.Sprintf("purgefuture-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("purgefuture-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Future Trash Thread")

	// Add "trash" label
	if err := testStore.AddLabel(ctx, threadID, orgID, "trash"); err != nil {
		t.Fatalf("AddLabel(trash): %v", err)
	}

	// Set trash expiry with default (30 days in the future)
	if err := testStore.SetTrashExpiry(ctx, []string{threadID}, orgID); err != nil {
		t.Fatalf("SetTrashExpiry: %v", err)
	}

	// Run purge -- should NOT affect this thread
	_, err := testStore.PurgeExpiredTrash(ctx)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}

	// Verify thread is NOT soft-deleted
	var deletedAt *time.Time
	err = testPool.QueryRow(ctx, "SELECT deleted_at FROM threads WHERE id = $1", threadID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("Query deleted_at: %v", err)
	}
	if deletedAt != nil {
		t.Error("expected thread NOT to be soft-deleted, but deleted_at is set")
	}

	// Verify it still has the trash label
	if !testStore.HasLabel(ctx, threadID, "trash") {
		t.Error("expected thread to still have 'trash' label")
	}
}
