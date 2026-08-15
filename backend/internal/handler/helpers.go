package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"github.com/inboxes/backend/internal/service"
)

// writeResendError logs a Resend API failure and surfaces the real Resend
// message to the client where that is safe. The fallback text is used when
// the error carries no usable message.
func writeResendError(w http.ResponseWriter, err error, fallback string) {
	var re *service.ResendError
	if errors.As(err, &re) {
		slog.Error("resend: request failed", "status", re.StatusCode, "body", re.Body)
		switch {
		case re.StatusCode == 401 || re.StatusCode == 403:
			writeError(w, http.StatusBadGateway, "Resend rejected the API key. Check the key in Settings.")
		case re.StatusCode == 429:
			writeError(w, http.StatusTooManyRequests, "Resend rate limit reached. Try again in a moment.")
		case re.StatusCode >= 400 && re.StatusCode < 500:
			msg := re.Message()
			if msg == "" {
				msg = fallback
			}
			writeError(w, http.StatusBadRequest, msg)
		default:
			writeError(w, http.StatusBadGateway, fallback)
		}
		return
	}
	slog.Error("resend: request failed", "error", err)
	writeError(w, http.StatusBadGateway, fallback)
}

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

// setIfNotNil sets m[key] = *v if v is non-nil.
func setIfNotNil[T any](m map[string]any, key string, v *T) {
	if v != nil {
		m[key] = *v
	}
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

// escapeLIKE escapes LIKE/ILIKE metacharacters (%, _, \) in a search string.
func escapeLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

