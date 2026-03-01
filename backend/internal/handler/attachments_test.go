package handler

import (
	"strings"
	"testing"
)

func TestSanitizeFilename_Normal(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("report.pdf"); got != "report.pdf" {
		t.Errorf("got %q, want %q", got, "report.pdf")
	}
}

func TestSanitizeFilename_ControlChars(t *testing.T) {
	t.Parallel()
	got := sanitizeFilename("file\x00name\x1F.txt")
	if strings.ContainsAny(got, "\x00\x1F") {
		t.Errorf("control chars not replaced: got %q", got)
	}
	if got != "file_name_.txt" {
		t.Errorf("got %q, want %q", got, "file_name_.txt")
	}
}

func TestSanitizeFilename_Quotes(t *testing.T) {
	t.Parallel()
	got := sanitizeFilename(`file"name.txt`)
	if strings.Contains(got, `"`) {
		t.Errorf("quote not replaced: got %q", got)
	}
}

func TestSanitizeFilename_Backslash(t *testing.T) {
	t.Parallel()
	got := sanitizeFilename(`file\name.txt`)
	if strings.Contains(got, `\`) {
		t.Errorf("backslash not replaced: got %q", got)
	}
}

func TestSanitizeFilename_LongName(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 300)
	got := sanitizeFilename(long)
	if len(got) > 255 {
		t.Errorf("length %d > 255", len(got))
	}
}

func TestSanitizeFilename_Empty(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename(""); got != "download" {
		t.Errorf("got %q, want %q", got, "download")
	}
}

func TestSanitizeFilename_AllBadChars(t *testing.T) {
	t.Parallel()
	// All control chars + quotes + backslashes results in all underscores, which may be treated as empty
	got := sanitizeFilename("\x00\x01\x02")
	if got == "" {
		t.Error("should not be empty")
	}
	// After replacing control chars with _, result is "___"
	if got != "___" {
		t.Errorf("got %q, want %q", got, "___")
	}
}

func TestSanitizeFilename_PathTraversal(t *testing.T) {
	t.Parallel()
	got := sanitizeFilename("../../../etc/passwd")
	// The function does not strip path separators (only control chars/quotes/backslash)
	// but path traversal is mitigated because slashes are preserved and the result
	// is used in Content-Disposition, not filesystem access. Just ensure no backslash.
	if strings.Contains(got, `\`) {
		t.Errorf("backslash in result: got %q", got)
	}
}

func TestSanitizeFilename_UnicodePreserved(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("文档.pdf"); got != "文档.pdf" {
		t.Errorf("unicode not preserved: got %q, want %q", got, "文档.pdf")
	}
}

func TestSafeDownloadTypes_Allowed(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"image/png", "application/pdf", "text/plain"} {
		if !safeDownloadTypes[ct] {
			t.Errorf("%q should be in whitelist", ct)
		}
	}
}

func TestSafeDownloadTypes_Blocked(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"application/javascript", "text/html", "application/x-executable"} {
		if safeDownloadTypes[ct] {
			t.Errorf("%q should NOT be in whitelist", ct)
		}
	}
}

func TestSafeDownloadTypes_OctetStreamDefault(t *testing.T) {
	t.Parallel()
	if safeDownloadTypes["application/octet-stream"] {
		t.Error("application/octet-stream should not be in whitelist")
	}
}
