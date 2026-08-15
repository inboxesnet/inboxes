package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

// BounceHandler manages the org bounce block list.
type BounceHandler struct {
	Store store.Store
}

// List returns the bounce block list for the org.
func (h *BounceHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	bounces, err := h.Store.ListBounces(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list bounces")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"bounces":     bounces,
		"expiry_days": store.BounceExpiryDays,
	})
}

// Delete removes one address from the bounce block list.
func (h *BounceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bounce id")
		return
	}

	rows, err := h.Store.DeleteBounce(r.Context(), claims.OrgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove bounce")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "bounce not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
