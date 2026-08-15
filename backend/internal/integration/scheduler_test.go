//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/worker"
)

func TestSchedulerReleasesDueSend(t *testing.T) {
	orgID, userID := seedOrg(t, "sched-org", "sched@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "sched.test")
	threadID := seedThread(t, orgID, userID, domainID, "Scheduled")

	ctx := context.Background()
	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "me@sched.test", []byte(`["dest@example.com"]`), []byte(`[]`), []byte(`[]`),
		"Scheduled", "<p>body</p>", "body", "scheduled", "", nil)
	if err != nil {
		t.Fatalf("InsertEmail failed: %v", err)
	}
	jobID, err := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("CreateEmailJob failed: %v", err)
	}
	// Due one second ago.
	if _, err := testPool.Exec(ctx,
		"UPDATE email_jobs SET run_after = now() - interval '1 second' WHERE id = $1", jobID); err != nil {
		t.Fatalf("set run_after: %v", err)
	}

	testRDB.Del(ctx, "email:jobs")
	s := worker.NewScheduler(testPool, testRDB, testBus(), time.Second)
	s.ReleaseDueSendsForTest(ctx)

	// The job must be released: run_after cleared, email queued, id on the queue.
	var runAfter *time.Time
	if err := testPool.QueryRow(ctx, "SELECT run_after FROM email_jobs WHERE id = $1", jobID).Scan(&runAfter); err != nil {
		t.Fatalf("job lookup: %v", err)
	}
	if runAfter != nil {
		t.Error("expected run_after cleared after release")
	}
	var emailStatus string
	testPool.QueryRow(ctx, "SELECT status FROM emails WHERE id = $1", emailID).Scan(&emailStatus)
	if emailStatus != "queued" {
		t.Errorf("expected email status queued, got %q", emailStatus)
	}
	queued, err := testRDB.LRange(ctx, "email:jobs", 0, -1).Result()
	if err != nil {
		t.Fatalf("lrange: %v", err)
	}
	found := false
	for _, id := range queued {
		if id == jobID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected job %s on the queue, got %v", jobID, queued)
	}
	testRDB.Del(ctx, "email:jobs")
}

func TestSchedulerLeavesFutureSendAlone(t *testing.T) {
	orgID, userID := seedOrg(t, "sched-future-org", "sched-future@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "sched-future.test")
	threadID := seedThread(t, orgID, userID, domainID, "Future")

	ctx := context.Background()
	emailID, _ := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID,
		"outbound", "me@sched-future.test", []byte(`["dest@example.com"]`), []byte(`[]`), []byte(`[]`),
		"Future", "<p>body</p>", "body", "scheduled", "", nil)
	jobID, _ := testStore.CreateEmailJob(ctx, orgID, userID, domainID, "send", emailID, threadID, []byte(`{}`), nil)
	testPool.Exec(ctx, "UPDATE email_jobs SET run_after = now() + interval '1 hour' WHERE id = $1", jobID)

	testRDB.Del(ctx, "email:jobs")
	s := worker.NewScheduler(testPool, testRDB, testBus(), time.Second)
	s.ReleaseDueSendsForTest(ctx)

	var runAfter *time.Time
	testPool.QueryRow(ctx, "SELECT run_after FROM email_jobs WHERE id = $1", jobID).Scan(&runAfter)
	if runAfter == nil {
		t.Error("a future job must keep its run_after")
	}
	n, _ := testRDB.LLen(ctx, "email:jobs").Result()
	if n != 0 {
		t.Errorf("a future job must not be enqueued, queue has %d entries", n)
	}
}

func TestSnoozeHidesFromInboxAndWakes(t *testing.T) {
	orgID, userID := seedOrg(t, "snooze-org", "snooze@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "snooze.test")
	threadID := seedThread(t, orgID, userID, domainID, "Snooze Me")

	ctx := context.Background()

	// Snooze via the handler.
	h := &handler.ThreadHandler{Store: testStore, Bus: testBus()}
	until := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodPatch, "/api/threads/"+threadID+"/snooze",
		jsonBody(map[string]string{"until": until}))
	req = withChiParam(req, "id", threadID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Snooze(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snooze: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Hidden from inbox, visible in the snoozed view.
	_, inboxTotal, err := testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads inbox: %v", err)
	}
	if inboxTotal != 0 {
		t.Errorf("expected 0 inbox threads while snoozed, got %d", inboxTotal)
	}
	snoozed, snoozedTotal, err := testStore.ListThreads(ctx, orgID, "snoozed", "", "admin", nil, 1, 50)
	if err != nil {
		t.Fatalf("ListThreads snoozed: %v", err)
	}
	if snoozedTotal != 1 || len(snoozed) != 1 {
		t.Fatalf("expected 1 snoozed thread, got %d", snoozedTotal)
	}
	if snoozed[0]["snoozed_until"] == nil {
		t.Error("expected snoozed_until on the listed thread")
	}

	// Force the snooze into the past; the scheduler wakes it.
	testPool.Exec(ctx, "UPDATE threads SET snoozed_until = now() - interval '1 minute' WHERE id = $1", threadID)
	s := worker.NewScheduler(testPool, testRDB, testBus(), time.Second)
	s.WakeSnoozedThreadsForTest(ctx)

	_, inboxTotal, _ = testStore.ListThreads(ctx, orgID, "inbox", "", "admin", nil, 1, 50)
	if inboxTotal != 1 {
		t.Errorf("expected thread back in inbox after wake, got %d", inboxTotal)
	}
	var snoozedUntil *time.Time
	testPool.QueryRow(ctx, "SELECT snoozed_until FROM threads WHERE id = $1", threadID).Scan(&snoozedUntil)
	if snoozedUntil != nil {
		t.Error("expected snoozed_until cleared after wake")
	}
}

func TestUnsnoozeViaHandler(t *testing.T) {
	orgID, userID := seedOrg(t, "unsnooze-org", "unsnooze@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "unsnooze.test")
	threadID := seedThread(t, orgID, userID, domainID, "Unsnooze Me")

	ctx := context.Background()
	testPool.Exec(ctx, "UPDATE threads SET snoozed_until = now() + interval '1 day' WHERE id = $1", threadID)

	h := &handler.ThreadHandler{Store: testStore, Bus: testBus()}
	req := httptest.NewRequest(http.MethodPatch, "/api/threads/"+threadID+"/snooze",
		jsonBody(map[string]any{"until": nil}))
	req = withChiParam(req, "id", threadID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Snooze(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unsnooze: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var snoozedUntil *time.Time
	testPool.QueryRow(ctx, "SELECT snoozed_until FROM threads WHERE id = $1", threadID).Scan(&snoozedUntil)
	if snoozedUntil != nil {
		t.Error("expected snoozed_until cleared by handler")
	}
}

func TestDraftScheduledSendAndCancel(t *testing.T) {
	orgID, userID := seedOrg(t, "draftsched-org", "draftsched@test.io", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })
	domainID := seedDomain(t, orgID, "draftsched.test")

	ctx := context.Background()
	draftID, err := testStore.CreateDraft(ctx, orgID, userID, domainID, nil, "compose",
		"Later Please", "me@draftsched.test",
		[]byte(`["dest@example.com"]`), []byte(`[]`), []byte(`[]`), []byte(`[]`), "", "")
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}

	h := &handler.DraftHandler{Store: testStore, RDB: testRDB, Bus: testBus()}
	scheduledAt := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodPost, "/api/drafts/"+draftID+"/send",
		jsonBody(map[string]string{"scheduled_at": scheduledAt}))
	req = withChiParam(req, "id", draftID)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Send(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("scheduled send: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		EmailID string `json:"email_id"`
		JobID   string `json:"job_id"`
		Status  string `json:"status"`
	}
	parseJSON(t, w, &resp)
	if resp.Status != "scheduled" {
		t.Fatalf("expected status scheduled, got %q", resp.Status)
	}

	// The job must be parked, the email marked scheduled, the draft stamped.
	var runAfter *time.Time
	if err := testPool.QueryRow(ctx, "SELECT run_after FROM email_jobs WHERE id = $1", resp.JobID).Scan(&runAfter); err != nil {
		t.Fatalf("job lookup: %v", err)
	}
	if runAfter == nil {
		t.Fatal("expected run_after set on the scheduled job")
	}
	var emailStatus string
	testPool.QueryRow(ctx, "SELECT status FROM emails WHERE id = $1", resp.EmailID).Scan(&emailStatus)
	if emailStatus != "scheduled" {
		t.Errorf("expected email status scheduled, got %q", emailStatus)
	}
	var draftSched *time.Time
	testPool.QueryRow(ctx, "SELECT scheduled_send_at FROM drafts WHERE id = $1", draftID).Scan(&draftSched)
	if draftSched == nil {
		t.Error("expected scheduled_send_at on the draft")
	}

	// Cancel the schedule: job cancelled, email gone, draft kept and cleared.
	req2 := httptest.NewRequest(http.MethodPost, "/api/drafts/"+draftID+"/cancel-schedule", nil)
	req2 = withChiParam(req2, "id", draftID)
	req2 = withClaims(req2, userID, orgID, "admin")
	w2 := httptest.NewRecorder()
	h.CancelSchedule(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("cancel-schedule: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var jobStatus string
	testPool.QueryRow(ctx, "SELECT status FROM email_jobs WHERE id = $1", resp.JobID).Scan(&jobStatus)
	if jobStatus != "cancelled" {
		t.Errorf("expected job cancelled, got %q", jobStatus)
	}
	var emailCount int
	testPool.QueryRow(ctx, "SELECT count(*) FROM emails WHERE id = $1", resp.EmailID).Scan(&emailCount)
	if emailCount != 0 {
		t.Error("expected parked email deleted after cancel")
	}
	var draftCount int
	testPool.QueryRow(ctx, "SELECT count(*) FROM drafts WHERE id = $1 AND scheduled_send_at IS NULL", draftID).Scan(&draftCount)
	if draftCount != 1 {
		t.Error("expected draft kept with scheduled_send_at cleared")
	}
}
