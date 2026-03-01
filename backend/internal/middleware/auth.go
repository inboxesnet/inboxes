package middleware

import (
	"context"
	"net/http"
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

func AuthMiddleware(secret string, rdb *redis.Client, db *pgxpool.Pool) func(http.Handler) http.Handler {
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

func SetTokenCookie(w http.ResponseWriter, token, appURL string) {
	secure := strings.HasPrefix(appURL, "https")
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearTokenCookie(w http.ResponseWriter, appURL string) {
	secure := strings.HasPrefix(appURL, "https")
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
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
			var planExpiresAt *time.Time
			err := db.QueryRow(r.Context(),
				"SELECT plan, plan_expires_at FROM orgs WHERE id = $1 AND deleted_at IS NULL", claims.OrgID,
			).Scan(&plan, &planExpiresAt)
			if err != nil {
				http.Error(w, `{"error":"subscription_required"}`, http.StatusPaymentRequired)
				return
			}

			if plan == "pro" || plan == "past_due" {
				next.ServeHTTP(w, r)
				return
			}
			if plan == "cancelled" && planExpiresAt != nil && planExpiresAt.After(time.Now()) {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(`{"error":"subscription_required"}`))
		})
	}
}
