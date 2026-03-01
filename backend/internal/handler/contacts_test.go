package handler

import "testing"

func TestEscapeLIKE_NoSpecialChars(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("hello"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEscapeLIKE_Percent(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("100%"); got != `100\%` {
		t.Errorf("got %q, want %q", got, `100\%`)
	}
}

func TestEscapeLIKE_Underscore(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("first_name"); got != `first\_name` {
		t.Errorf("got %q, want %q", got, `first\_name`)
	}
}

func TestEscapeLIKE_Backslash(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE(`path\to`); got != `path\\to` {
		t.Errorf("got %q, want %q", got, `path\\to`)
	}
}

func TestEscapeLIKE_AllMetachars(t *testing.T) {
	t.Parallel()
	got := escapeLIKE(`%_\`)
	want := `\%\_\\`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeLIKE_Empty(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE(""); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestEscapeLIKE_SQLInjection(t *testing.T) {
	t.Parallel()
	input := `'; DROP TABLE users; --`
	got := escapeLIKE(input)
	// Only LIKE metacharacters should be escaped; SQL injection is handled by parameterized queries
	if got != input {
		t.Errorf("got %q, want %q (no LIKE metachar to escape)", got, input)
	}
}
