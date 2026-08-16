package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type contextKey string

const UserContextKey contextKey = "user"

type Claims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
}

// tokenRenewAge is the token age after which the middleware reissues the
// cookie. Active users stay logged in; an idle session still expires with
// the 7-day token lifetime.
const tokenRenewAge = 24 * time.Hour

func AuthMiddleware(secret string, rdb *redis.Client, db *pgxpool.Pool, appURL string) func(http.Handler) http.Handler {
	blacklist := service.NewTokenBlacklist(rdb)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("token")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Check token revocation (fail open if Redis is down)
			issuedAt := time.Time{}
			if claims.IssuedAt != nil {
				issuedAt = claims.IssuedAt.Time
			}
			if blacklist.IsRevoked(r.Context(), claims.ID, claims.UserID, issuedAt) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Check user is still active (cached in Redis, 2-min TTL)
			if db != nil {
				statusKey := "user:status:" + claims.UserID
				var status string
				var cached bool
				if rdb != nil {
					val, redisErr := rdb.Get(r.Context(), statusKey).Result()
					if redisErr == nil {
						status = val
						cached = true
					}
				}
				if !cached {
					var dbStatus string
					var orgDeletedAt *time.Time
					dbErr := db.QueryRow(r.Context(),
						"SELECT u.status, o.deleted_at FROM users u JOIN orgs o ON o.id = u.org_id WHERE u.id = $1",
						claims.UserID,
					).Scan(&dbStatus, &orgDeletedAt)
					if dbErr != nil || dbStatus != "active" || orgDeletedAt != nil {
						http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
						return
					}
					status = dbStatus
					if rdb != nil {
						rdb.Set(r.Context(), statusKey, status, 2*time.Minute)
					}
				}
				if status != "active" {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
			}

			// CSRF defense-in-depth: require X-Requested-With on state-changing methods.
			// SameSite=Lax cookies + this header check prevents cross-origin form submissions.
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				if r.Header.Get("X-Requested-With") == "" {
					http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
					return
				}
			}

			// Sliding renewal: reissue the cookie once the token is older
			// than tokenRenewAge. The role comes fresh from the DB so a
			// role change does not persist in renewed tokens.
			if !issuedAt.IsZero() && time.Since(issuedAt) > tokenRenewAge {
				role := claims.Role
				if db != nil {
					var dbRole string
					if qErr := db.QueryRow(r.Context(),
						"SELECT role FROM users WHERE id = $1", claims.UserID,
					).Scan(&dbRole); qErr == nil {
						role = dbRole
					}
				}
				if newToken, jti, genErr := GenerateToken(secret, claims.UserID, claims.OrgID, role); genErr == nil {
					SetTokenCookie(w, newToken, appURL)
					blacklist.RegisterSession(r.Context(), claims.UserID, jti)
				}
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetCurrentUser(ctx context.Context) *Claims {
	claims, _ := ctx.Value(UserContextKey).(*Claims)
	return claims
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetCurrentUser(r.Context())
		if claims == nil || claims.Role != "admin" {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GenerateToken(secret, userID, orgID, role string) (tokenStr string, jti string, err error) {
	now := time.Now()
	jti = uuid.NewString()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
		},
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err = token.SignedString([]byte(secret))
	return
}

// cookieDomain derives a cross-subdomain cookie domain from appURL.
// For "https://app.inboxes.net" it returns ".inboxes.net".
// For localhost, IPs, or single-label hosts it returns "" (browser default).
func cookieDomain(appURL string) string {
	u, err := url.Parse(appURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" || host == "localhost" {
		return ""
	}
	// IP address — no domain attribute
	if net.ParseIP(host) != nil {
		return ""
	}
	// Single-label host (no dots) — no domain attribute
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return ""
	}
	// Return parent domain (browsers treat Domain=x.com as .x.com per RFC 6265)
	return strings.Join(parts[len(parts)-2:], ".")
}

func SetTokenCookie(w http.ResponseWriter, token, appURL string) {
	secure := strings.HasPrefix(appURL, "https")
	c := &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if d := cookieDomain(appURL); d != "" {
		c.Domain = d
	}
	http.SetCookie(w, c)
}

func ClearTokenCookie(w http.ResponseWriter, appURL string) {
	secure := strings.HasPrefix(appURL, "https")
	c := &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if d := cookieDomain(appURL); d != "" {
		c.Domain = d
	}
	http.SetCookie(w, c)
}

// RequireOwner restricts access to the instance owner (is_owner = true).
func RequireOwner(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetCurrentUser(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			var isOwner bool
			err := db.QueryRow(r.Context(),
				"SELECT is_owner FROM users WHERE id = $1", claims.UserID,
			).Scan(&isOwner)
			if err != nil || !isOwner {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePlan enforces an active subscription when Stripe is configured.
// When stripeKey is empty (self-hosted), all requests pass through.
// Lapsed orgs (downgraded after a grace period) keep read-only access:
// GET and HEAD requests pass, all other methods get a 402.
func RequirePlan(stripeKey string, db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if stripeKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims := GetCurrentUser(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			var plan string
			var planExpiresAt, lapsedAt *time.Time
			err := db.QueryRow(r.Context(),
				"SELECT plan, plan_expires_at, lapsed_at FROM orgs WHERE id = $1 AND deleted_at IS NULL", claims.OrgID,
			).Scan(&plan, &planExpiresAt, &lapsedAt)
			if err != nil {
				writePlanRequired(w, "", nil, false)
				return
			}

			if plan == "pro" {
				next.ServeHTTP(w, r)
				return
			}
			// past_due keeps access until the grace period ends. A past_due
			// org without an expiry (for example a paused subscription) also
			// keeps access. cancelled keeps access only until the expiry.
			inGrace := planExpiresAt == nil || planExpiresAt.After(time.Now())
			if plan == "past_due" && inGrace {
				next.ServeHTTP(w, r)
				return
			}
			if plan == "cancelled" && planExpiresAt != nil && planExpiresAt.After(time.Now()) {
				next.ServeHTTP(w, r)
				return
			}

			// Read-only mode for orgs that had a plan before. A cancelled org
			// past its grace period counts as lapsed even before the grace
			// period worker downgrades it.
			lapsed := lapsedAt != nil || plan == "cancelled" || plan == "past_due"
			if lapsed && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				next.ServeHTTP(w, r)
				return
			}

			writePlanRequired(w, plan, planExpiresAt, lapsed)
		})
	}
}

// writePlanRequired writes a 402 with enough detail for the frontend to route
// the user to the correct billing state.
func writePlanRequired(w http.ResponseWriter, plan string, planExpiresAt *time.Time, readOnly bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	resp := map[string]interface{}{
		"error":           "subscription_required",
		"plan":            plan,
		"plan_expires_at": planExpiresAt,
		"read_only":       readOnly,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("middleware: write 402 response failed", "error", err)
	}
}
