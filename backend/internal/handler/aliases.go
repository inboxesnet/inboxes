package handler

import (
	"log/slog"
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
	          FROM aliases a WHERE a.org_id = $1 AND a.deleted_at IS NULL`
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

	// Fetch alias_users with user info
	if len(aliases) > 0 {
		aliasIDs := make([]string, len(aliases))
		for i, a := range aliases {
			aliasIDs[i] = a["id"].(string)
		}

		userRows, err := h.DB.Query(r.Context(),
			`SELECT au.alias_id, au.user_id, au.can_send_as, au.is_default, u.name, u.email
			 FROM alias_users au
			 JOIN users u ON u.id = au.user_id
			 WHERE au.alias_id = ANY($1)
			 ORDER BY u.name`, aliasIDs)
		if err == nil {
			defer userRows.Close()
			aliasUsers := map[string][]map[string]interface{}{}
			for userRows.Next() {
				var aliasID, userID, userName, userEmail string
				var canSendAs, isDefault bool
				if userRows.Scan(&aliasID, &userID, &canSendAs, &isDefault, &userName, &userEmail) == nil {
					aliasUsers[aliasID] = append(aliasUsers[aliasID], map[string]interface{}{
						"user_id": userID, "can_send_as": canSendAs,
						"is_default": isDefault, "name": userName, "email": userEmail,
					})
				}
			}
			for _, a := range aliases {
				id := a["id"].(string)
				if users, ok := aliasUsers[id]; ok {
					a["users"] = users
				} else {
					a["users"] = []map[string]interface{}{}
				}
			}
		}
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
	if err := validateEmail(req.Address); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLength(req.Name, "name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	// Update discovered_addresses if this address was previously discovered
	if _, err := h.DB.Exec(r.Context(),
		`UPDATE discovered_addresses SET type = 'alias', alias_id = $1 WHERE domain_id = $2 AND address = $3`,
		aliasID, req.DomainID, req.Address); err != nil {
		slog.Error("aliases: failed to update discovered address", "error", err)
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id": aliasID, "address": req.Address, "name": req.Name,
	})
}

func (h *AliasHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	aliasID := chi.URLParam(r, "id")
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateLength(req.Name, "name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.DB.Exec(r.Context(),
		`UPDATE aliases SET name = $1 WHERE id = $2 AND org_id = $3`,
		req.Name, aliasID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": aliasID, "name": req.Name})
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

	// Verify target user belongs to the same org
	var targetOrgID string
	if err := h.DB.QueryRow(r.Context(),
		"SELECT org_id FROM users WHERE id = $1", req.UserID,
	).Scan(&targetOrgID); err != nil || targetOrgID != claims.OrgID {
		writeError(w, http.StatusBadRequest, "user not found in this organization")
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

	if _, err := h.DB.Exec(r.Context(),
		`DELETE FROM alias_users WHERE alias_id = $1 AND user_id = $2`,
		aliasID, userID); err != nil {
		slog.Error("aliases: failed to remove alias user", "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) SetDefault(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	aliasID := chi.URLParam(r, "id")

	// Get the domain_id for this alias and verify org ownership
	var domainID string
	err := h.DB.QueryRow(r.Context(),
		`SELECT a.domain_id FROM aliases a WHERE a.id = $1 AND a.org_id = $2`,
		aliasID, claims.OrgID).Scan(&domainID)
	if err != nil {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	// Clear is_default for all of this user's aliases on this domain
	if _, err := h.DB.Exec(r.Context(),
		`UPDATE alias_users SET is_default = false
		 WHERE user_id = $1 AND alias_id IN (
		   SELECT id FROM aliases WHERE domain_id = $2 AND org_id = $3
		 )`,
		claims.UserID, domainID, claims.OrgID); err != nil {
		slog.Error("aliases: failed to clear default alias", "error", err)
	}

	// Set this alias as default (only if user already has access)
	tag, err := h.DB.Exec(r.Context(),
		`UPDATE alias_users SET is_default = true WHERE alias_id = $1 AND user_id = $2`,
		aliasID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set default alias")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusForbidden, "you do not have access to this alias")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) DiscoveredAddresses(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT da.id, da.domain_id, da.address, da.local_part, da.type,
		        (SELECT COUNT(*) FROM emails e
		         WHERE e.domain_id = da.domain_id
		           AND (e.from_address = da.address
		                OR e.to_addresses @> to_jsonb(da.address)
		                OR e.cc_addresses @> to_jsonb(da.address))
		        ) as email_count
		 FROM discovered_addresses da
		 JOIN domains d ON d.id = da.domain_id
		 WHERE d.org_id = $1 AND da.type = 'unclaimed'
		 ORDER BY email_count DESC`,
		claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	defer rows.Close()

	var addresses []map[string]interface{}
	for rows.Next() {
		var id, domainID, address, localPart, addrType string
		var emailCount int
		if rows.Scan(&id, &domainID, &address, &localPart, &addrType, &emailCount) == nil {
			addresses = append(addresses, map[string]interface{}{
				"id": id, "domain_id": domainID, "address": address,
				"local_part": localPart, "type": addrType, "email_count": emailCount,
			})
		}
	}

	if addresses == nil {
		addresses = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, addresses)
}
