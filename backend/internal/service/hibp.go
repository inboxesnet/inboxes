package service

import (
	"bufio"
	"context"
	"crypto/sha1"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// hibpBaseURL is a var so tests can point it at a local server.
var hibpBaseURL = "https://api.pwnedpasswords.com/range/"

var hibpClient = &http.Client{Timeout: 2 * time.Second}

// HIBPDisabled reports whether the Pwned Passwords check is turned off.
// Set HIBP_CHECK_DISABLED=true to disable (for example on air-gapped installs).
func HIBPDisabled() bool {
	return strings.EqualFold(os.Getenv("HIBP_CHECK_DISABLED"), "true")
}

// PasswordPwned checks a password against the Pwned Passwords range API.
// Only the first 5 characters of the SHA-1 hash leave the server
// (k-anonymity); the password itself is never sent.
//
// The check fails open: a network error or non-200 response returns
// (false, err) and callers must treat that as "not pwned" so an HIBP
// outage never blocks signups.
func PasswordPwned(ctx context.Context, password string) (bool, error) {
	if HIBPDisabled() {
		return false, nil
	}

	sum := sha1.Sum([]byte(password))
	hexHash := strings.ToUpper(fmt.Sprintf("%x", sum))
	prefix, suffix := hexHash[:5], hexHash[5:]

	req, err := http.NewRequestWithContext(ctx, "GET", hibpBaseURL+prefix, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Add-Padding", "true")

	resp, err := hibpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if strings.EqualFold(line[:colon], suffix) {
			// Padded responses list some suffixes with count 0; those are
			// filler, not real breach entries.
			count := strings.TrimSpace(line[colon+1:])
			if count != "0" {
				return true, nil
			}
		}
	}
	return false, scanner.Err()
}

// CheckPwnedPassword is the handler-facing wrapper. It returns an error
// only when the password is confirmed breached. Lookup failures log and
// pass (fail open).
func CheckPwnedPassword(ctx context.Context, password string) error {
	pwned, err := PasswordPwned(ctx, password)
	if err != nil {
		slog.Warn("hibp: lookup failed, skipping check", "error", err)
		return nil
	}
	if pwned {
		return fmt.Errorf("this password appears in known data breaches; choose a different one")
	}
	return nil
}
