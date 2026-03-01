package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("writeJSON: failed to encode response", "error", err)
	}
}

func readJSON(r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB limit
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// marshalOrFail marshals v to JSON. On failure it logs, writes a 500, and returns nil, false.
// Callers should `if !ok { return }` after the call.
func marshalOrFail(w http.ResponseWriter, v interface{}, userMsg string) ([]byte, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error(userMsg, "error", err)
		writeError(w, http.StatusInternalServerError, userMsg)
		return nil, false
	}
	return b, true
}

// warnIfErr logs a warning if err is non-nil. Use for non-critical lookups that have a fallback.
func warnIfErr(err error, msg string, args ...any) {
	if err != nil {
		slog.Warn(msg, append(args, "error", err)...)
	}
}

// normalizeEmail lowercases and trims whitespace from an email address.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validateEmail checks that the email is well-formed (RFC 5322) and within length limits.
func validateEmail(email string) error {
	if len(email) > 254 {
		return fmt.Errorf("email address too long")
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// validatePassword enforces password complexity requirements.
func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return fmt.Errorf("password must be 8-128 characters with at least one uppercase letter, one lowercase letter, and one digit")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("password must be 8-128 characters with at least one uppercase letter, one lowercase letter, and one digit")
	}
	return nil
}

// validateLength checks that a string does not exceed the given maximum length.
func validateLength(value, field string, max int) error {
	if len(value) > max {
		return fmt.Errorf("%s must be %d characters or fewer", field, max)
	}
	return nil
}

// canSendAs checks whether a user is authorized to send email from the given address.
// Admins can always send. Non-admins need either:
// - alias_users.can_send_as=true for an alias matching the address, OR
// - the address is their own user email.
func canSendAs(ctx context.Context, db *pgxpool.Pool, userID, orgID, fromAddress, role string) bool {
	if role == "admin" {
		return true
	}

	// Check if sending from own email
	var userEmail string
	err := db.QueryRow(ctx,
		"SELECT email FROM users WHERE id = $1 AND org_id = $2 AND status = 'active'",
		userID, orgID,
	).Scan(&userEmail)
	if err == nil && strings.EqualFold(userEmail, fromAddress) {
		return true
	}

	// Check alias_users.can_send_as
	var allowed bool
	if err := db.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM alias_users au
			JOIN aliases a ON a.id = au.alias_id
			JOIN domains d ON d.id = a.domain_id
			WHERE au.user_id = $1 AND a.org_id = $2 AND a.address = $3 AND au.can_send_as = true
			AND d.status NOT IN ('disconnected', 'pending', 'deleted')
		)`,
		userID, orgID, fromAddress,
	).Scan(&allowed); err != nil {
		slog.Warn("canSendAs: alias check failed", "user_id", userID, "address", fromAddress, "error", err)
		return false
	}
	return allowed
}

// escapeLIKE escapes LIKE/ILIKE metacharacters (%, _, \) in a search string.
func escapeLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// resolveFromDisplay looks up a display name for an email address.
// Checks aliases table first, then users table, falls back to bare address.
// Returns "Display Name <address>" or just "address" if no name found.
func resolveFromDisplay(ctx context.Context, db *pgxpool.Pool, orgID, address string) string {
	var name string
	err := db.QueryRow(ctx,
		"SELECT name FROM aliases WHERE org_id = $1 AND address = $2 AND name != ''",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address)
	}

	err = db.QueryRow(ctx,
		"SELECT name FROM users WHERE org_id = $1 AND email = $2 AND name != '' AND status = 'active'",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address)
	}

	return address
}
