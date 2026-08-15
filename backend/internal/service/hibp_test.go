package service

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sha1Upper(s string) string {
	sum := sha1.Sum([]byte(s))
	return strings.ToUpper(fmt.Sprintf("%x", sum))
}

func withHIBPServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := hibpBaseURL
	hibpBaseURL = srv.URL + "/range/"
	t.Cleanup(func() { hibpBaseURL = old })
}

func TestPasswordPwned_Found(t *testing.T) {
	password := "Password123"
	hash := sha1Upper(password)
	suffix := hash[5:]

	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+hash[:5]) {
			t.Errorf("expected prefix %s in path, got %s", hash[:5], r.URL.Path)
		}
		fmt.Fprintf(w, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:0\r\n%s:42\r\n", suffix)
	})

	pwned, err := PasswordPwned(context.Background(), password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pwned {
		t.Error("expected pwned=true")
	}
}

func TestPasswordPwned_NotFound(t *testing.T) {
	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:3\r\n")
	})

	pwned, err := PasswordPwned(context.Background(), "Password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pwned {
		t.Error("expected pwned=false")
	}
}

func TestPasswordPwned_PaddingZeroCountIgnored(t *testing.T) {
	password := "Password123"
	suffix := sha1Upper(password)[5:]

	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		// A padded entry for our suffix with count 0 must not count as a hit.
		fmt.Fprintf(w, "%s:0\r\n", suffix)
	})

	pwned, err := PasswordPwned(context.Background(), password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pwned {
		t.Error("padded zero-count entry must not report pwned")
	}
}

func TestPasswordPwned_Disabled(t *testing.T) {
	t.Setenv("HIBP_CHECK_DISABLED", "true")
	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be called when disabled")
	})

	pwned, err := PasswordPwned(context.Background(), "Password123")
	if err != nil || pwned {
		t.Errorf("expected (false, nil), got (%v, %v)", pwned, err)
	}
}

func TestCheckPwnedPassword_FailsOpen(t *testing.T) {
	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := CheckPwnedPassword(context.Background(), "Password123"); err != nil {
		t.Errorf("lookup failure must fail open, got: %v", err)
	}
}

func TestCheckPwnedPassword_Blocks(t *testing.T) {
	password := "Password123"
	suffix := sha1Upper(password)[5:]
	withHIBPServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s:9000\r\n", suffix)
	})

	if err := CheckPwnedPassword(context.Background(), password); err == nil {
		t.Error("expected an error for a breached password")
	}
}
