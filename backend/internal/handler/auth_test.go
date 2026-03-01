package handler

import "testing"

func TestGenerateVerificationCode_Length(t *testing.T) {
	t.Parallel()
	code := generateVerificationCode()
	if len(code) != 6 {
		t.Errorf("length: got %d, want 6", len(code))
	}
}

func TestGenerateVerificationCode_AllDigits(t *testing.T) {
	t.Parallel()
	code := generateVerificationCode()
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			t.Errorf("non-digit %q in code %q", ch, code)
		}
	}
}

func TestGenerateVerificationCode_LeadingZeroPreserved(t *testing.T) {
	t.Parallel()
	// Generate many codes and check that short numbers are padded
	for i := 0; i < 100; i++ {
		code := generateVerificationCode()
		if len(code) != 6 {
			t.Fatalf("code %q length %d != 6", code, len(code))
		}
	}
}

func TestGenerateVerificationCode_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		seen[generateVerificationCode()] = true
	}
	if len(seen) < 50 {
		t.Errorf("only %d unique codes out of 100", len(seen))
	}
}

func TestNormalizeEmail_Lowercase(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("USER@EXAMPLE.COM"); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_TrimWhitespace(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("  user@example.com  "); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_AlreadyNormalized(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("user@example.com"); got != "user@example.com" {
		t.Errorf("got %q, want %q", got, "user@example.com")
	}
}

func TestNormalizeEmail_Empty(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail(""); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestNormalizeEmail_MixedCase(t *testing.T) {
	t.Parallel()
	if got := normalizeEmail("Alice.Smith@Gmail.COM"); got != "alice.smith@gmail.com" {
		t.Errorf("got %q, want %q", got, "alice.smith@gmail.com")
	}
}
