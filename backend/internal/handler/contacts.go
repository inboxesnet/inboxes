package handler

import (
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ContactHandler struct {
	DB *pgxpool.Pool
}

func (h *ContactHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	if query == "" {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	// Search across from_address and to_addresses in user's org emails
	rows, err := h.DB.Query(r.Context(),
		`SELECT address AS email, COUNT(*) as count
		 FROM (
		   SELECT from_address as address FROM emails WHERE org_id = $1
		   UNION ALL
		   SELECT jsonb_array_elements_text(to_addresses) as address FROM emails WHERE org_id = $1
		 ) sub
		 WHERE address ILIKE $2
		 GROUP BY address
		 ORDER BY count DESC
		 LIMIT 10`,
		claims.OrgID, "%"+escapeLIKE(query)+"%")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search contacts")
		return
	}
	suggestions, err := scanMaps(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search contacts")
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

