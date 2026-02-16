package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AliasHandler struct {
	DB *pgxpool.Pool
}

func (h *AliasHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	domainID := r.URL.Query().Get("domain_id")

	query := `SELECT a.id, a.address, a.name, a.domain_id, a.created_at
	          FROM aliases a WHERE a.org_id = $1`
	args := []interface{}{claims.OrgID}

	if domainID != "" {
		query += " AND a.domain_id = $2"
		args = append(args, domainID)
	}
	query += " ORDER BY a.address"

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list aliases")
		return
	}
	defer rows.Close()

	var aliases []map[string]interface{}
	for rows.Next() {
		var id, address, name, dID string
		var createdAt interface{}
		if rows.Scan(&id, &address, &name, &dID, &createdAt) == nil {
			aliases = append(aliases, map[string]interface{}{
				"id": id, "address": address, "name": name,
				"domain_id": dID, "created_at": createdAt,
			})
		}
	}

	if aliases == nil {
		aliases = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (h *AliasHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	var req struct {
		Address  string `json:"address"`
		Name     string `json:"name"`
		DomainID string `json:"domain_id"`
	}
	if err := readJSON(r, &req); err != nil || req.Address == "" || req.DomainID == "" {
		writeError(w, http.StatusBadRequest, "address and domain_id are required")
		return
	}

	var aliasID string
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO aliases (org_id, domain_id, address, name)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		claims.OrgID, req.DomainID, req.Address, req.Name,
	).Scan(&aliasID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create alias")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id": aliasID, "address": req.Address, "name": req.Name,
	})
}

func (h *AliasHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	aliasID := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(r.Context(),
		`DELETE FROM aliases WHERE id = $1 AND org_id = $2`,
		aliasID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	aliasID := chi.URLParam(r, "id")
	var req struct {
		UserID    string `json:"user_id"`
		CanSendAs bool   `json:"can_send_as"`
	}
	if err := readJSON(r, &req); err != nil || req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	// Verify alias belongs to org
	var count int
	h.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM aliases WHERE id = $1 AND org_id = $2`,
		aliasID, claims.OrgID).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	_, err := h.DB.Exec(r.Context(),
		`INSERT INTO alias_users (alias_id, user_id, can_send_as)
		 VALUES ($1, $2, $3) ON CONFLICT (alias_id, user_id) DO UPDATE SET can_send_as = $3`,
		aliasID, req.UserID, req.CanSendAs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user to alias")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	aliasID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")

	// Verify alias belongs to org
	var count int
	h.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM aliases WHERE id = $1 AND org_id = $2`,
		aliasID, claims.OrgID).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	h.DB.Exec(r.Context(),
		`DELETE FROM alias_users WHERE alias_id = $1 AND user_id = $2`,
		aliasID, userID)

	w.WriteHeader(http.StatusNoContent)
}
