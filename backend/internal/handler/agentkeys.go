package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/mcp"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

// AgentKeyHandler manages the caller's own MCP credentials: API keys they
// created plus OAuth tokens agents obtained through the consent flow.
type AgentKeyHandler struct {
	Store store.Store
}

// List returns the caller's active agent credentials. Raw tokens are never
// stored, so only metadata comes back.
func (h *AgentKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.Store.Q().Query(r.Context(),
		`SELECT id, kind, name, created_at, last_used_at, expires_at
		 FROM agent_tokens
		 WHERE user_id = $1 AND org_id = $2 AND revoked_at IS NULL
		 ORDER BY created_at DESC`,
		claims.UserID, claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	defer rows.Close()

	keys := []map[string]interface{}{}
	for rows.Next() {
		var id, kind, name string
		var createdAt time.Time
		var lastUsedAt, expiresAt *time.Time
		if rows.Scan(&id, &kind, &name, &createdAt, &lastUsedAt, &expiresAt) == nil {
			keys = append(keys, map[string]interface{}{
				"id":           id,
				"kind":         kind,
				"name":         name,
				"created_at":   createdAt,
				"last_used_at": lastUsedAt,
				"expires_at":   expiresAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

// Create makes a new API key for the caller and returns the raw key once.
func (h *AgentKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Name == "" {
		req.Name = "API key"
	}
	if err := validateLength(req.Name, "name", 100); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Cap keys per user so a runaway client cannot fill the table.
	var count int
	h.Store.Q().QueryRow(r.Context(),
		`SELECT COUNT(*) FROM agent_tokens WHERE user_id = $1 AND revoked_at IS NULL`,
		claims.UserID,
	).Scan(&count)
	if count >= 25 {
		writeError(w, http.StatusBadRequest, "too many active keys — revoke one first")
		return
	}

	raw, err := mcp.NewToken("inbx_k")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	var id string
	if err := h.Store.Q().QueryRow(r.Context(),
		`INSERT INTO agent_tokens (org_id, user_id, kind, name, token_hash)
		 VALUES ($1, $2, 'key', $3, $4) RETURNING id`,
		claims.OrgID, claims.UserID, req.Name, mcp.HashToken(raw),
	).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store key")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   id,
		"name": req.Name,
		"key":  raw, // shown once; only the hash is stored
	})
}

// Revoke disables one of the caller's credentials.
func (h *AgentKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	id := chi.URLParam(r, "id")

	tag, err := h.Store.Q().Exec(r.Context(),
		`UPDATE agent_tokens SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND org_id = $3 AND revoked_at IS NULL`,
		id, claims.UserID, claims.OrgID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "revoked"})
}
