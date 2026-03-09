package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
)

type DomainHandler struct {
	Store     store.Store
	ResendSvc *service.ResendService
	EncSvc    *service.EncryptionService
	PublicURL string
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domains, err := h.Store.ListDomains(r.Context(), claims.OrgID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domains, err := h.Store.ListDomains(r.Context(), claims.OrgID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Domain string `json:"domain"`
	}
	if err := readJSON(r, &req); err != nil || req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if err := validateLength(req.Domain, "domain", 253); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create domain via Resend API
	body := map[string]string{"name": req.Domain}
	bodyBytes, ok := marshalOrFail(w, body, "failed to prepare request")
	if !ok {
		return
	}
	respBytes, err := h.ResendSvc.Fetch(r.Context(), claims.OrgID, "POST", "/domains", bodyBytes)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to create domain in Resend")
		return
	}

	var resendDomain struct {
		ID     string          `json:"id"`
		Name   string          `json:"name"`
		Status string          `json:"status"`
		Records json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(respBytes, &resendDomain); err != nil {
		slog.Error("domain: failed to parse Resend response", "error", err)
		writeError(w, http.StatusBadGateway, "failed to parse Resend response")
		return
	}

	domainID, err := h.Store.InsertDomain(r.Context(), claims.OrgID, req.Domain, resendDomain.ID, "pending", resendDomain.Records)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save domain")
		return
	}

	slog.Info("domain: created", "domain", req.Domain, "domain_id", domainID, "resend_domain_id", resendDomain.ID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":               domainID,
		"domain":           req.Domain,
		"resend_domain_id": resendDomain.ID,
		"status":           "pending",
		"dns_records":      resendDomain.Records,
	})
}

func (h *DomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	domainID := chi.URLParam(r, "id")

	resendDomainID, err := h.Store.GetResendDomainID(r.Context(), domainID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	// Verify via Resend
	respBytes, err := h.ResendSvc.Fetch(r.Context(), claims.OrgID, "POST", "/domains/"+resendDomainID+"/verify", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to verify domain")
		return
	}

	var result struct {
		Status  string          `json:"status"`
		Records json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		slog.Error("domain: failed to parse verify response", "domain_id", domainID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to parse verify response")
		return
	}

	// Update local record
	if err := h.Store.UpdateDomainStatus(r.Context(), domainID, result.Status, result.Records); err != nil {
		slog.Error("domain: update after verify failed", "domain_id", domainID, "error", err)
	}

	slog.Info("domain: verified", "domain_id", domainID, "status", result.Status)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      result.Status,
		"dns_records": result.Records,
	})
}

func (h *DomainHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Order []struct {
			ID    string `json:"id"`
			Order int    `json:"order"`
		} `json:"order"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	order := make([]store.DomainOrder, len(req.Order))
	for i, item := range req.Order {
		order[i] = store.DomainOrder{ID: item.ID, Order: item.Order}
	}

	if err := h.Store.ReorderDomains(r.Context(), claims.OrgID, order); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save order")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) UnreadCounts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	var aliasAddrs []string
	if claims.Role != "admin" {
		addrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if addrs == nil {
			addrs = []string{}
		}
		aliasAddrs = addrs
	}

	counts, err := h.Store.GetUnreadCounts(ctx, claims.OrgID, claims.Role, aliasAddrs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get unread counts")
		return
	}

	writeJSON(w, http.StatusOK, counts)
}

func (h *DomainHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Visible []string `json:"visible"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(req.Visible) == 0 {
		writeError(w, http.StatusBadRequest, "at least one domain must remain visible")
		return
	}

	if err := h.Store.UpdateDomainVisibility(r.Context(), claims.OrgID, req.Visible); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) Sync(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	// Fetch domains from Resend
	respBytes, err := h.ResendSvc.Fetch(r.Context(), claims.OrgID, "GET", "/domains", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch domains from Resend")
		return
	}

	var resendResp struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resendResp); err != nil {
		slog.Error("domain: failed to parse sync response", "error", err)
		writeError(w, http.StatusBadGateway, "failed to parse Resend response")
		return
	}

	// Build store-compatible domain info slice
	resendDomains := make([]store.ResendDomainInfo, len(resendResp.Data))
	for i, rd := range resendResp.Data {
		resendDomains[i] = store.ResendDomainInfo{ID: rd.ID, Name: rd.Name, Status: rd.Status}
	}

	if err := h.Store.SyncDomains(r.Context(), claims.OrgID, resendDomains); err != nil {
		slog.Error("domain: sync failed", "org_id", claims.OrgID, "error", err)
	}

	// Return full domain list (including hidden) so frontend can update without a second fetch
	domains, err := h.Store.ListDomains(r.Context(), claims.OrgID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains after sync")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domainID := chi.URLParam(r, "id")
	ctx := r.Context()

	txErr := h.Store.WithTx(ctx, func(tx store.Store) error {
		rows, err := tx.SoftDeleteDomain(ctx, domainID, claims.OrgID)
		if err != nil || rows == 0 {
			return fmt.Errorf("domain not found")
		}
		return tx.CascadeDeleteDomain(ctx, domainID)
	})
	if txErr != nil {
		if txErr.Error() == "domain not found" {
			writeError(w, http.StatusNotFound, "domain not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to delete domain")
		}
		return
	}

	slog.Info("domain: soft-deleted with cascade", "domain_id", domainID, "admin", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) DiscoveredDomains(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	domains, err := h.Store.ListDiscoveredDomains(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list discovered domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) DismissDiscoveredDomain(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.Store.DismissDiscoveredDomain(r.Context(), id, claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to dismiss")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) ReregisterWebhook(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()
	webhookURL := h.PublicURL + "/api/webhooks/resend/" + claims.OrgID

	data, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "POST", "/webhooks", map[string]interface{}{
		"endpoint": webhookURL,
		"events": []string{
			"email.sent",
			"email.delivered",
			"email.bounced",
			"email.complained",
			"email.received",
		},
	})
	if err != nil {
		slog.Error("domain: register webhook failed", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to register webhook")
		return
	}

	var webhookResp struct {
		ID     string `json:"id"`
		Secret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(data, &webhookResp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse webhook response")
		return
	}

	encSecret, encIV, encTag, encErr := h.EncSvc.Encrypt(webhookResp.Secret)
	if encErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt webhook secret")
		return
	}

	if err := h.Store.UpdateWebhookConfig(ctx, claims.OrgID, webhookResp.ID, encSecret, encIV, encTag); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store webhook config")
		return
	}

	slog.Info("domain: webhook re-registered", "org_id", claims.OrgID, "webhook_id", webhookResp.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_id":  webhookResp.ID,
		"webhook_url": webhookURL,
	})
}
