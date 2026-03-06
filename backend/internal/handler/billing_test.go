package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/store"
	"github.com/stripe/stripe-go/v84"
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

// ── #103: Stripe webhook signature verification ──

func stripeSignature(payload []byte, secret string, ts time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d", ts.Unix())))
	mac.Write([]byte("."))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), sig)
}

func TestStripeWebhook_InvalidSignature(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		StripeWebhookSecret: "whsec_test_secret",
		Store:               &store.MockStore{},
	}
	body := `{"id":"evt_test","type":"checkout.session.completed","data":{"object":{}}}`
	req := httptest.NewRequest("POST", "/billing/webhook", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", "t=1234567890,v1=invalidsignature")
	w := httptest.NewRecorder()
	h.HandleStripeWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Webhook(invalid sig): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid signature") {
		t.Errorf("Webhook(invalid sig): body = %q", w.Body.String())
	}
}

func TestStripeWebhook_MissingSignature(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		StripeWebhookSecret: "whsec_test_secret",
		Store:               &store.MockStore{},
	}
	body := `{"id":"evt_test","type":"checkout.session.completed","data":{"object":{}}}`
	req := httptest.NewRequest("POST", "/billing/webhook", strings.NewReader(body))
	// No Stripe-Signature header
	w := httptest.NewRecorder()
	h.HandleStripeWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Webhook(no sig): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStripeWebhook_ValidSignature(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		StripeWebhookSecret: "whsec_test_secret",
		Store: &store.MockStore{
			InsertStripeEventFn: func(ctx context.Context, eventID string) (bool, error) {
				return true, nil
			},
		},
		// Bypass Stripe SDK signature verification in tests — just parse the JSON.
		VerifyWebhook: func(payload []byte, header string, secret string) (stripe.Event, error) {
			var evt stripe.Event
			err := json.Unmarshal(payload, &evt)
			return evt, err
		},
	}
	payload := []byte(`{"id":"evt_valid","type":"unknown.event","data":{"object":{}}}`)
	sig := stripeSignature(payload, "whsec_test_secret", time.Now())
	req := httptest.NewRequest("POST", "/billing/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()
	h.HandleStripeWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Webhook(valid sig): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ── GetBilling handler tests ──

func TestGetBilling_OrgNotFound(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return nil, fmt.Errorf("org not found")
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "org not found" {
		t.Errorf("error: got %q, want %q", body["error"], "org not found")
	}
}

func TestGetBilling_ProPlan(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "pro",
					"plan_expires_at":         (*time.Time)(nil),
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "pro" {
		t.Errorf("plan: got %q, want %q", body["plan"], "pro")
	}
}

func TestGetBilling_CancelledWithFutureExpiry(t *testing.T) {
	t.Parallel()
	futureExpiry := time.Now().Add(7 * 24 * time.Hour)
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "cancelled",
					"plan_expires_at":         &futureExpiry,
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "cancelled" {
		t.Errorf("plan: got %q, want %q (grace period still active)", body["plan"], "cancelled")
	}
}

func TestGetBilling_CancelledWithPastExpiry(t *testing.T) {
	t.Parallel()
	pastExpiry := time.Now().Add(-24 * time.Hour)
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "cancelled",
					"plan_expires_at":         &pastExpiry,
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "free" {
		t.Errorf("plan: got %q, want %q (grace period expired)", body["plan"], "free")
	}
}

func TestGetBilling_PastDueWithExpiry(t *testing.T) {
	t.Parallel()
	futureExpiry := time.Now().Add(14 * 24 * time.Hour)
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "past_due",
					"plan_expires_at":         &futureExpiry,
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "past_due" {
		t.Errorf("plan: got %q, want %q", body["plan"], "past_due")
	}
}

func TestGetBilling_ProClearsExpiry(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "pro",
					"plan_expires_at":         (*time.Time)(nil),
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "pro" {
		t.Errorf("plan: got %q, want %q", body["plan"], "pro")
	}
	if body["plan_expires_at"] != nil {
		t.Errorf("plan_expires_at: got %v, want nil", body["plan_expires_at"])
	}
}

func TestGetBilling_BillingEnabled(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "free",
					"plan_expires_at":         (*time.Time)(nil),
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	billingEnabled, ok := body["billing_enabled"].(bool)
	if !ok {
		t.Fatal("billing_enabled: not a bool or missing")
	}
	if !billingEnabled {
		t.Error("billing_enabled: got false, want true")
	}
}

func TestGetBilling_FreePlan(t *testing.T) {
	t.Parallel()
	h := &BillingHandler{
		Store: &store.MockStore{
			GetBillingInfoFn: func(ctx context.Context, orgID string) (map[string]any, error) {
				return map[string]any{
					"plan":                    "free",
					"plan_expires_at":         (*time.Time)(nil),
					"stripe_subscription_id":  (*string)(nil),
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = withClaims(req, "user-1", "org-1", "admin")
	rec := httptest.NewRecorder()

	h.GetBilling(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["plan"] != "free" {
		t.Errorf("plan: got %q, want %q", body["plan"], "free")
	}
	if body["plan_expires_at"] != nil {
		t.Errorf("plan_expires_at: got %v, want nil", body["plan_expires_at"])
	}
}
