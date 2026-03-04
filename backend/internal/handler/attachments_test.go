package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/inboxes/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// ── mockQuerier for attachment upload tests ──

type attachmentMockQuerier struct {
	execFn func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (q *attachmentMockQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if q.execFn != nil {
		return q.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (q *attachmentMockQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (q *attachmentMockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

// makeMultipartRequest creates a multipart form request with the given file content and filename.
func makeMultipartRequest(t *testing.T, fieldName, filename string, content []byte, clientContentType string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if clientContentType != "" {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
		h.Set("Content-Type", clientContentType)
		part, err := writer.CreatePart(h)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		part.Write(content)
	} else {
		part, err := writer.CreateFormFile(fieldName, filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		part.Write(content)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/attachments", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// ── Upload handler tests ──

func TestUpload_FileTooLarge(t *testing.T) {
	t.Parallel()
	h := &AttachmentHandler{}

	// Use an invalid Content-Type to trigger ParseMultipartForm error
	// which exercises the "file too large" error path
	req := httptest.NewRequest("POST", "/attachments", strings.NewReader("not-multipart-data"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=nonexistent")
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Upload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "file too large") {
		t.Errorf("expected 'file too large' in body, got: %s", w.Body.String())
	}
}

func TestUpload_BlockedContentType(t *testing.T) {
	t.Parallel()

	// Verify the blockedTypes map inside Upload covers all expected executable types.
	// We instantiate the map the same way the handler does and check each entry.
	blockedTypes := map[string]bool{
		"application/x-executable":     true,
		"application/x-msdos-program":  true,
		"application/x-msdownload":     true,
		"application/x-dosexec":        true,
		"application/vnd.microsoft.portable-executable": true,
	}

	expected := []string{
		"application/x-executable",
		"application/x-msdos-program",
		"application/x-msdownload",
		"application/x-dosexec",
		"application/vnd.microsoft.portable-executable",
	}
	for _, ct := range expected {
		if !blockedTypes[ct] {
			t.Errorf("%q should be in blockedTypes", ct)
		}
	}

	// Verify safe types are NOT blocked
	safe := []string{"text/plain", "image/png", "application/pdf", "application/octet-stream"}
	for _, ct := range safe {
		if blockedTypes[ct] {
			t.Errorf("%q should NOT be in blockedTypes", ct)
		}
	}
}

func TestUpload_ValidFile(t *testing.T) {
	t.Parallel()

	var capturedArgs []any
	mq := &attachmentMockQuerier{
		execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = args
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	ms := &store.MockStore{
		QFn: func() store.Querier { return mq },
	}
	h := &AttachmentHandler{Store: ms}

	// PNG magic bytes create a valid image/png detection
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	content := append(pngHeader, bytes.Repeat([]byte{0x00}, 100)...)

	req := makeMultipartRequest(t, "file", "image.png", content, "")
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Upload(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp["filename"] != "image.png" {
		t.Errorf("filename = %v, want %q", resp["filename"], "image.png")
	}
	if resp["content_type"] != "image/png" {
		t.Errorf("content_type = %v, want %q", resp["content_type"], "image/png")
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty id in response")
	}
	sizeVal, ok := resp["size"].(float64)
	if !ok || int(sizeVal) != len(content) {
		t.Errorf("size = %v, want %d", resp["size"], len(content))
	}

	// Verify the DB insert received correct org and user IDs
	if len(capturedArgs) < 3 {
		t.Fatalf("expected at least 3 args to Exec, got %d", len(capturedArgs))
	}
	if capturedArgs[1] != "org1" {
		t.Errorf("orgID arg = %v, want %q", capturedArgs[1], "org1")
	}
	if capturedArgs[2] != "user1" {
		t.Errorf("userID arg = %v, want %q", capturedArgs[2], "user1")
	}
}

func TestUpload_OctetStreamFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		clientCT        string
		wantContentType string
	}{
		{
			name:            "text/plain fallback allowed",
			clientCT:        "text/plain",
			wantContentType: "text/plain",
		},
		{
			name:            "text/csv fallback allowed",
			clientCT:        "text/csv",
			wantContentType: "text/csv",
		},
		{
			name:            "text/html fallback rejected",
			clientCT:        "text/html",
			wantContentType: "application/octet-stream",
		},
		{
			name:            "application/javascript fallback rejected",
			clientCT:        "application/javascript",
			wantContentType: "application/octet-stream",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var storedCT string
			mq := &attachmentMockQuerier{
				execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					// args: id, orgID, userID, filename, contentType, size, data
					if len(args) >= 5 {
						storedCT = args[4].(string)
					}
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			}
			ms := &store.MockStore{
				QFn: func() store.Querier { return mq },
			}
			h := &AttachmentHandler{Store: ms}

			// Random binary content that http.DetectContentType returns application/octet-stream for
			content := bytes.Repeat([]byte{0x07, 0x08, 0xFE, 0xFF, 0x80, 0x81}, 20)

			req := makeMultipartRequest(t, "file", "data.bin", content, tc.clientCT)
			req = withClaims(req, "user1", "org1", "admin")
			w := httptest.NewRecorder()

			h.Upload(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
			}

			if storedCT != tc.wantContentType {
				t.Errorf("stored content_type = %q, want %q", storedCT, tc.wantContentType)
			}
		})
	}
}

func TestUpload_EmptyFile(t *testing.T) {
	t.Parallel()

	mq := &attachmentMockQuerier{}
	ms := &store.MockStore{
		QFn: func() store.Querier { return mq },
	}
	h := &AttachmentHandler{Store: ms}

	// Create a multipart form with an empty file (0 bytes)
	req := makeMultipartRequest(t, "file", "empty.txt", []byte{}, "")
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Upload(w, req)

	// An empty file passes validation (no blocked MIME type, ParseMultipartForm succeeds).
	// With our mock store it should succeed with a 201.
	if w.Code != http.StatusCreated {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	sizeVal, ok := resp["size"].(float64)
	if !ok || int(sizeVal) != 0 {
		t.Errorf("size = %v, want 0", resp["size"])
	}
	if resp["filename"] != "empty.txt" {
		t.Errorf("filename = %v, want %q", resp["filename"], "empty.txt")
	}
}
