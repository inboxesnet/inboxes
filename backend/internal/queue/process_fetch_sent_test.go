package queue

import (
	"testing"
	"time"
)

// --- mapSentStatus tests ---

func TestMapSentStatus_Delivered(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus("delivered"); got != "delivered" {
		t.Errorf("mapSentStatus(\"delivered\") = %q, want %q", got, "delivered")
	}
}

func TestMapSentStatus_Bounced(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus("bounced"); got != "bounced" {
		t.Errorf("mapSentStatus(\"bounced\") = %q, want %q", got, "bounced")
	}
}

func TestMapSentStatus_Complained(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus("complained"); got != "complained" {
		t.Errorf("mapSentStatus(\"complained\") = %q, want %q", got, "complained")
	}
}

func TestMapSentStatus_Failed(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus("failed"); got != "failed" {
		t.Errorf("mapSentStatus(\"failed\") = %q, want %q", got, "failed")
	}
}

func TestMapSentStatus_Unknown(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus(""); got != "sent" {
		t.Errorf("mapSentStatus(\"\") = %q, want %q", got, "sent")
	}
}

func TestMapSentStatus_UnrecognizedEvent(t *testing.T) {
	t.Parallel()
	if got := mapSentStatus("some-unknown-event"); got != "sent" {
		t.Errorf("mapSentStatus(\"some-unknown-event\") = %q, want %q", got, "sent")
	}
}

// --- parseSentTime tests ---

func TestParseSentTime_RFC3339(t *testing.T) {
	t.Parallel()
	input := "2024-06-15T14:30:00Z"
	got := parseSentTime(input)
	want := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseSentTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseSentTime_RFC3339Nano(t *testing.T) {
	t.Parallel()
	input := "2024-06-15T14:30:00.123456789Z"
	got := parseSentTime(input)
	want := time.Date(2024, 6, 15, 14, 30, 0, 123456789, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseSentTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseSentTime_PgFormat(t *testing.T) {
	t.Parallel()
	input := "2024-01-15 10:30:00.123456+00"
	got := parseSentTime(input)
	want := time.Date(2024, 1, 15, 10, 30, 0, 123456000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseSentTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseSentTime_PgFormatWithColonOffset(t *testing.T) {
	t.Parallel()
	input := "2024-01-15 10:30:00.123456+00:00"
	got := parseSentTime(input)
	want := time.Date(2024, 1, 15, 10, 30, 0, 123456000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseSentTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseSentTime_PgFormatNoFractional(t *testing.T) {
	t.Parallel()
	input := "2024-01-15 10:30:00+00"
	got := parseSentTime(input)
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseSentTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseSentTime_Invalid(t *testing.T) {
	t.Parallel()
	before := time.Now()
	got := parseSentTime("not-a-date")
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("parseSentTime(invalid) = %v, want approximately time.Now() (between %v and %v)", got, before, after)
	}
}

func TestParseSentTime_Empty(t *testing.T) {
	t.Parallel()
	before := time.Now()
	got := parseSentTime("")
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("parseSentTime(\"\") = %v, want approximately time.Now()", got)
	}
}

// --- extractDisplayName tests ---

func TestExtractDisplayName_WithName(t *testing.T) {
	t.Parallel()
	got := extractDisplayName("CX Agency <hello@cx.agency>")
	if got != "CX Agency" {
		t.Errorf("extractDisplayName(\"CX Agency <hello@cx.agency>\") = %q, want %q", got, "CX Agency")
	}
}

func TestExtractDisplayName_NoName(t *testing.T) {
	t.Parallel()
	got := extractDisplayName("hello@cx.agency")
	if got != "hello" {
		t.Errorf("extractDisplayName(\"hello@cx.agency\") = %q, want %q", got, "hello")
	}
}

func TestExtractDisplayName_EmptyName(t *testing.T) {
	t.Parallel()
	got := extractDisplayName("<hello@cx.agency>")
	if got != "hello" {
		t.Errorf("extractDisplayName(\"<hello@cx.agency>\") = %q, want %q", got, "hello")
	}
}

func TestExtractDisplayName_NameWithExtraSpaces(t *testing.T) {
	t.Parallel()
	got := extractDisplayName("  John Doe   <john@example.com>")
	if got != "John Doe" {
		t.Errorf("extractDisplayName(\"  John Doe   <john@example.com>\") = %q, want %q", got, "John Doe")
	}
}

func TestExtractDisplayName_QuotedName(t *testing.T) {
	t.Parallel()
	// Even with quotes, the function just trims whitespace and takes the part before <
	got := extractDisplayName("\"Support Team\" <support@example.com>")
	if got != "\"Support Team\"" {
		t.Errorf("extractDisplayName with quoted name = %q, want %q", got, "\"Support Team\"")
	}
}
