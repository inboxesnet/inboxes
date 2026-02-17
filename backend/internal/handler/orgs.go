package handler

import (
	"net/http"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrgHandler struct {
	DB        *pgxpool.Pool
	EncSvc    *service.EncryptionService
	ResendSvc *service.ResendService
	Bus       *event.Bus
	StripeKey string
}

func (h *OrgHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var name string
	var onboardingCompleted bool
	var hasAPIKey bool

	err := h.DB.QueryRow(r.Context(),
		`SELECT name, onboarding_completed, (resend_api_key_encrypted IS NOT NULL) as has_key
		 FROM orgs WHERE id = $1`, claims.OrgID,
	).Scan(&name, &onboardingCompleted, &hasAPIKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   claims.OrgID,
		"name":                 name,
		"onboarding_completed": onboardingCompleted,
		"has_api_key":          hasAPIKey,
		"billing_enabled":      h.StripeKey != "",
	})
}

func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	var req struct {
		Name   string `json:"name"`
		APIKey string `json:"api_key"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.Name != "" {
		h.DB.Exec(r.Context(),
			`UPDATE orgs SET name = $1, updated_at = now() WHERE id = $2`,
			req.Name, claims.OrgID)
	}

	if req.APIKey != "" {
		ciphertext, iv, tag, err := h.EncSvc.Encrypt(req.APIKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
			return
		}
		h.DB.Exec(r.Context(),
			`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2, resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
			ciphertext, iv, tag, claims.OrgID)
	}

	w.WriteHeader(http.StatusNoContent)
}

