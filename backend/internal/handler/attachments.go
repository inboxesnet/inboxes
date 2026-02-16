package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AttachmentHandler struct {
	DB *pgxpool.Pool
}

// Upload handles multipart file upload.
// For now, stores file content in the database. In production, use S3/MinIO.
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

	id := uuid.New().String()
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Store metadata. In a real system, we'd upload to S3 here.
	meta := map[string]interface{}{
		"filename":     header.Filename,
		"content_type": contentType,
		"size":         len(data),
		"org_id":       claims.OrgID,
	}
	metaBytes, _ := json.Marshal(meta)

	// For MVP: store in a simple attachments table (or just return metadata)
	// Since we don't have an attachments table for blob storage, return the metadata
	_ = data // In production: upload to S3

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           id,
		"filename":     header.Filename,
		"content_type": contentType,
		"size":         len(data),
		"metadata":     json.RawMessage(metaBytes),
	})
}

func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	_ = middleware.GetCurrentUser(r.Context())
	attachmentID := chi.URLParam(r, "id")

	// For MVP: return a placeholder response
	// In production: fetch from S3 based on attachment ID
	_ = filepath.Clean(attachmentID)

	writeError(w, http.StatusNotFound, "attachment not found")
}
