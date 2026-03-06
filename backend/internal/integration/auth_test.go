//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/service"
)

// newAuthHandler creates an AuthHandler backed by the real test store and Redis.
func newAuthHandler() *handler.AuthHandler {
	return &handler.AuthHandler{
		Store:     testStore,
		RDB:       testRDB,
		Secret:    "integration-test-secret-32chars!",
		AppURL:    "http://localhost:3000",
		StripeKey: "", // self-hosted mode: no email verification, solo signup allowed
	}
}

func TestAuth_SignupAndLogin(t *testing.T) {
	truncateAll(context.Background())

	h := newAuthHandler()

	// --- Signup ---
	body := `{"email":"integ-signup@example.com","password":"Password1","org_name":"Integ Org","name":"Alice"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Signup: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var signupResp map[string]interface{}
	parseJSON(t, w, &signupResp)
	user, ok := signupResp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Signup: response missing 'user' object")
	}
	userID := user["id"].(string)
	orgID := user["org_id"].(string)
	if userID == "" || orgID == "" {
		t.Fatalf("Signup: empty user_id=%q or org_id=%q", userID, orgID)
	}

	// Verify token cookie was set
	cookies := w.Result().Cookies()
	var tokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "token" {
			tokenCookie = c
			break
		}
	}
	if tokenCookie == nil {
		t.Fatal("Signup: response missing 'token' cookie")
	}

	// --- Login ---
	loginBody := `{"email":"integ-signup@example.com","password":"Password1"}`
	req2 := httptest.NewRequest("POST", "/auth/login", strings.NewReader(loginBody))
	w2 := httptest.NewRecorder()
	h.Login(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Login: got status %d, want %d; body: %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	var loginResp map[string]interface{}
	parseJSON(t, w2, &loginResp)
	loginUser, ok := loginResp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Login: response missing 'user' object")
	}
	if loginUser["id"] != userID {
		t.Errorf("Login: user_id = %v, want %v", loginUser["id"], userID)
	}
	if loginUser["role"] != "admin" {
		t.Errorf("Login: role = %v, want admin", loginUser["role"])
	}

	// Verify login token cookie
	loginCookies := w2.Result().Cookies()
	found := false
	for _, c := range loginCookies {
		if c.Name == "token" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Login: response missing 'token' cookie")
	}

	t.Cleanup(func() { cleanupOrg(t, orgID) })
}

func TestAuth_DuplicateSignup(t *testing.T) {
	truncateAll(context.Background())

	h := newAuthHandler()

	// First signup succeeds
	body := `{"email":"integ-dup@example.com","password":"Password1","org_name":"Dup Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("First signup: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	orgID := resp["user"].(map[string]interface{})["org_id"].(string)
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// In self-hosted mode, second signup is blocked because a user already exists (solo mode).
	body2 := `{"email":"integ-dup2@example.com","password":"Password1","org_name":"Another Org"}`
	req2 := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body2))
	w2 := httptest.NewRecorder()
	h.Signup(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Errorf("Second signup (solo mode): got status %d, want %d; body: %s", w2.Code, http.StatusForbidden, w2.Body.String())
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	h := newAuthHandler()

	// Seed user
	orgID, _ := seedOrg(t, "WrongPW Org", "integ-wrongpw@example.com", "CorrectPass1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	body := `{"email":"integ-wrongpw@example.com","password":"WrongPass1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Login(wrong password): got status %d, want %d; body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	if resp["error"] != "invalid credentials" {
		t.Errorf("Login(wrong password): error = %v, want 'invalid credentials'", resp["error"])
	}
}

func TestAuth_LoginNonexistentUser(t *testing.T) {
	h := newAuthHandler()

	body := `{"email":"nobody-integ@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Login(nonexistent): got status %d, want %d; body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestAuth_LoginEmailNormalization(t *testing.T) {
	h := newAuthHandler()

	orgID, _ := seedOrg(t, "Norm Org", "integ-norm@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Login with uppercase/whitespace email -- should still match
	body := `{"email":"  INTEG-NORM@EXAMPLE.COM  ","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Login(normalized email): got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAuth_SignupNameDefaultsToEmailPrefix(t *testing.T) {
	// In self-hosted mode, only one user can exist. Clear all users first.
	truncateAll(context.Background())

	h := newAuthHandler()

	// Signup without name -- should default to email prefix
	body := `{"email":"integ-noname@example.com","password":"Password1","org_name":"NoName Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Signup(no name): got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	user := resp["user"].(map[string]interface{})
	orgID := user["org_id"].(string)
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	if user["name"] != "integ-noname" {
		t.Errorf("Signup(no name): name = %v, want 'integ-noname'", user["name"])
	}
}

func TestAuth_LoginReturnsOnboardingStatus(t *testing.T) {
	h := newAuthHandler()

	orgID, _ := seedOrg(t, "Onboarding Org", "integ-onb@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	body := `{"email":"integ-onb@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login: got status %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)

	// onboarding_completed should be false for a fresh org
	if resp["onboarding_completed"] != false {
		t.Errorf("Login: onboarding_completed = %v, want false", resp["onboarding_completed"])
	}
}

func TestAuth_SignupValidation(t *testing.T) {
	// Use hosted mode (StripeKey set) so the solo-user check is skipped
	// and validation errors are returned as expected.
	h := &handler.AuthHandler{
		Store:     testStore,
		RDB:       testRDB,
		Secret:    "integration-test-secret-32chars!",
		AppURL:    "http://localhost:3000",
		StripeKey: "sk_test_fake",
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing email",
			body:       `{"email":"","password":"Password1","org_name":"Test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "required",
		},
		{
			name:       "invalid email format",
			body:       `{"email":"not-an-email","password":"Password1","org_name":"Test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "email",
		},
		{
			name:       "weak password",
			body:       `{"email":"valid@example.com","password":"weak","org_name":"Test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "password",
		},
		{
			name:       "missing org name",
			body:       `{"email":"valid@example.com","password":"Password1","org_name":""}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "required",
		},
		{
			name:       "invalid JSON",
			body:       `{bad json`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.Signup(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantError) {
				t.Errorf("body = %q, want to contain %q", w.Body.String(), tt.wantError)
			}
		})
	}
}

func TestAuth_ResetPasswordFlow(t *testing.T) {
	h := newAuthHandler()

	orgID, _ := seedOrg(t, "Reset Org", "integ-reset@example.com", "OldPassword1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Set reset token directly in DB (we can't call ForgotPassword without ResendSvc).
	// Use raw SQL to set a token with a future expiry.
	_, err := testPool.Exec(context.Background(),
		"UPDATE users SET reset_token = $1, reset_expires_at = now() + interval '1 hour' WHERE email = $2",
		"test-reset-token-123", "integ-reset@example.com",
	)
	if err != nil {
		t.Fatalf("set reset token: %v", err)
	}

	// Now reset password
	body := `{"token":"test-reset-token-123","password":"NewPassword1"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ResetPassword: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify login with new password works
	loginBody := `{"email":"integ-reset@example.com","password":"NewPassword1"}`
	req2 := httptest.NewRequest("POST", "/auth/login", strings.NewReader(loginBody))
	w2 := httptest.NewRecorder()
	h.Login(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Login(new password): got status %d, want %d; body: %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	// Verify login with old password fails
	oldLoginBody := `{"email":"integ-reset@example.com","password":"OldPassword1"}`
	req3 := httptest.NewRequest("POST", "/auth/login", strings.NewReader(oldLoginBody))
	w3 := httptest.NewRecorder()
	h.Login(w3, req3)

	if w3.Code != http.StatusUnauthorized {
		t.Errorf("Login(old password): got status %d, want %d; body: %s", w3.Code, http.StatusUnauthorized, w3.Body.String())
	}
}

func TestAuth_ResetPasswordInvalidToken(t *testing.T) {
	h := newAuthHandler()

	body := `{"token":"nonexistent-token","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ResetPassword(bad token): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "invalid") {
		t.Errorf("ResetPassword(bad token): error = %q, want to contain 'invalid'", errMsg)
	}
}

func TestAuth_LogoutClearsCookie(t *testing.T) {
	h := newAuthHandler()

	orgID, userID := seedOrg(t, "Logout Org", "integ-logout@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Simulate an authenticated request
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req = withClaims(req, userID, orgID, "admin")
	w := httptest.NewRecorder()
	h.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Logout: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify token cookie was cleared (MaxAge = -1)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "token" {
			if c.MaxAge >= 0 {
				t.Errorf("Logout: token cookie MaxAge = %d, want < 0", c.MaxAge)
			}
			if c.Value != "" {
				t.Errorf("Logout: token cookie value = %q, want empty", c.Value)
			}
			return
		}
	}
	t.Error("Logout: no 'token' cookie in response")
}

func TestAuth_LoginResponseShape(t *testing.T) {
	h := newAuthHandler()

	orgID, _ := seedOrg(t, "Shape Org", "integ-shape@example.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	body := `{"email":"integ-shape@example.com","password":"Password1"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login: got status %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)

	// Verify response shape contains expected fields
	user, ok := resp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("Login: response missing 'user' object")
	}

	requiredFields := []string{"id", "org_id", "email", "name", "role"}
	for _, f := range requiredFields {
		if _, exists := user[f]; !exists {
			t.Errorf("Login: user object missing field %q", f)
		}
	}

	if _, exists := resp["onboarding_completed"]; !exists {
		t.Error("Login: response missing 'onboarding_completed' field")
	}

	// Verify Content-Type header
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Login: Content-Type = %q, want application/json", ct)
	}
}

// TestAuth_DuplicateEmailConflict tests that creating two users with the same email
// returns a 409 conflict (via hosted mode to bypass solo check).
func TestAuth_DuplicateEmailConflict(t *testing.T) {
	// Hosted mode requires ResendSvc for sending verification emails.
	// Provide a real ResendService with a dummy system key -- the actual send
	// will fail (bad key), but the handler logs the error and continues.
	resendSvc := service.NewResendService(testEncSvc, testPool, "re_test_dummy", "noreply@test.example.com")

	h := &handler.AuthHandler{
		Store:     testStore,
		RDB:       testRDB,
		Secret:    "integration-test-secret-32chars!",
		AppURL:    "http://localhost:3000",
		ResendSvc: resendSvc,
		StripeKey: "sk_test_fake", // hosted mode: no solo check, email verification returned
	}

	// Signup first user
	body := `{"email":"integ-conflict@example.com","password":"Password1","org_name":"Conflict Org"}`
	req := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Signup(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("First signup: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Get org_id from the user in DB for cleanup
	var firstOrgID string
	err := testPool.QueryRow(context.Background(),
		"SELECT org_id FROM users WHERE email = $1", "integ-conflict@example.com",
	).Scan(&firstOrgID)
	if err != nil {
		t.Fatalf("query first user org_id: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, firstOrgID) })

	// Second signup with same email
	body2 := `{"email":"integ-conflict@example.com","password":"Password1","org_name":"Another Org"}`
	req2 := httptest.NewRequest("POST", "/auth/signup", strings.NewReader(body2))
	w2 := httptest.NewRecorder()
	h.Signup(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("Duplicate signup: got status %d, want %d; body: %s", w2.Code, http.StatusConflict, w2.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["error"] != "email already registered" {
		t.Errorf("Duplicate signup: error = %v, want 'email already registered'", resp["error"])
	}
}

func TestAuth_InviteTokenExpired(t *testing.T) {
	ctx := context.Background()
	truncateAll(ctx)

	h := newAuthHandler()

	orgID, _ := seedOrg(t, "Expired Invite Org", "expired-invite-admin@test.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	// Insert an invited user with an expiry 1 hour in the past
	expiredAt := time.Now().Add(-1 * time.Hour)
	_, err := testStore.InsertInvitedUser(ctx, orgID, "expired-invite@test.com", "Expired User", "member", "expired-token-abc", expiredAt)
	if err != nil {
		t.Fatalf("InsertInvitedUser: %v", err)
	}

	// Try to claim the expired invite
	body := `{"token":"expired-token-abc","password":"NewPassword1","name":"Claimed User"}`
	req := httptest.NewRequest("POST", "/auth/claim-invite", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Claim(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ClaimInvite(expired): got status %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp map[string]interface{}
	parseJSON(t, w, &resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "expired") && !strings.Contains(errMsg, "invalid") {
		t.Errorf("ClaimInvite(expired): error = %q, want to contain 'expired' or 'invalid'", errMsg)
	}
}

func TestAuth_InviteSameEmailTwice(t *testing.T) {
	ctx := context.Background()
	truncateAll(ctx)

	orgID, _ := seedOrg(t, "Dupe Invite Org", "dupe-invite-admin@test.com", "Password1")
	t.Cleanup(func() { cleanupOrg(t, orgID) })

	expiresAt := time.Now().Add(24 * time.Hour)

	// First invite
	_, err := testStore.InsertInvitedUser(ctx, orgID, "dupe@test.com", "First Invite", "member", "invite-token-1", expiresAt)
	if err != nil {
		t.Fatalf("InsertInvitedUser (first): %v", err)
	}

	// Second invite with the same email -- the ON CONFLICT upsert should
	// succeed (update existing row), so err should be nil.
	_, err = testStore.InsertInvitedUser(ctx, orgID, "dupe@test.com", "Second Invite", "member", "invite-token-2", expiresAt)
	if err != nil {
		t.Errorf("InsertInvitedUser (second): unexpected error: %v", err)
	}
}
