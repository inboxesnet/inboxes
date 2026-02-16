package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CronHandler struct {
	DB *pgxpool.Pool
}

func (h *CronHandler) PurgeTrash(w http.ResponseWriter, r *http.Request) {
	// Delete threads that have been in trash for more than 30 days
	tag, err := h.DB.Exec(r.Context(),
		`DELETE FROM threads WHERE folder = 'trash' AND trash_expires_at IS NOT NULL AND trash_expires_at < now()`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to purge trash")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"purged": tag.RowsAffected(),
	})
}
