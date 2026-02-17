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
		`SELECT DISTINCT address, COUNT(*) as cnt
		 FROM (
		   SELECT from_address as address FROM emails WHERE org_id = $1
		   UNION ALL
		   SELECT jsonb_array_elements_text(to_addresses) as address FROM emails WHERE org_id = $1
		 ) sub
		 WHERE address ILIKE $2
		 GROUP BY address
		 ORDER BY cnt DESC
		 LIMIT 10`,
		claims.OrgID, "%"+query+"%")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search contacts")
		return
	}
	defer rows.Close()

	var suggestions []map[string]interface{}
	for rows.Next() {
		var address string
		var count int
		if rows.Scan(&address, &count) == nil {
			suggestions = append(suggestions, map[string]interface{}{
				"email": address,
				"count": count,
			})
		}
	}

	if suggestions == nil {
		suggestions = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *ContactHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	ctx := r.Context()

	// Count emails sent by and to this address
	var sentCount, receivedCount int
	h.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM emails WHERE org_id = $1 AND from_address = $2`,
		claims.OrgID, email).Scan(&sentCount)
	h.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM emails WHERE org_id = $1 AND to_addresses @> $2::jsonb`,
		claims.OrgID, `["`+email+`"]`).Scan(&receivedCount)

	// Try to find a name from aliases or users
	var name *string
	h.DB.QueryRow(ctx,
		`SELECT u.name FROM users u WHERE u.org_id = $1 AND u.email = $2 AND u.status != 'disabled'`,
		claims.OrgID, email).Scan(&name)
	if name == nil {
		h.DB.QueryRow(ctx,
			`SELECT a.name FROM aliases a JOIN domains d ON a.domain_id = d.id
			 WHERE d.org_id = $1 AND a.address = $2 AND a.name != ''`,
			claims.OrgID, email).Scan(&name)
	}

	result := map[string]interface{}{
		"email":          email,
		"sent_count":     sentCount,
		"received_count": receivedCount,
		"total_count":    sentCount + receivedCount,
	}
	if name != nil {
		result["name"] = *name
	}
	writeJSON(w, http.StatusOK, result)
}
