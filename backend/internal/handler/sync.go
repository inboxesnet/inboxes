package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
	"github.com/redis/go-redis/v9"
)

const syncJobsQueue = "sync:jobs"

type SyncHandler struct {
	Store store.Store
	RDB   *redis.Client
}

// StartSync creates a new sync job or returns an existing active one.
// POST /api/sync
func (h *SyncHandler) StartSync(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()

	// Check for an existing pending/running job for this org
	var existingID, existingStatus, existingPhase string
	var imported, total, sentCount, receivedCount, threadCount, addressCount int
	var createdAt time.Time
	err := h.Store.Q().QueryRow(ctx,
		`SELECT id, status, phase, imported, total, sent_count, received_count, thread_count, address_count, created_at
		 FROM sync_jobs WHERE org_id = $1 AND status IN ('pending', 'running')
		 ORDER BY created_at DESC LIMIT 1`, claims.OrgID,
	).Scan(&existingID, &existingStatus, &existingPhase, &imported, &total,
		&sentCount, &receivedCount, &threadCount, &addressCount, &createdAt)

	if err == nil {
		// Active job exists — return it
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":             existingID,
			"status":         existingStatus,
			"phase":          existingPhase,
			"imported":       imported,
			"total":          total,
			"sent_count":     sentCount,
			"received_count": receivedCount,
			"thread_count":   threadCount,
			"address_count":  addressCount,
			"already_active": true,
			"created_at":     createdAt,
		})
		return
	}

	// Create new job
	jobID, err := h.Store.CreateSyncJob(ctx, claims.OrgID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create sync job")
		return
	}

	// Push to Redis queue
	if err := h.RDB.LPush(ctx, syncJobsQueue, jobID).Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue sync job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             jobID,
		"status":         "pending",
		"phase":          "pending",
		"imported":       0,
		"total":          0,
		"already_active": false,
	})
}

// GetSync returns the status of a sync job.
// GET /api/sync/{id}
func (h *SyncHandler) GetSync(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	jobID := chi.URLParam(r, "id")

	resp, err := h.Store.GetSyncJob(r.Context(), jobID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "sync job not found")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
