//go:build integration

package integration

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

func TestCreateAndListOrgLabels(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("labelslist-%s", t.Name()), fmt.Sprintf("labelslist-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	labelID, err := testStore.CreateOrgLabel(ctx, orgID, "Important")
	if err != nil {
		t.Fatalf("CreateOrgLabel: %v", err)
	}
	if labelID == "" {
		t.Fatal("expected non-empty label ID")
	}

	labels, err := testStore.ListOrgLabels(ctx, orgID)
	if err != nil {
		t.Fatalf("ListOrgLabels: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0]["id"] != labelID {
		t.Errorf("expected label ID %s, got %v", labelID, labels[0]["id"])
	}
	if labels[0]["name"] != "Important" {
		t.Errorf("expected name 'Important', got %v", labels[0]["name"])
	}
}

func TestRenameOrgLabel(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("labelsrename-%s", t.Name()), fmt.Sprintf("labelsrename-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	labelID, err := testStore.CreateOrgLabel(ctx, orgID, "OldName")
	if err != nil {
		t.Fatalf("CreateOrgLabel: %v", err)
	}

	// RenameOrgLabel uses FOR UPDATE so it must run inside a transaction
	var oldName string
	err = testStore.WithTx(ctx, func(txStore store.Store) error {
		var txErr error
		oldName, txErr = txStore.RenameOrgLabel(ctx, labelID, orgID, "NewName")
		return txErr
	})
	if err != nil {
		t.Fatalf("RenameOrgLabel: %v", err)
	}
	if oldName != "OldName" {
		t.Errorf("expected old name 'OldName', got %q", oldName)
	}

	// Verify rename persisted
	labels, err := testStore.ListOrgLabels(ctx, orgID)
	if err != nil {
		t.Fatalf("ListOrgLabels after rename: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0]["name"] != "NewName" {
		t.Errorf("expected name 'NewName', got %v", labels[0]["name"])
	}
}

func TestDeleteOrgLabel(t *testing.T) {
	ctx := context.Background()
	orgID, _ := seedOrg(t, fmt.Sprintf("labelsdel-%s", t.Name()), fmt.Sprintf("labelsdel-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	labelID, err := testStore.CreateOrgLabel(ctx, orgID, "ToDelete")
	if err != nil {
		t.Fatalf("CreateOrgLabel: %v", err)
	}

	// DeleteOrgLabel uses transaction
	var deletedName string
	err = testStore.WithTx(ctx, func(txStore store.Store) error {
		var txErr error
		deletedName, txErr = txStore.DeleteOrgLabel(ctx, labelID, orgID)
		return txErr
	})
	if err != nil {
		t.Fatalf("DeleteOrgLabel: %v", err)
	}
	if deletedName != "ToDelete" {
		t.Errorf("expected deleted name 'ToDelete', got %q", deletedName)
	}

	// Verify no longer in list
	labels, err := testStore.ListOrgLabels(ctx, orgID)
	if err != nil {
		t.Fatalf("ListOrgLabels after delete: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected 0 labels after delete, got %d", len(labels))
	}
}

func TestThreadLabelOperations(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("threadlabels-%s", t.Name()), fmt.Sprintf("threadlabels-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("threadlabels-%s.example.com", t.Name()))
	threadID := seedThread(t, orgID, userID, domainID, "Thread Labels Test")

	// Initially should have "inbox" label from seedThread
	labels := testStore.GetLabels(ctx, threadID)
	if len(labels) != 1 || labels[0] != "inbox" {
		t.Fatalf("expected [inbox], got %v", labels)
	}

	// Add a custom label
	err := testStore.AddLabel(ctx, threadID, orgID, "starred")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}

	labels = testStore.GetLabels(ctx, threadID)
	sort.Strings(labels)
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(labels), labels)
	}
	if labels[0] != "inbox" || labels[1] != "starred" {
		t.Errorf("expected [inbox, starred], got %v", labels)
	}

	// HasLabel
	if !testStore.HasLabel(ctx, threadID, "starred") {
		t.Error("expected HasLabel('starred') to return true")
	}
	if testStore.HasLabel(ctx, threadID, "nonexistent") {
		t.Error("expected HasLabel('nonexistent') to return false")
	}

	// RemoveLabel
	err = testStore.RemoveLabel(ctx, threadID, "starred")
	if err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}

	labels = testStore.GetLabels(ctx, threadID)
	if len(labels) != 1 || labels[0] != "inbox" {
		t.Errorf("expected [inbox] after removing starred, got %v", labels)
	}
}

func TestBulkLabelOperations(t *testing.T) {
	ctx := context.Background()
	orgID, userID := seedOrg(t, fmt.Sprintf("bulklabel-%s", t.Name()), fmt.Sprintf("bulklabel-%s@test.com", t.Name()), "password123")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	domainID := seedDomain(t, orgID, fmt.Sprintf("bulklabel-%s.example.com", t.Name()))

	threadID1 := seedThread(t, orgID, userID, domainID, "Bulk Label Thread 1")
	threadID2 := seedThread(t, orgID, userID, domainID, "Bulk Label Thread 2")
	threadID3 := seedThread(t, orgID, userID, domainID, "Bulk Label Thread 3")

	threadIDs := []string{threadID1, threadID2, threadID3}

	// BulkAddLabel
	err := testStore.BulkAddLabel(ctx, threadIDs, orgID, "urgent")
	if err != nil {
		t.Fatalf("BulkAddLabel: %v", err)
	}

	// Verify all three threads have the label
	for _, tid := range threadIDs {
		if !testStore.HasLabel(ctx, tid, "urgent") {
			t.Errorf("expected thread %s to have label 'urgent'", tid)
		}
	}

	// BatchFetchLabels
	labelMap, err := testStore.BatchFetchLabels(ctx, threadIDs)
	if err != nil {
		t.Fatalf("BatchFetchLabels: %v", err)
	}
	for _, tid := range threadIDs {
		labels, ok := labelMap[tid]
		if !ok {
			t.Errorf("expected thread %s in label map", tid)
			continue
		}
		// Each thread should have "inbox" (from seedThread) and "urgent"
		hasInbox, hasUrgent := false, false
		for _, l := range labels {
			if l == "inbox" {
				hasInbox = true
			}
			if l == "urgent" {
				hasUrgent = true
			}
		}
		if !hasInbox || !hasUrgent {
			t.Errorf("thread %s: expected both 'inbox' and 'urgent', got %v", tid, labels)
		}
	}

	// BulkRemoveLabel
	err = testStore.BulkRemoveLabel(ctx, threadIDs, "urgent")
	if err != nil {
		t.Fatalf("BulkRemoveLabel: %v", err)
	}

	for _, tid := range threadIDs {
		if testStore.HasLabel(ctx, tid, "urgent") {
			t.Errorf("expected thread %s to NOT have label 'urgent' after bulk remove", tid)
		}
	}
}
