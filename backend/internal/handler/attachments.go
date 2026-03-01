package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AttachmentHandler struct {
	DB *pgxpool.Pool
}

// Upload handles multipart file upload and stores file content in the database.
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	// Detect actual MIME type from file content to block dangerous uploads
	detected := http.DetectContentType(data)
	blockedTypes := map[string]bool{
		"application/x-executable":     true,
		"application/x-msdos-program":  true,
		"application/x-msdownload":     true,
		"application/x-dosexec":        true,
		"application/vnd.microsoft.portable-executable": true,
	}
	if blockedTypes[detected] {
		writeError(w, http.StatusBadRequest, "file type not allowed")
		return
	}

	id := uuid.New().String()
	// Always use the detected content type — never trust client-provided Content-Type
	contentType := detected
	if contentType == "application/octet-stream" {
		// Fall back to client hint only for safe types
		clientCT := header.Header.Get("Content-Type")
		if clientCT == "text/plain" || clientCT == "text/csv" {
			contentType = clientCT
		}
	}

	// Store in attachments table
	_, err = h.DB.Exec(r.Context(),
		`INSERT INTO attachments (id, org_id, user_id, filename, content_type, size, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		id, claims.OrgID, claims.UserID, header.Filename, contentType, len(data), data,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store attachment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           id,
		"filename":     header.Filename,
		"content_type": contentType,
		"size":         len(data),
	})
}

// safeDownloadTypes is a whitelist of MIME types safe to serve with their real Content-Type.
// Everything else is forced to application/octet-stream.
var safeDownloadTypes = map[string]bool{
	"image/png":             true,
	"image/jpeg":            true,
	"image/gif":             true,
	"image/webp":            true,
	"application/pdf":       true,
	"text/plain":            true,
	"text/csv":              true,
	"application/zip":       true,
	"application/gzip":      true,
	"application/x-tar":     true,
}

// sanitizeFilename removes control characters and quotes from a filename
// for safe inclusion in Content-Disposition headers.
func sanitizeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if len(clean) > 255 {
		clean = clean[:255]
	}
	if clean == "" {
		clean = "download"
	}
	return clean
}

// Meta returns attachment metadata without the binary data.
func (h *AttachmentHandler) Meta(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	attachmentID := chi.URLParam(r, "id")

	var filename, contentType string
	var size int
	err := h.DB.QueryRow(r.Context(),
		`SELECT filename, content_type, size FROM attachments WHERE id = $1 AND org_id = $2`,
		attachmentID, claims.OrgID,
	).Scan(&filename, &contentType, &size)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           attachmentID,
		"filename":     filename,
		"content_type": contentType,
		"size":         size,
	})
}

func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	attachmentID := chi.URLParam(r, "id")

	var filename, contentType string
	var size int
	var data []byte
	err := h.DB.QueryRow(r.Context(),
		`SELECT filename, content_type, size, data FROM attachments WHERE id = $1 AND org_id = $2`,
		attachmentID, claims.OrgID,
	).Scan(&filename, &contentType, &size, &data)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	// Force safe content type for downloads
	if !safeDownloadTypes[contentType] {
		contentType = "application/octet-stream"
	}

	// Sanitize filename for Content-Disposition header
	safe := sanitizeFilename(filename)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}
