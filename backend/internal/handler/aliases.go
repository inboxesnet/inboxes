package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

type AliasHandler struct {
	Store store.Store
}

func (h *AliasHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	domainID := r.URL.Query().Get("domain_id")

	aliases, err := h.Store.ListAliases(r.Context(), claims.OrgID, domainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list aliases")
		return
	}

	writeJSON(w, http.StatusOK, aliases)
}

func (h *AliasHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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

	aliasID, err := h.Store.CreateAlias(r.Context(), claims.OrgID, req.DomainID, req.Address, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create alias")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id": aliasID, "address": req.Address, "name": req.Name,
	})
}

func (h *AliasHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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

	n, err := h.Store.UpdateAlias(r.Context(), aliasID, claims.OrgID, req.Name)
	if err != nil || n == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": aliasID, "name": req.Name})
}

func (h *AliasHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	aliasID := chi.URLParam(r, "id")
	n, err := h.Store.DeleteAlias(r.Context(), aliasID, claims.OrgID)
	if err != nil || n == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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
	count, _ := h.Store.CheckAliasOrg(r.Context(), aliasID, claims.OrgID)
	if count == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	// Verify target user belongs to the same org
	sameOrg, err := h.Store.CheckUserOrg(r.Context(), req.UserID, claims.OrgID)
	if err != nil || !sameOrg {
		writeError(w, http.StatusBadRequest, "user not found in this organization")
		return
	}

	if err := h.Store.AddAliasUser(r.Context(), aliasID, claims.OrgID, req.UserID, req.CanSendAs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user to alias")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	aliasID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")

	// Verify alias belongs to org
	count, _ := h.Store.CheckAliasOrg(r.Context(), aliasID, claims.OrgID)
	if count == 0 {
		writeError(w, http.StatusNotFound, "alias not found")
		return
	}

	h.Store.RemoveAliasUser(r.Context(), aliasID, userID)

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) SetDefault(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	aliasID := chi.URLParam(r, "id")

	err := h.Store.SetDefaultAlias(r.Context(), aliasID, claims.UserID, claims.OrgID)
	if err != nil {
		if err.Error() == "user does not have access to this alias" {
			writeError(w, http.StatusForbidden, "you do not have access to this alias")
		} else {
			writeError(w, http.StatusNotFound, "alias not found")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AliasHandler) DiscoveredAddresses(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	addresses, err := h.Store.ListDiscoveredAddresses(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	writeJSON(w, http.StatusOK, addresses)
}
