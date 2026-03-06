package util

import (
	"strings"
	"testing"
)

// ── TruncateRunes tests ──

func TestTruncateRunes_ShortString(t *testing.T) {
	t.Parallel()
	got := TruncateRunes("hello", 200)
	if got != "hello" {
		t.Errorf("TruncateRunes(short): got %q, want %q", got, "hello")
	}
}

func TestTruncateRunes_ExactLength(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("a", 200)
	got := TruncateRunes(input, 200)
	if len([]rune(got)) != 200 {
		t.Errorf("TruncateRunes(exact): got %d runes, want 200", len([]rune(got)))
	}
}

func TestTruncateRunes_TruncatesLong(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("b", 300)
	got := TruncateRunes(input, 200)
	if len([]rune(got)) != 200 {
		t.Errorf("TruncateRunes(long): got %d runes, want 200", len([]rune(got)))
	}
}

func TestTruncateRunes_Unicode(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("🎉", 250)
	got := TruncateRunes(input, 200)
	if len([]rune(got)) != 200 {
		t.Errorf("TruncateRunes(unicode): got %d runes, want 200", len([]rune(got)))
	}
}

func TestTruncateRunes_HTMLEntities(t *testing.T) {
	t.Parallel()
	got := TruncateRunes("Hello &amp; world", 200)
	want := "Hello & world"
	if got != want {
		t.Errorf("TruncateRunes(html): got %q, want %q", got, want)
	}
}

func TestTruncateRunes_EmptyString(t *testing.T) {
	t.Parallel()
	got := TruncateRunes("", 200)
	if got != "" {
		t.Errorf("TruncateRunes(empty): got %q, want %q", got, "")
	}
}

func TestTruncateRunes_SmallLimit(t *testing.T) {
	t.Parallel()
	got := TruncateRunes("abcdefghij", 5)
	if got != "abcde" {
		t.Errorf("TruncateRunes(small limit): got %q, want %q", got, "abcde")
	}
}

func TestTruncateRunes_UnicodeSmallLimit(t *testing.T) {
	t.Parallel()
	got := TruncateRunes("日本語文字", 3)
	if got != "日本語" {
		t.Errorf("TruncateRunes(unicode small): got %q, want %q", got, "日本語")
	}
}

func TestTruncateRunes_HTMLUnescapeThenTruncate(t *testing.T) {
	t.Parallel()
	// "&amp;" is 5 chars but unescapes to "&" (1 rune)
	got := TruncateRunes("&amp;hello", 4)
	if got != "&hel" {
		t.Errorf("TruncateRunes(html+truncate): got %q, want %q", got, "&hel")
	}
}

// ── CleanSubjectLine tests ──

func TestCleanSubjectLine_NoPrefix(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Meeting tomorrow")
	if got != "Meeting tomorrow" {
		t.Errorf("CleanSubjectLine(no prefix): got %q, want %q", got, "Meeting tomorrow")
	}
}

func TestCleanSubjectLine_SingleRe(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Re: Meeting tomorrow")
	if got != "Meeting tomorrow" {
		t.Errorf("CleanSubjectLine(Re:): got %q, want %q", got, "Meeting tomorrow")
	}
}

func TestCleanSubjectLine_SingleFwd(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Fwd: Meeting tomorrow")
	if got != "Meeting tomorrow" {
		t.Errorf("CleanSubjectLine(Fwd:): got %q, want %q", got, "Meeting tomorrow")
	}
}

func TestCleanSubjectLine_NestedReFwd(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Re: Fwd: Re: Meeting")
	if got != "Meeting" {
		t.Errorf("CleanSubjectLine(nested): got %q, want %q", got, "Meeting")
	}
}

func TestCleanSubjectLine_CaseInsensitive(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("RE: FWD: re: Topic")
	if got != "Topic" {
		t.Errorf("CleanSubjectLine(case): got %q, want %q", got, "Topic")
	}
}

func TestCleanSubjectLine_CollapseSpaces(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Re:  Meeting   tomorrow ")
	if got != "Meeting tomorrow" {
		t.Errorf("CleanSubjectLine(spaces): got %q, want %q", got, "Meeting tomorrow")
	}
}

func TestCleanSubjectLine_EmptyAfterStrip(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("Re: ")
	if got != "" {
		t.Errorf("CleanSubjectLine(empty after strip): got %q, want %q", got, "")
	}
}

func TestCleanSubjectLine_OnlySpaces(t *testing.T) {
	t.Parallel()
	got := CleanSubjectLine("   ")
	if got != "" {
		t.Errorf("CleanSubjectLine(only spaces): got %q, want %q", got, "")
	}
}
