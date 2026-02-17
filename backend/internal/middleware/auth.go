package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const UserContextKey contextKey = "user"

type Claims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
}

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
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

func GenerateToken(secret, userID, orgID, role string) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
		},
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
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
				"SELECT plan, plan_expires_at FROM orgs WHERE id = $1", claims.OrgID,
			).Scan(&plan, &planExpiresAt)
			if err != nil {
				http.Error(w, `{"error":"subscription_required"}`, http.StatusPaymentRequired)
				return
			}

			if plan == "pro" {
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
