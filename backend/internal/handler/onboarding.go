package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
)

type OnboardingHandler struct {
	Store     store.Store
	ResendSvc *service.ResendService
	EncSvc    *service.EncryptionService
	Bus       *event.Bus
	PublicURL string
}

type connectRequest struct {
	APIKey string `json:"api_key"`
}

type resendDomain struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Region    string          `json:"region"`
	Records   json.RawMessage `json:"records"`
	CreatedAt string          `json:"created_at"`
}

type resendDomainList struct {
	Data []resendDomain `json:"data"`
}

// Status returns the current onboarding step based on DB state.
// This lets the frontend resume where it left off after a page close.
func (h *OnboardingHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()

	// Check if API key is stored
	hasAPIKey, err := h.Store.HasAPIKey(ctx, claims.OrgID)
	if err != nil {
		slog.Error("onboarding: failed to check API key", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check onboarding status")
		return
	}

	if !hasAPIKey {
		writeJSON(w, http.StatusOK, map[string]string{"step": "connect"})
		return
	}

	// Check if domains exist (non-hidden)
	domainCount, err := h.Store.CountVisibleDomains(ctx, claims.OrgID)
	if err != nil {
		slog.Error("onboarding: failed to count domains", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check onboarding status")
		return
	}

	if domainCount == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"step": "domains"})
		return
	}

	// Check if a sync is still running
	syncJobID, phase, syncErr := h.Store.GetActiveSyncJob(ctx, claims.OrgID)
	if syncErr == nil {
		// Check the phase — if aliases are ready, let user configure them while import continues
		if phase == "aliases_ready" || phase == "importing" || phase == "addresses" || phase == "done" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"step":             "addresses",
				"sync_in_progress": true,
				"sync_job_id":      syncJobID,
			})
			return
		}
		// Still scanning — keep on sync step
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"step":             "sync",
			"sync_in_progress": true,
			"sync_job_id":      syncJobID,
		})
		return
	}

	// Check if emails have been imported
	emailCount, err := h.Store.CountEmails(ctx, claims.OrgID)
	if err != nil {
		slog.Error("onboarding: failed to count emails", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check onboarding status")
		return
	}

	if emailCount == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"step": "sync"})
		return
	}

	// Sync done + emails exist — go to addresses step
	writeJSON(w, http.StatusOK, map[string]string{"step": "addresses"})
}

func (h *OnboardingHandler) Connect(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req connectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// Validate key by fetching domains from Resend
	data, err := service.ResendDirectFetch(req.APIKey, "GET", "/domains", nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid API key: could not fetch domains")
		return
	}

	var domainList resendDomainList
	if err := json.Unmarshal(data, &domainList); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse domains response")
		return
	}

	// Encrypt and store API key
	ciphertext, iv, tag, err := h.EncSvc.Encrypt(req.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
		return
	}

	ctx := r.Context()

	// Wrap API key update + domain upsert loop in a single transaction
	var domains []map[string]interface{}
	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		if err := tx.StoreEncryptedAPIKey(ctx, claims.OrgID, ciphertext, iv, tag); err != nil {
			return err
		}

		// Upsert domains
		for i, d := range domainList.Data {
			status := "pending"
			if d.Status == "verified" || d.Status == "active" {
				status = "active"
			}
			domainID, err := tx.UpsertDomain(ctx, claims.OrgID, d.Name, d.ID, status, d.Records, i)
			if err != nil {
				return err
			}
			domains = append(domains, map[string]interface{}{
				"id":               domainID,
				"domain":           d.Name,
				"resend_domain_id": d.ID,
				"status":           status,
			})
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save changes")
		return
	}

	slog.Info("onboarding: connected", "org_id", claims.OrgID, "domains_imported", len(domains))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"domains": domains,
	})
}

func (h *OnboardingHandler) SelectDomains(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		DomainIDs []string `json:"domain_ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		return tx.SelectDomains(ctx, claims.OrgID, req.DomainIDs)
	}); err != nil {
		slog.Error("onboarding: select domains failed", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "domains updated"})
}

func (h *OnboardingHandler) SetupWebhook(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()

	webhookURL := h.PublicURL + "/api/webhooks/resend/" + claims.OrgID

	// Skip webhook registration if PUBLIC_URL is localhost — Resend can't reach it
	if strings.Contains(h.PublicURL, "localhost") || strings.Contains(h.PublicURL, "127.0.0.1") {
		slog.Warn("onboarding: skipping webhook registration — PUBLIC_URL is localhost",
			"org_id", claims.OrgID, "public_url", h.PublicURL)

		// Auto-enable polling so the user still gets new emails without webhooks
		if _, err := h.Store.Q().Exec(ctx,
			`UPDATE orgs SET auto_poll_enabled = true WHERE id = $1`, claims.OrgID); err != nil {
			slog.Error("onboarding: failed to enable auto-poll", "org_id", claims.OrgID, "error", err)
		} else {
			slog.Info("onboarding: auto-poll enabled (no webhook)", "org_id", claims.OrgID)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"webhook_skipped": true,
			"reason":          "PUBLIC_URL points to localhost — Resend cannot deliver webhooks here. Auto-polling has been enabled so new emails will be checked every 5 minutes. For real-time delivery, set up a tunnel (ngrok, Cloudflare Tunnel) and update PUBLIC_URL.",
		})
		return
	}

	// Register webhook with Resend
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
		slog.Error("onboarding: register webhook failed", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register webhook")
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

	// Encrypt webhook secret before storing
	encSecret, encIV, encTag, encErr := h.EncSvc.Encrypt(webhookResp.Secret)
	if encErr != nil {
		slog.Error("onboarding: encrypt webhook secret failed", "org_id", claims.OrgID, "error", encErr)
		writeError(w, http.StatusInternalServerError, "failed to encrypt webhook secret")
		return
	}

	if err := h.Store.StoreWebhookConfig(ctx, claims.OrgID, webhookResp.ID, encSecret, encIV, encTag); err != nil {
		slog.Error("onboarding: store webhook config failed", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store webhook config")
		return
	}

	slog.Info("onboarding: webhook registered", "org_id", claims.OrgID, "webhook_id", webhookResp.ID, "webhook_url", webhookURL)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_id":  webhookResp.ID,
		"webhook_url": webhookURL,
	})
}

func (h *OnboardingHandler) GetAddresses(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()
	addresses, err := h.Store.GetDiscoveredAddresses(ctx, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	writeJSON(w, http.StatusOK, addresses)
}

type addressSetup struct {
	Address string `json:"address"`
	Type    string `json:"type"` // "individual", "group", "skip"
	Name    string `json:"name"`
}

type setupAddressesRequest struct {
	Addresses []addressSetup `json:"addresses"`
}

func (h *OnboardingHandler) SetupAddresses(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req setupAddressesRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		for _, addr := range req.Addresses {
			if err := tx.SetupAddress(ctx, claims.OrgID, claims.UserID, addr.Address, addr.Type, addr.Name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.Error("onboarding: setup addresses failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save address configuration")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "addresses configured"})
}

func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()
	if err := h.Store.CompleteOnboarding(ctx, claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete onboarding")
		return
	}

	// Get first domain for redirect
	firstDomainID, _ := h.Store.GetFirstDomainID(ctx, claims.OrgID)

	slog.Info("onboarding: completed", "org_id", claims.OrgID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":         "onboarding completed",
		"first_domain_id": firstDomainID,
	})
}
