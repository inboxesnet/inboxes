package handler

import (
	"log/slog"
	"net/http"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/subscription"
)

type OrgHandler struct {
	Store      store.Store
	RDB        *redis.Client
	EncSvc     *service.EncryptionService
	ResendSvc  *service.ResendService
	Bus        *event.Bus
	StripeKey  string
	LimiterMap *queue.OrgLimiterMap
}

func (h *OrgHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	settings, err := h.Store.GetOrgSettings(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	resp := map[string]interface{}{
		"id":                   claims.OrgID,
		"name":                 settings["name"],
		"onboarding_completed": settings["onboarding_completed"],
		"has_api_key":          settings["has_api_key"],
		"billing_enabled":      h.StripeKey != "",
		"resend_rps":           settings["resend_rps"],
	}
	if h.StripeKey == "" {
		resp["auto_poll_enabled"] = settings["auto_poll_enabled"]
		resp["auto_poll_interval"] = settings["auto_poll_interval"]
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Name             string `json:"name"`
		APIKey           string `json:"api_key"`
		ResendRPS        *int   `json:"resend_rps"`
		AutoPollEnabled  *bool  `json:"auto_poll_enabled"`
		AutoPollInterval *int   `json:"auto_poll_interval"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Auto-poll settings (self-hosted only)
	if h.StripeKey == "" && req.AutoPollEnabled != nil {
		if err := h.Store.UpdateOrgAutoPoll(r.Context(), claims.OrgID, *req.AutoPollEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update auto-poll setting")
			return
		}
	}
	if h.StripeKey == "" && req.AutoPollInterval != nil {
		interval := *req.AutoPollInterval
		if interval < 120 || interval > 3600 {
			writeError(w, http.StatusBadRequest, "auto_poll_interval must be between 120 and 3600 seconds")
			return
		}
		if err := h.Store.UpdateOrgAutoPollInterval(r.Context(), claims.OrgID, interval); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update auto-poll interval")
			return
		}
	}

	if req.ResendRPS != nil {
		rps := *req.ResendRPS
		if rps < 1 || rps > 100 {
			writeError(w, http.StatusBadRequest, "resend_rps must be between 1 and 100")
			return
		}
		if err := h.Store.UpdateOrgRPS(r.Context(), claims.OrgID, rps); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update rate limit")
			return
		}
		if h.LimiterMap != nil {
			h.LimiterMap.UpdateOrgRPS(claims.OrgID, rps)
		}
	}

	if req.Name != "" {
		if err := validateLength(req.Name, "name", 255); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.Store.UpdateOrgName(r.Context(), claims.OrgID, req.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update org name")
			return
		}
	}

	if req.APIKey != "" {
		ciphertext, iv, tag, err := h.EncSvc.Encrypt(req.APIKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
			return
		}
		if err := h.Store.UpdateOrgAPIKey(r.Context(), claims.OrgID, ciphertext, iv, tag); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update API key")
			return
		}
		h.ResendSvc.InvalidateOrgKeyCache(claims.OrgID)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *OrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	// Only the owner can delete the org
	isOwner, err := h.Store.IsOrgOwner(r.Context(), claims.UserID, claims.OrgID)
	if err != nil || !isOwner {
		writeError(w, http.StatusForbidden, "only the organization owner can delete it")
		return
	}

	ctx := r.Context()

	// Cancel Stripe subscription if in commercial mode
	if h.StripeKey != "" {
		stripeSubID, _ := h.Store.GetStripeSubscriptionID(ctx, claims.OrgID)
		if stripeSubID != nil && *stripeSubID != "" {
			stripe.Key = h.StripeKey
			if _, err := subscription.Cancel(*stripeSubID, nil); err != nil {
				slog.Error("org: failed to cancel Stripe subscription", "org_id", claims.OrgID, "sub_id", *stripeSubID, "error", err)
				// Continue with deletion — don't block on Stripe failure
			} else {
				slog.Info("org: cancelled Stripe subscription", "org_id", claims.OrgID, "sub_id", *stripeSubID)
			}
		}
	}

	// Unregister Resend webhook (best-effort — don't block deletion on failure)
	webhookID, _ := h.Store.GetWebhookID(ctx, claims.OrgID)
	if webhookID != nil && *webhookID != "" {
		if _, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "DELETE", "/webhooks/"+*webhookID, nil); err != nil {
			slog.Error("org: failed to unregister Resend webhook", "org_id", claims.OrgID, "webhook_id", *webhookID, "error", err)
		} else {
			slog.Info("org: unregistered Resend webhook", "org_id", claims.OrgID, "webhook_id", *webhookID)
		}
	}

	// Soft-delete: cascade to all child entities in a transaction
	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		return tx.SoftDeleteOrg(ctx, claims.OrgID)
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	// Revoke all tokens for every user in the org and clear status caches
	userIDs, err := h.Store.ListOrgUserIDs(ctx, claims.OrgID)
	if err == nil {
		orgBlacklist := service.NewTokenBlacklist(h.RDB)
		for _, uid := range userIDs {
			if err := orgBlacklist.RevokeAllForUser(ctx, uid); err != nil {
				slog.Error("orgs: session revocation failed on org delete", "user_id", uid, "error", err)
			}
			orgBlacklist.ClearSessions(ctx, uid)
			if h.RDB != nil {
				h.RDB.Del(ctx, "user:status:"+uid)
			}
		}
	}

	// Cancel all pending/running email jobs and remove from Redis queue
	jobIDs, jobErr := h.Store.CancelOrgJobs(ctx, claims.OrgID)
	if jobErr == nil {
		for _, jobID := range jobIDs {
			h.RDB.LRem(ctx, "email:jobs", 0, jobID)
		}
	}

	slog.Info("org: deleted", "org_id", claims.OrgID, "by", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *OrgHandler) HardDelete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	// Only the owner can delete
	isOwner, err := h.Store.IsOrgOwner(r.Context(), claims.UserID, claims.OrgID)
	if err != nil || !isOwner {
		writeError(w, http.StatusForbidden, "only the organization owner can permanently delete it")
		return
	}

	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := readJSON(r, &req); err != nil || req.Confirm == "" {
		writeError(w, http.StatusBadRequest, "confirmation is required: {\"confirm\": \"DELETE <org-name>\"}")
		return
	}

	// Verify confirmation matches org name
	orgName, err := h.Store.GetOrgNameByID(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}
	if req.Confirm != "DELETE "+orgName {
		writeError(w, http.StatusBadRequest, "confirmation does not match: expected \"DELETE "+orgName+"\"")
		return
	}

	ctx := r.Context()

	// Cancel Stripe subscription if active
	if h.StripeKey != "" {
		stripeSubID, _ := h.Store.GetStripeSubscriptionID(ctx, claims.OrgID)
		if stripeSubID != nil && *stripeSubID != "" {
			stripe.Key = h.StripeKey
			if _, err := subscription.Cancel(*stripeSubID, nil); err != nil {
				slog.Error("org: delete: failed to cancel Stripe subscription", "org_id", claims.OrgID, "error", err)
			}
		}
	}

	// Unregister Resend webhook (best-effort)
	webhookID, _ := h.Store.GetWebhookID(ctx, claims.OrgID)
	if webhookID != nil && *webhookID != "" {
		if _, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "DELETE", "/webhooks/"+*webhookID, nil); err != nil {
			slog.Error("org: delete: failed to unregister webhook", "org_id", claims.OrgID, "error", err)
		}
	}

	// Revoke all sessions before deletion
	userIDs, err := h.Store.ListOrgUserIDs(ctx, claims.OrgID)
	if err == nil {
		bl := service.NewTokenBlacklist(h.RDB)
		for _, uid := range userIDs {
			bl.RevokeAllForUser(ctx, uid)
			bl.ClearSessions(ctx, uid)
			if h.RDB != nil {
				h.RDB.Del(ctx, "user:status:"+uid)
			}
		}
	}

	// Soft-delete the org
	rowsAffected, err := h.Store.HardDeleteOrg(ctx, claims.OrgID)
	if err != nil || rowsAffected == 0 {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	slog.Info("org: deleted", "org_id", claims.OrgID, "by", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

