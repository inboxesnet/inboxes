package service

import (
	"testing"
)

func TestExtractEmail_NameAndAngleBrackets(t *testing.T) {
	t.Parallel()
	got := ExtractEmail("John Doe <john@example.com>")
	want := "john@example.com"
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", "John Doe <john@example.com>", got, want)
	}
}

func TestExtractEmail_BareEmail(t *testing.T) {
	t.Parallel()
	got := ExtractEmail("john@example.com")
	want := "john@example.com"
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", "john@example.com", got, want)
	}
}

func TestExtractEmail_OnlyAngleBrackets(t *testing.T) {
	t.Parallel()
	got := ExtractEmail("<john@example.com>")
	want := "john@example.com"
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", "<john@example.com>", got, want)
	}
}

func TestExtractEmail_EmptyString(t *testing.T) {
	t.Parallel()
	got := ExtractEmail("")
	want := ""
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", "", got, want)
	}
}

func TestExtractEmail_MalformedNoBracketClose(t *testing.T) {
	t.Parallel()
	input := "John <john@example.com"
	got := ExtractEmail(input)
	// No closing >, falls through to TrimSpace of original
	want := input
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", input, got, want)
	}
}

func TestExtractEmail_SpacesAroundEmail(t *testing.T) {
	t.Parallel()
	got := ExtractEmail("John < john@example.com >")
	want := "john@example.com"
	if got != want {
		t.Errorf("ExtractEmail(%q): got %q, want %q", "John < john@example.com >", got, want)
	}
}
