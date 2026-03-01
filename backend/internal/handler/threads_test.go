package handler

import (
	"strings"
	"testing"
)

// --- appendAliasVisibility ---

func TestAppendAliasVisibility_SingleAlias(t *testing.T) {
	t.Parallel()
	filter, args, nextIdx := appendAliasVisibility("", nil, 1, []string{"hello@example.com"})

	if !strings.Contains(filter, "EXISTS") {
		t.Errorf("filter should contain EXISTS, got %q", filter)
	}
	if !strings.Contains(filter, "$1") {
		t.Errorf("filter should contain $1, got %q", filter)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	labels, ok := args[0].([]string)
	if !ok {
		t.Fatalf("arg should be []string, got %T", args[0])
	}
	if len(labels) != 1 || labels[0] != "alias:hello@example.com" {
		t.Errorf("labels = %v, want [alias:hello@example.com]", labels)
	}
	if nextIdx != 2 {
		t.Errorf("nextIdx = %d, want 2", nextIdx)
	}
}

func TestAppendAliasVisibility_MultipleAliases(t *testing.T) {
	t.Parallel()
	filter, args, nextIdx := appendAliasVisibility("", nil, 3, []string{"a@x.com", "b@x.com"})

	if !strings.Contains(filter, "$3") {
		t.Errorf("filter should contain $3, got %q", filter)
	}
	labels := args[0].([]string)
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0] != "alias:a@x.com" || labels[1] != "alias:b@x.com" {
		t.Errorf("labels = %v", labels)
	}
	if nextIdx != 4 {
		t.Errorf("nextIdx = %d, want 4", nextIdx)
	}
}

func TestAppendAliasVisibility_ArgIndexThreading(t *testing.T) {
	t.Parallel()
	// Start with existing args
	existingArgs := []interface{}{"org-1", "inbox"}
	filter, args, nextIdx := appendAliasVisibility(" WHERE org_id = $1", existingArgs, 3, []string{"x@y.com"})

	if !strings.Contains(filter, "$3") {
		t.Errorf("filter should contain $3, got %q", filter)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[0] != "org-1" || args[1] != "inbox" {
		t.Errorf("existing args should be preserved, got %v", args[:2])
	}
	if nextIdx != 4 {
		t.Errorf("nextIdx = %d, want 4", nextIdx)
	}
}

func TestAppendAliasVisibility_SQLShape(t *testing.T) {
	t.Parallel()
	filter, _, _ := appendAliasVisibility("", nil, 1, []string{"test@example.com"})

	if !strings.Contains(filter, "AND EXISTS") {
		t.Errorf("filter should start with AND EXISTS, got %q", filter)
	}
	if !strings.Contains(filter, "thread_labels") {
		t.Errorf("filter should reference thread_labels, got %q", filter)
	}
	if !strings.Contains(filter, "ANY") {
		t.Errorf("filter should use ANY, got %q", filter)
	}
}

func TestAppendAliasVisibility_LabelsGetPrefix(t *testing.T) {
	t.Parallel()
	_, args, _ := appendAliasVisibility("", nil, 1, []string{"foo@bar.com", "baz@bar.com"})
	labels := args[0].([]string)
	for _, l := range labels {
		if !strings.HasPrefix(l, "alias:") {
			t.Errorf("label %q should have 'alias:' prefix", l)
		}
	}
}

func TestAppendAliasVisibility_EmptyAliases(t *testing.T) {
	t.Parallel()
	filter, args, nextIdx := appendAliasVisibility("", nil, 1, []string{})
	// Even empty should produce the SQL fragment (caller responsibility to handle empty alias list)
	if !strings.Contains(filter, "EXISTS") {
		t.Errorf("filter should still contain EXISTS, got %q", filter)
	}
	labels := args[0].([]string)
	if len(labels) != 0 {
		t.Errorf("labels should be empty, got %v", labels)
	}
	if nextIdx != 2 {
		t.Errorf("nextIdx = %d, want 2", nextIdx)
	}
}
