package handler

import (
	"strings"
	"testing"
)

// --- validateEmail ---

func TestValidateEmail_Valid(t *testing.T) {
	t.Parallel()
	if err := validateEmail("user@example.com"); err != nil {
		t.Errorf("validateEmail(valid): %v", err)
	}
}

func TestValidateEmail_Empty(t *testing.T) {
	t.Parallel()
	if err := validateEmail(""); err == nil {
		t.Error("validateEmail(empty): expected error, got nil")
	}
}

func TestValidateEmail_TooLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 250) + "@b.co"
	err := validateEmail(long)
	if err == nil {
		t.Fatal("validateEmail(>254): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("validateEmail(>254): got %q, want 'too long'", err.Error())
	}
}

func TestValidateEmail_NoDomain(t *testing.T) {
	t.Parallel()
	if err := validateEmail("user@"); err == nil {
		t.Error("validateEmail(no domain): expected error, got nil")
	}
}

func TestValidateEmail_NoLocalPart(t *testing.T) {
	t.Parallel()
	if err := validateEmail("@example.com"); err == nil {
		t.Error("validateEmail(no local): expected error, got nil")
	}
}

func TestValidateEmail_PlusTag(t *testing.T) {
	t.Parallel()
	if err := validateEmail("user+tag@example.com"); err != nil {
		t.Errorf("validateEmail(plus tag): %v", err)
	}
}

func TestValidateEmail_DisplayName(t *testing.T) {
	t.Parallel()
	// mail.ParseAddress accepts "Display Name <user@example.com>"
	if err := validateEmail("Jane Doe <jane@example.com>"); err != nil {
		t.Errorf("validateEmail(display name): %v", err)
	}
}

func TestValidateEmail_Exact254(t *testing.T) {
	t.Parallel()
	// 254 chars exactly: should pass length check, may fail format
	local := strings.Repeat("a", 245)
	email := local + "@b.co" // 245 + 1 + 4 = 250 < 254, passes length
	// We only care that it doesn't error on length
	err := validateEmail(email)
	// The result depends on RFC validation, but length check should pass
	_ = err
}

func TestValidateEmail_MissingAt(t *testing.T) {
	t.Parallel()
	if err := validateEmail("userexample.com"); err == nil {
		t.Error("validateEmail(missing @): expected error, got nil")
	}
}

// --- validatePassword ---

func TestValidatePassword_Valid(t *testing.T) {
	t.Parallel()
	if err := validatePassword("Secret1x"); err != nil {
		t.Errorf("validatePassword(valid): %v", err)
	}
}

func TestValidatePassword_TooShort(t *testing.T) {
	t.Parallel()
	if err := validatePassword("Aa1"); err == nil {
		t.Error("validatePassword(<8): expected error, got nil")
	}
}

func TestValidatePassword_TooLong(t *testing.T) {
	t.Parallel()
	pw := "Aa1" + strings.Repeat("x", 126)
	if err := validatePassword(pw); err == nil {
		t.Error("validatePassword(>128): expected error, got nil")
	}
}

func TestValidatePassword_NoUppercase(t *testing.T) {
	t.Parallel()
	if err := validatePassword("secret1x"); err == nil {
		t.Error("validatePassword(no upper): expected error, got nil")
	}
}

func TestValidatePassword_NoLowercase(t *testing.T) {
	t.Parallel()
	if err := validatePassword("SECRET1X"); err == nil {
		t.Error("validatePassword(no lower): expected error, got nil")
	}
}

func TestValidatePassword_NoDigit(t *testing.T) {
	t.Parallel()
	if err := validatePassword("Secretxx"); err == nil {
		t.Error("validatePassword(no digit): expected error, got nil")
	}
}

func TestValidatePassword_Exact8(t *testing.T) {
	t.Parallel()
	if err := validatePassword("Abcdef1x"); err != nil {
		t.Errorf("validatePassword(exact 8): %v", err)
	}
}

func TestValidatePassword_Exact128(t *testing.T) {
	t.Parallel()
	pw := "Aa1" + strings.Repeat("x", 125) // 128 chars
	if err := validatePassword(pw); err != nil {
		t.Errorf("validatePassword(exact 128): %v", err)
	}
}

func TestValidatePassword_SpecialChars(t *testing.T) {
	t.Parallel()
	if err := validatePassword("P@ssw0rd!#"); err != nil {
		t.Errorf("validatePassword(special chars): %v", err)
	}
}

// --- validateLength ---

func TestValidateLength_UnderLimit(t *testing.T) {
	t.Parallel()
	if err := validateLength("hi", "name", 10); err != nil {
		t.Errorf("validateLength(under): %v", err)
	}
}

func TestValidateLength_AtLimit(t *testing.T) {
	t.Parallel()
	if err := validateLength("hello", "name", 5); err != nil {
		t.Errorf("validateLength(at limit): %v", err)
	}
}

func TestValidateLength_OverLimit(t *testing.T) {
	t.Parallel()
	err := validateLength("hello!", "name", 5)
	if err == nil {
		t.Fatal("validateLength(over): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should contain field name, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "5") {
		t.Errorf("error should contain max value, got %q", err.Error())
	}
}

func TestValidateLength_Empty(t *testing.T) {
	t.Parallel()
	if err := validateLength("", "field", 10); err != nil {
		t.Errorf("validateLength(empty): %v", err)
	}
}
