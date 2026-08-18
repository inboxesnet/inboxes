package mcp

import (
	"net"
	"testing"
)

// TestUserRateLimitIP checks the synthetic per-user IP is valid and distinct.
// The router keys per-IP rate limits on this value, so two users must map to
// two different, parseable addresses.
func TestUserRateLimitIP(t *testing.T) {
	a := userRateLimitIP("11111111-1111-1111-1111-111111111111")
	b := userRateLimitIP("22222222-2222-2222-2222-222222222222")

	if net.ParseIP(a) == nil {
		t.Fatalf("user A IP is not valid: %q", a)
	}
	if net.ParseIP(b) == nil {
		t.Fatalf("user B IP is not valid: %q", b)
	}
	if a == b {
		t.Fatalf("two users share one IP bucket: %q", a)
	}

	// The mapping is stable across calls.
	if a != userRateLimitIP("11111111-1111-1111-1111-111111111111") {
		t.Fatal("user A IP is not stable")
	}
}
