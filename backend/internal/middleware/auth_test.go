package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestGenerateToken_Valid(t *testing.T) {
	t.Parallel()
	tokenStr, _, err := GenerateToken(testSecret, "user1", "org1", "admin")
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
	tokenStr, _, err := GenerateToken("secret-A", "user1", "org1", "admin")
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
	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	tokenStr, _, err := GenerateToken(testSecret, "user1", "org1", "member")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	called := false
	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if cookie.Domain != "example.com" {
		t.Errorf("SetTokenCookie(HTTPS): Domain = %q, want %q", cookie.Domain, "example.com")
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
	if cookie.Domain != "" {
		t.Errorf("SetTokenCookie(HTTP): Domain = %q, want empty", cookie.Domain)
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
	if cookie.Domain != "example.com" {
		t.Errorf("ClearTokenCookie: Domain = %q, want %q", cookie.Domain, "example.com")
	}
}

func TestCookieDomain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		appURL string
		want   string
	}{
		{"subdomain", "https://app.inboxes.net", "inboxes.net"},
		{"root domain", "https://inboxes.net", "inboxes.net"},
		{"localhost", "http://localhost:3000", ""},
		{"IP address", "http://192.168.1.1:8080", ""},
		{"IPv6", "http://[::1]:8080", ""},
		{"single-label host", "http://myhost:8080", ""},
		{"invalid URL", "://bad", ""},
		{"deep subdomain", "https://a.b.example.com", "example.com"},
		{"multi-label suffix ts.net", "https://app.tail1234.ts.net", "tail1234.ts.net"},
		{"multi-label suffix ts.net root", "https://tail1234.ts.net", "tail1234.ts.net"},
		{"multi-label suffix co.uk", "https://app.example.co.uk", "example.co.uk"},
		{"deep multi-label suffix", "https://a.b.example.co.uk", "example.co.uk"},
		{"bare public suffix", "https://co.uk", ""},
		{"uppercase host", "https://App.Example.CO.UK", "example.co.uk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cookieDomain(tt.appURL)
			if got != tt.want {
				t.Errorf("cookieDomain(%q) = %q, want %q", tt.appURL, got, tt.want)
			}
		})
	}
}

// Suppress lint about unused imports
func init() {
	_ = strings.Contains
}

func TestAuthMiddleware_SlidingRenewal(t *testing.T) {
	t.Parallel()
	// A token older than tokenRenewAge gets a fresh cookie.
	now := time.Now()
	oldClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "old-jti",
			IssuedAt:  jwt.NewNumericDate(now.Add(-25 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(6 * 24 * time.Hour)),
		},
		UserID: "user1",
		OrgID:  "org1",
		Role:   "member",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, oldClaims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var renewed string
	for _, c := range w.Result().Cookies() {
		if c.Name == "token" {
			renewed = c.Value
		}
	}
	if renewed == "" {
		t.Fatal("expected a renewed token cookie, got none")
	}
	newClaims := &Claims{}
	if _, err := jwt.ParseWithClaims(renewed, newClaims, func(t *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	}); err != nil {
		t.Fatalf("parse renewed token: %v", err)
	}
	if newClaims.UserID != "user1" || newClaims.OrgID != "org1" || newClaims.Role != "member" {
		t.Errorf("renewed claims: got %+v", newClaims)
	}
	if newClaims.ID == "old-jti" {
		t.Error("renewed token kept the old JTI")
	}
	if newClaims.Prev != "old-jti" {
		t.Errorf("renewed token must name the token it replaced: got prv %q", newClaims.Prev)
	}
	if time.Until(newClaims.ExpiresAt.Time) < 6*24*time.Hour {
		t.Errorf("renewed expiry too short: %v", time.Until(newClaims.ExpiresAt.Time))
	}
}

func TestAuthMiddleware_NoRenewalForFreshToken(t *testing.T) {
	t.Parallel()
	tokenStr, _, err := GenerateToken(testSecret, "user1", "org1", "member")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	handler := AuthMiddleware(testSecret, nil, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == "token" {
			t.Error("fresh token must not be renewed")
		}
	}
}

// oldToken signs a token that is past tokenRenewAge.
func oldToken(t *testing.T, userID, jti string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now.Add(-25 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(6 * 24 * time.Hour)),
		},
		UserID: userID,
		OrgID:  "org1",
		Role:   "member",
	})
	s, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func tokenCookie(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == "token" {
			return c.Value
		}
	}
	return ""
}

func TestAuthMiddleware_DatabaseErrorIs503(t *testing.T) {
	t.Parallel()
	// Nothing listens on port 1, so each query fails with a connection error.
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	tokenStr, _, err := GenerateToken(testSecret, "user1", "org1", "member")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	handler := AuthMiddleware(testSecret, nil, pool, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("database error: got status %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// A burst of parallel requests with one old token must mint one new token,
// must not reject a request, and must not evict the other sessions.
func TestAuthMiddleware_RenewalStorm(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()
	ctx := context.Background()
	bl := service.NewTokenBlacklist(rdb)

	user := "test-user-" + uuid.NewString()
	oldJTI, otherJTI := uuid.NewString(), uuid.NewString()
	defer rdb.Del(ctx, "sessions:"+user, "token:renew:"+oldJTI, "token:bl:"+oldJTI)
	bl.RegisterSession(ctx, user, otherJTI)
	bl.RegisterSession(ctx, user, oldJTI)
	tokenStr := oldToken(t, user, oldJTI)

	handler := AuthMiddleware(testSecret, rdb, nil, "http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const n = 12
	recs := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		recs[i] = httptest.NewRecorder()
		wg.Add(1)
		go func(w *httptest.ResponseRecorder) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
			handler.ServeHTTP(w, req)
		}(recs[i])
	}
	wg.Wait()

	var renewed string
	count := 0
	for _, w := range recs {
		if w.Code != http.StatusOK {
			t.Errorf("request in the burst: got status %d, want 200", w.Code)
		}
		if c := tokenCookie(w); c != "" {
			renewed = c
			count++
		}
	}
	if count != 1 {
		t.Fatalf("renewed cookies: got %d, want 1", count)
	}
	if got := len(bl.ListSessions(ctx, user)); got != 2 {
		t.Errorf("session count after the burst: got %d, want 2", got)
	}
	if bl.IsRevoked(ctx, otherJTI, user, time.Now()) {
		t.Error("the burst evicted the session of another device")
	}

	// The first request with the new token retires the old token after a
	// grace period. Inside the grace period the old token still works.
	for _, tok := range []string{renewed, tokenStr} {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "token", Value: tok})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request after renewal: got status %d, want 200", w.Code)
		}
	}
	val, err := rdb.Get(ctx, "token:bl:"+oldJTI).Int64()
	if err != nil {
		t.Fatalf("old token has no retire deadline: %v", err)
	}
	if val <= time.Now().Unix() {
		t.Errorf("retire deadline is not in the future: %d", val)
	}

	// After the deadline the old token is dead.
	rdb.Set(ctx, "token:bl:"+oldJTI, time.Now().Add(-time.Second).Unix(), time.Minute)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenStr})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("old token after the grace period: got status %d, want 401", w.Code)
	}
}
