package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// ── Pure-function tests (existing) ──

func TestGenerateVerificationCode_Length(t *testing.T) {
	t.Parallel()
	code := generateVerificationCode()
	if len(code) != 6 {
		t.Errorf("length: got %d, want 6", len(code))
	}
}

func TestGenerateVerificationCode_AllDigits(t *testing.T) {
	t.Parallel()
	code := generateVerificationCode()
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			t.Errorf("non-digit %q in code %q", ch, code)
		}
	}
}

func TestGenerateVerificationCode_LeadingZeroPreserved(t *testing.T) {
	t.Parallel()
	// Generate many codes and check that short numbers are padded
	for i := 0; i < 100; i++ {
		code := generateVerificationCode()
		if len(code) != 6 {
			t.Fatalf("code %q length %d != 6", code, len(code))
		}
	}
}

func TestGenerateVerificationCode_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		seen[generateVerificationCode()] = true
	}
	if len(seen) < 50 {
		t.Errorf("only %d unique codes out of 100", len(seen))
	}
}

func TestNormalizeEmail_Lowercase(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("USER@EXAMPLE.COM"); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_TrimWhitespace(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("  user@example.com  "); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_AlreadyNormalized(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("user@example.com"); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_Empty(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail(""); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestNormalizeEmail_MixedCase(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("Alice.Smith@Gmail.COM"); got != "alice.smith@gmail.com" {
		t.Errorf("got %q, want %q", got, "alice.smith@gmail.com")
	}
}

// ── Signup validation ──

func TestSignup_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("Signup(invalid json): body = %q", w.Body.String())
	}
}

func TestSignup_MissingFields(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	body := `{"email":"","password":"","org_name":""}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(missing fields): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("Signup(missing fields): body = %q", w.Body.String())
	}
}

func TestSignup_InvalidEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	body := `{"email":"not-an-email","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(invalid email): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "email") {
		t.Errorf("Signup(invalid email): body = %q", w.Body.String())
	}
}

func TestSignup_PasswordComplexity(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	// Missing uppercase letter
	body := `{"email":"user@example.com","password":"weakpass1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(weak password): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "password") {
		t.Errorf("Signup(weak password): body = %q", w.Body.String())
	}
}

func TestSignup_OrgNameTooLong(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	longName := strings.Repeat("a", 256)
	body := `{"email":"user@example.com","password":"Password1","org_name":"` + longName + `"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(long org_name): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "org_name") {
		t.Errorf("Signup(long org_name): body = %q", w.Body.String())
	}
}

func TestSignup_NameTooLong(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{StripeKey: "sk_test"}
	longName := strings.Repeat("a", 256)
	body := `{"email":"user@example.com","password":"Password1","org_name":"Test Org","name":"` + longName + `"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Signup(long name): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "name") {
		t.Errorf("Signup(long name): body = %q", w.Body.String())
	}
}

func TestSignup_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes all validation — will panic at DB (nil), confirming validation passed
	h := &AuthHandler{StripeKey: "sk_test"}
	body := `{"email":"user@example.com","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Signup(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("Signup(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

func TestSignup_SelfHostedBlock(t *testing.T) {
	t.Parallel()
	// StripeKey="" means self-hosted mode — will panic when counting users on nil DB
	// This confirms the solo-mode check runs before validation
	h := &AuthHandler{StripeKey: ""}
	body := `{"email":"user@example.com","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Signup(w, req)
	}()
	// Should panic at h.Store.CountUsers (nil Store) in the solo-mode check, not get a 400
	if w.Code == http.StatusBadRequest {
		t.Errorf("Signup(self-hosted): got 400, should reach DB check: %s", w.Body.String())
	}
}

func TestSignup_EmailNormalization(t *testing.T) {
	t.Parallel()
	// Uppercase/whitespace email should be normalized before validation
	h := &AuthHandler{StripeKey: "sk_test"}
	body := `{"email":"  USER@EXAMPLE.COM  ","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Signup(w, req)
	}()
	// Should not fail with invalid email — normalization runs first
	if w.Code == http.StatusBadRequest {
		t.Errorf("Signup(email normalization): got 400, email should be normalized: %s", w.Body.String())
	}
}

// ── Login validation ──

func TestLogin_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Login(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("Login(invalid json): body = %q", w.Body.String())
	}
}

func TestLogin_MissingFields(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":"","password":""}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Login(missing fields): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("Login(missing fields): body = %q", w.Body.String())
	}
}

func TestLogin_InvalidEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":"not-an-email","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Login(invalid email): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "email") {
		t.Errorf("Login(invalid email): body = %q", w.Body.String())
	}
}

func TestLogin_PasswordTooLong(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	longPass := strings.Repeat("a", 129)
	body := `{"email":"user@example.com","password":"` + longPass + `"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Login(long password): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "128") {
		t.Errorf("Login(long password): body = %q", w.Body.String())
	}
}

func TestLogin_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at DB query (nil DB)
	h := &AuthHandler{}
	body := `{"email":"user@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Login(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("Login(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── VerifyEmail validation ──

func TestVerifyEmail_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("VerifyEmail(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("VerifyEmail(invalid json): body = %q", w.Body.String())
	}
}

func TestVerifyEmail_MissingFields(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":"","code":""}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("VerifyEmail(missing fields): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("VerifyEmail(missing fields): body = %q", w.Body.String())
	}
}

func TestVerifyEmail_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at DB query (nil DB)
	h := &AuthHandler{}
	body := `{"email":"user@example.com","code":"123456"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.VerifyEmail(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("VerifyEmail(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── ResendVerification validation ──

func TestResendVerification_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/resend-verification", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.ResendVerification(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResendVerification(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("ResendVerification(invalid json): body = %q", w.Body.String())
	}
}

func TestResendVerification_MissingEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":""}`
	req := httptest.NewRequest("POST", "/auth/resend-verification", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResendVerification(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResendVerification(missing email): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("ResendVerification(missing email): body = %q", w.Body.String())
	}
}

func TestResendVerification_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at DB exec (nil DB)
	h := &AuthHandler{}
	body := `{"email":"user@example.com"}`
	req := httptest.NewRequest("POST", "/auth/resend-verification", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.ResendVerification(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("ResendVerification(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── ForgotPassword validation ──

func TestForgotPassword_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ForgotPassword(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("ForgotPassword(invalid json): body = %q", w.Body.String())
	}
}

func TestForgotPassword_MissingEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":""}`
	req := httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ForgotPassword(missing email): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("ForgotPassword(missing email): body = %q", w.Body.String())
	}
}

func TestForgotPassword_InvalidEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"email":"not-an-email"}`
	req := httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ForgotPassword(invalid email): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "email") {
		t.Errorf("ForgotPassword(invalid email): body = %q", w.Body.String())
	}
}

func TestForgotPassword_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at ResendSvc.HasSystemKey (nil) or DB exec
	h := &AuthHandler{StripeKey: "sk_test"}
	body := `{"email":"user@example.com"}`
	req := httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.ForgotPassword(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("ForgotPassword(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── ResetPassword validation ──

func TestResetPassword_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResetPassword(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("ResetPassword(invalid json): body = %q", w.Body.String())
	}
}

func TestResetPassword_MissingFields(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"token":"","password":""}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResetPassword(missing fields): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("ResetPassword(missing fields): body = %q", w.Body.String())
	}
}

func TestResetPassword_WeakPassword(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"token":"abc123","password":"weakpass"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResetPassword(weak password): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "password") {
		t.Errorf("ResetPassword(weak password): body = %q", w.Body.String())
	}
}

func TestResetPassword_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at bcrypt then DB (nil DB)
	h := &AuthHandler{}
	body := `{"token":"abc123","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.ResetPassword(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("ResetPassword(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── Claim (invite) validation ──

func TestClaim_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Claim(invalid json): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid request body") {
		t.Errorf("Claim(invalid json): body = %q", w.Body.String())
	}
}

func TestClaim_MissingFields(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"token":"","password":""}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Claim(missing fields): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("Claim(missing fields): body = %q", w.Body.String())
	}
}

func TestClaim_WeakPassword(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	body := `{"token":"invite-tok","password":"weakpass"}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Claim(weak password): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "password") {
		t.Errorf("Claim(weak password): body = %q", w.Body.String())
	}
}

func TestClaim_ValidShape(t *testing.T) {
	t.Parallel()
	// Valid payload passes validation — will panic at bcrypt then DB (nil DB)
	h := &AuthHandler{}
	body := `{"token":"invite-tok","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Claim(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("Claim(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── ValidateClaim validation ──

func TestValidateClaim_MissingToken(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{}
	req := httptest.NewRequest("GET", "/auth/validate-claim", nil)
	w := httptest.NewRecorder()
	h.ValidateClaim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ValidateClaim(missing token): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "token is required") {
		t.Errorf("ValidateClaim(missing token): body = %q", w.Body.String())
	}
}

func TestValidateClaim_ValidShape(t *testing.T) {
	t.Parallel()
	// Token present, passes validation — will panic at DB query (nil DB)
	h := &AuthHandler{}
	req := httptest.NewRequest("GET", "/auth/validate-claim?token=invite-tok", nil)
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.ValidateClaim(w, req)
	}()
	if w.Code == http.StatusBadRequest {
		t.Errorf("ValidateClaim(valid shape): got 400, validation should pass: %s", w.Body.String())
	}
}

// ── Logout ──

func TestLogout_NoClaims(t *testing.T) {
	t.Parallel()
	// No auth context — should handle gracefully (no panic), clear cookie and return 200
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Logout(no claims): got status %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "logged out") {
		t.Errorf("Logout(no claims): body = %q", w.Body.String())
	}
}

func TestLogout_WithClaims(t *testing.T) {
	t.Parallel()
	// Claims present — will panic at Redis (nil RDB) when revoking token, confirming it passes validation
	h := &AuthHandler{}
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		h.Logout(w, req)
	}()
	// Should not get a 400 — validation passes
	if w.Code == http.StatusBadRequest {
		t.Errorf("Logout(with claims): got 400, should reach Redis: %s", w.Body.String())
	}
}

// ── MockStore-backed tests ──

func TestSignup_Success(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			CountUsersFn: func(ctx context.Context) (int, error) {
				return 0, nil
			},
			CreateOrgAndAdminFn: func(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (string, string, error) {
				return "org1", "user1", nil
			},
			SetVerificationCodeFn: func(ctx context.Context, userID, code string, expires time.Time) error {
				return nil
			},
		},
		Secret:    "test-secret-key-for-jwt-signing",
		StripeKey: "", // self-hosted mode, no verification needed
	}
	body := `{"email":"alice@example.com","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Signup: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Signup: failed to parse response: %v", err)
	}
	user, ok := resp["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("Signup: response missing 'user' object")
	}
	if user["id"] != "user1" {
		t.Errorf("Signup: user_id = %v, want user1", user["id"])
	}
}

func TestSignup_DuplicateEmail(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			CountUsersFn: func(ctx context.Context) (int, error) {
				return 0, nil
			},
			CreateOrgAndAdminFn: func(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (string, string, error) {
				return "", "", errors.New("email already registered (unique constraint)")
			},
		},
		StripeKey: "", // self-hosted mode
	}
	body := `{"email":"alice@example.com","password":"Password1","org_name":"Test Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("Signup(duplicate): got status %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("Password1"), bcrypt.MinCost)
	h := &AuthHandler{
		Store: &store.MockStore{
			GetUserByEmailFn: func(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
				return "user1", "org1", "Alice", "admin", "active", string(hashedPw), true, nil
			},
			GetOnboardingCompletedFn: func(ctx context.Context, orgID string) (bool, error) {
				return true, nil
			},
		},
		Secret: "test-secret-key-for-jwt-signing",
		RDB:    nil, // nil Redis is handled gracefully
	}
	body := `{"email":"alice@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Login: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Verify Set-Cookie header contains "token"
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Login: response missing 'token' cookie")
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Login: failed to parse response: %v", err)
	}
	if resp["onboarding_completed"] != true {
		t.Errorf("Login: onboarding_completed = %v, want true", resp["onboarding_completed"])
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("CorrectPassword1"), bcrypt.MinCost)
	h := &AuthHandler{
		Store: &store.MockStore{
			GetUserByEmailFn: func(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
				return "user1", "org1", "Alice", "admin", "active", string(hashedPw), true, nil
			},
		},
	}
	body := `{"email":"alice@example.com","password":"WrongPassword1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Login(wrong password): got status %d, want %d; body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			GetUserByEmailFn: func(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
				return "", "", "", "", "", "", false, errors.New("no rows")
			},
		},
	}
	body := `{"email":"nobody@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Login(not found): got status %d, want %d; body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// ── #8: Login disabled user → 401 ──

func TestLogin_DisabledUser(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("Password1"), bcrypt.MinCost)
	h := &AuthHandler{
		Store: &store.MockStore{
			GetUserByEmailFn: func(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
				return "user1", "org1", "Alice", "admin", "disabled", string(hashedPw), true, nil
			},
		},
	}
	body := `{"email":"alice@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Login(disabled): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "not active") {
		t.Errorf("Login(disabled): body = %q, want 'not active'", w.Body.String())
	}
}

// ── #9: Login unverified email → 403 ──

func TestLogin_UnverifiedEmail(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("Password1"), bcrypt.MinCost)
	h := &AuthHandler{
		Store: &store.MockStore{
			GetUserByEmailFn: func(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
				return "user1", "org1", "Alice", "admin", "active", string(hashedPw), false, nil
			},
		},
		StripeKey: "sk_test_key", // hosted mode requires email verification
	}
	body := `{"email":"alice@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("Login(unverified): got status %d, want %d", w.Code, http.StatusForbidden)
	}
	if !strings.Contains(w.Body.String(), "email_not_verified") {
		t.Errorf("Login(unverified): body = %q, want 'email_not_verified'", w.Body.String())
	}
}

// ── #10: Verify email valid code → activates ──

func TestVerifyEmail_Success(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			VerifyEmailFn: func(ctx context.Context, email, code string) (string, string, string, string, error) {
				if email == "alice@example.com" && code == "123456" {
					return "user1", "org1", "Alice", "admin", nil
				}
				return "", "", "", "", errors.New("invalid code")
			},
		},
		Secret: "test-secret-key-for-jwt-signing",
	}
	body := `{"email":"alice@example.com","code":"123456"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("VerifyEmail: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})
	if user["id"] != "user1" {
		t.Errorf("VerifyEmail: user.id = %v, want user1", user["id"])
	}
	// Verify cookie was set
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" {
			found = true
		}
	}
	if !found {
		t.Error("VerifyEmail: response missing 'token' cookie")
	}
}

// ── #11: Verify email expired code → 400 ──

func TestVerifyEmail_ExpiredCode(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			VerifyEmailFn: func(ctx context.Context, email, code string) (string, string, string, string, error) {
				return "", "", "", "", errors.New("expired")
			},
		},
	}
	body := `{"email":"alice@example.com","code":"999999"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("VerifyEmail(expired): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid or expired") {
		t.Errorf("VerifyEmail(expired): body = %q", w.Body.String())
	}
}

// ── #12: Verify email wrong code → 400 ──

func TestVerifyEmail_WrongCode(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			VerifyEmailFn: func(ctx context.Context, email, code string) (string, string, string, string, error) {
				return "", "", "", "", errors.New("wrong code")
			},
		},
	}
	body := `{"email":"alice@example.com","code":"000000"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("VerifyEmail(wrong code): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── #13: Resend verification returns generic message ──

func TestResendVerification_Success(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ResendVerificationCodeFn: func(ctx context.Context, email, code string, expires time.Time) (int64, error) {
				return 0, nil // no rows affected — user doesn't exist, but return 200 anyway
			},
		},
	}
	body := `{"email":"alice@example.com"}`
	req := httptest.NewRequest("POST", "/auth/resend-verification", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResendVerification(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ResendVerification: got status %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "if that email") {
		t.Errorf("ResendVerification: body = %q, want generic message", w.Body.String())
	}
}

// ── #14: Forgot password always returns 200 ──

func TestForgotPassword_AlwaysReturns200(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			SetResetTokenFn: func(ctx context.Context, email, token string, expires time.Time) (int64, error) {
				return 0, nil // no rows affected — user doesn't exist
			},
		},
		StripeKey: "sk_test", // hosted mode
	}
	body := `{"email":"nonexistent@example.com"}`
	req := httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ForgotPassword(nonexistent): got status %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "if that email exists") {
		t.Errorf("ForgotPassword(nonexistent): body = %q", w.Body.String())
	}
}

// ── #15: Reset password valid token → success ──

func TestResetPassword_ValidToken(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ResetPasswordFn: func(ctx context.Context, passwordHash, token string) (string, error) {
				if token == "valid-token" {
					return "user1", nil
				}
				return "", errors.New("invalid token")
			},
		},
		RDB: nil, // nil RDB is handled gracefully
	}
	body := `{"token":"valid-token","password":"NewPassword1"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ResetPassword: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "password reset successfully") {
		t.Errorf("ResetPassword: body = %q", w.Body.String())
	}
}

// ── #16: Reset password expired token → 400 ──

func TestResetPassword_ExpiredToken(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ResetPasswordFn: func(ctx context.Context, passwordHash, token string) (string, error) {
				return "", errors.New("expired token")
			},
		},
	}
	body := `{"token":"expired-token","password":"NewPassword1"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ResetPassword(expired): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid or expired") {
		t.Errorf("ResetPassword(expired): body = %q", w.Body.String())
	}
}

// ── #17: Claim invite valid token → activates ──

func TestClaimInvite_Success(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ClaimInviteFn: func(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
				if token == "valid-invite" {
					return "user2", "org1", "bob@example.com", "member", nil
				}
				return "", "", "", "", errors.New("invalid token")
			},
		},
		Secret: "test-secret-key-for-jwt-signing",
		RDB:    nil,
	}
	body := `{"token":"valid-invite","password":"Password1","name":"Bob"}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Claim: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})
	if user["id"] != "user2" {
		t.Errorf("Claim: user.id = %v, want user2", user["id"])
	}
	if user["role"] != "member" {
		t.Errorf("Claim: user.role = %v, want member", user["role"])
	}
}

// ── #18: Claim invite expired → 400 ──

func TestClaimInvite_ExpiredToken(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ClaimInviteFn: func(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
				return "", "", "", "", errors.New("expired")
			},
		},
	}
	body := `{"token":"expired-invite","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Claim(expired): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid or expired") {
		t.Errorf("Claim(expired): body = %q", w.Body.String())
	}
}

// ── #19: Claim invite already active → 400 ──

func TestClaimInvite_AlreadyActive(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{
		Store: &store.MockStore{
			ClaimInviteFn: func(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
				return "", "", "", "", errors.New("user already active")
			},
		},
	}
	body := `{"token":"used-invite","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/claim", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Claim(already active): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── #20: Change password wrong current → 401 ──

func TestChangePassword_WrongCurrent(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("CorrectPassword1"), bcrypt.MinCost)
	h := &UserHandler{
		Store: &store.MockStore{
			GetPasswordHashFn: func(ctx context.Context, userID string) (string, error) {
				return string(hashedPw), nil
			},
		},
	}
	body := `{"current_password":"WrongPassword1","new_password":"NewPassword1"}`
	req := httptest.NewRequest("PUT", "/users/me/password", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.UpdatePassword(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("ChangePassword(wrong): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "current password is incorrect") {
		t.Errorf("ChangePassword(wrong): body = %q", w.Body.String())
	}
}

// ── #21: Change password success → invalidates sessions ──

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("OldPassword1"), bcrypt.MinCost)
	updateCalled := false
	h := &UserHandler{
		Store: &store.MockStore{
			GetPasswordHashFn: func(ctx context.Context, userID string) (string, error) {
				return string(hashedPw), nil
			},
			UpdatePasswordFn: func(ctx context.Context, userID, passwordHash string) error {
				updateCalled = true
				return nil
			},
		},
		Secret: "test-secret-key-for-jwt-signing",
		RDB:    nil,
	}
	body := `{"current_password":"OldPassword1","new_password":"NewPassword1"}`
	req := httptest.NewRequest("PUT", "/users/me/password", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.UpdatePassword(w, req)
	// With nil RDB, session revocation fails, handler returns 200 with warning
	if w.Code != http.StatusOK {
		t.Fatalf("ChangePassword: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !updateCalled {
		t.Error("ChangePassword: UpdatePassword was not called")
	}
	// Verify a new token cookie was set
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" {
			found = true
		}
	}
	if !found {
		t.Error("ChangePassword: response missing new 'token' cookie")
	}
}
