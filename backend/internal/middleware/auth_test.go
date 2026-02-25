package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestGenerateToken_Valid(t *testing.T) {
	t.Parallel()
	tokenStr, err := GenerateToken(testSecret, "user1", "org1", "admin")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if !token.Valid {
		t.Error("token.Valid = false, want true")
	}
	if claims.UserID != "user1" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user1")
	}
	if claims.OrgID != "org1" {
		t.Errorf("OrgID: got %q, want %q", claims.OrgID, "org1")
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q, want %q", claims.Role, "admin")
	}
	// Expiry should be ~7 days from now
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	expiryDiff := time.Until(claims.ExpiresAt.Time)
	if expiryDiff < 6*24*time.Hour || expiryDiff > 8*24*time.Hour {
		t.Errorf("ExpiresAt: %v from now, want ~7 days", expiryDiff)
	}
}

func TestGenerateToken_DifferentSecrets(t *testing.T) {
	t.Parallel()
	tokenStr, err := GenerateToken("secret-A", "user1", "org1", "admin")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte("secret-B"), nil
	})
	if err == nil {
		t.Error("ParseWithClaims(wrong secret): expected error, got nil")
	}
}

func TestGetCurrentUser_NoClaimsInContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	claims := GetCurrentUser(ctx)
	if claims != nil {
		t.Errorf("GetCurrentUser(empty ctx): got %+v, want nil", claims)
	}
}

func TestGetCurrentUser_WithClaims(t *testing.T) {
	t.Parallel()
	expected := &Claims{UserID: "user1", OrgID: "org1", Role: "admin"}
	ctx := context.WithValue(context.Background(), UserContextKey, expected)
	got := GetCurrentUser(ctx)
	if got == nil {
		t.Fatal("GetCurrentUser: got nil, want claims")
	}
	if got.UserID != expected.UserID || got.OrgID != expected.OrgID || got.Role != expected.Role {
		t.Errorf("GetCurrentUser: got %+v, want %+v", got, expected)
	}
}

func TestAuthMiddleware_NoCookie(t *testing.T) {
	t.Parallel()
	handler := AuthMiddleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware(no cookie): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	t.Parallel()
	handler := AuthMiddleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "garbage-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware(invalid token): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	t.Parallel()
	tokenStr, err := GenerateToken(testSecret, "user1", "org1", "member")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	called := false
	handler := AuthMiddleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims := GetCurrentUser(r.Context())
		if claims == nil {
			t.Error("claims is nil in next handler")
			return
		}
		if claims.UserID != "user1" {
			t.Errorf("claims.UserID: got %q, want %q", claims.UserID, "user1")
		}
		if claims.OrgID != "org1" {
			t.Errorf("claims.OrgID: got %q, want %q", claims.OrgID, "org1")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("next handler was not called")
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	t.Parallel()
	// Generate a token that expired in the past
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)), // expired 1 hour ago
		},
		UserID: "user1",
		OrgID:  "org1",
		Role:   "admin",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	handler := AuthMiddleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for expired token")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware(expired): got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdmin_AdminRole(t *testing.T) {
	t.Parallel()
	called := false
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	claims := &Claims{UserID: "user1", OrgID: "org1", Role: "admin"}
	ctx := context.WithValue(req.Context(), UserContextKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("RequireAdmin(admin): next handler was not called")
	}
}

func TestRequireAdmin_MemberRole(t *testing.T) {
	t.Parallel()
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for member")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	claims := &Claims{UserID: "user1", OrgID: "org1", Role: "member"}
	ctx := context.WithValue(req.Context(), UserContextKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("RequireAdmin(member): got status %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAdmin_NoClaims(t *testing.T) {
	t.Parallel()
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called without claims")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("RequireAdmin(no claims): got status %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequirePlan_NoStripeKey(t *testing.T) {
	t.Parallel()
	called := false
	handler := RequirePlan("", nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("RequirePlan(no stripe key): next handler was not called")
	}
}

func TestSetTokenCookie_HTTPS(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	SetTokenCookie(w, "test-token", "https://app.example.com")
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("SetTokenCookie(HTTPS): no cookies set")
	}
	cookie := cookies[0]
	if !cookie.Secure {
		t.Error("SetTokenCookie(HTTPS): Secure = false, want true")
	}
	if cookie.Name != "token" {
		t.Errorf("SetTokenCookie(HTTPS): Name = %q, want %q", cookie.Name, "token")
	}
}

func TestSetTokenCookie_HTTP(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	SetTokenCookie(w, "test-token", "http://localhost:3000")
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("SetTokenCookie(HTTP): no cookies set")
	}
	cookie := cookies[0]
	if cookie.Secure {
		t.Error("SetTokenCookie(HTTP): Secure = true, want false")
	}
}

func TestClearTokenCookie(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ClearTokenCookie(w, "https://app.example.com")
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("ClearTokenCookie: no cookies set")
	}
	cookie := cookies[0]
	if cookie.MaxAge != -1 {
		t.Errorf("ClearTokenCookie: MaxAge = %d, want -1", cookie.MaxAge)
	}
	if cookie.Name != "token" {
		t.Errorf("ClearTokenCookie: Name = %q, want %q", cookie.Name, "token")
	}
	if cookie.Value != "" {
		t.Errorf("ClearTokenCookie: Value = %q, want empty", cookie.Value)
	}
}

// Suppress lint about unused imports
func init() {
	_ = strings.Contains
}
