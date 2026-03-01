package handler

import (
	"testing"

	"github.com/google/uuid"
)

func TestDecodeStripeObject_ValidJSON(t *testing.T) {
	t.Parallel()
	var dst struct {
		Name string `json:"name"`
	}
	err := decodeStripeObject([]byte(`{"name":"test"}`), &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "test" {
		t.Errorf("Name: got %q, want %q", dst.Name, "test")
	}
}

func TestDecodeStripeObject_InvalidJSON(t *testing.T) {
	t.Parallel()
	var dst struct{}
	err := decodeStripeObject([]byte(`{invalid`), &dst)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeStripeObject_EmptyBytes(t *testing.T) {
	t.Parallel()
	var dst struct{}
	err := decodeStripeObject([]byte{}, &dst)
	if err == nil {
		t.Error("expected error for empty bytes")
	}
}

func TestDecodeStripeObject_PartialFields(t *testing.T) {
	t.Parallel()
	var dst struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	err := decodeStripeObject([]byte(`{"name":"test"}`), &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "test" {
		t.Errorf("Name: got %q, want %q", dst.Name, "test")
	}
	if dst.Email != "" {
		t.Errorf("Email: got %q, want empty", dst.Email)
	}
}

func TestBillingDomainIDValidation_ValidUUID(t *testing.T) {
	t.Parallel()
	id := uuid.New().String()
	_, err := uuid.Parse(id)
	if err != nil {
		t.Errorf("valid UUID rejected: %v", err)
	}
}

func TestBillingDomainIDValidation_InvalidUUID(t *testing.T) {
	t.Parallel()
	_, err := uuid.Parse("not-a-uuid")
	if err == nil {
		t.Error("invalid UUID accepted")
	}
}

func TestBillingDomainIDValidation_Empty(t *testing.T) {
	t.Parallel()
	_, err := uuid.Parse("")
	if err == nil {
		t.Error("empty string accepted as UUID")
	}
}

func TestBillingDomainIDValidation_SQLInjection(t *testing.T) {
	t.Parallel()
	_, err := uuid.Parse("'; DROP TABLE orgs; --")
	if err == nil {
		t.Error("SQL injection string accepted as UUID")
	}
}
