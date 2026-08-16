package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
)

type CronHandler struct {
	Store     store.Store
	ResendSvc *service.ResendService
}

// isInstanceOwner reports whether the caller has users.is_owner set. Only the
// instance owner may run maintenance across every org on the instance.
func (h *CronHandler) isInstanceOwner(r *http.Request, userID string) bool {
	var isOwner bool
	if err := h.Store.Q().QueryRow(r.Context(),
		"SELECT is_owner FROM users WHERE id = $1", userID,
	).Scan(&isOwner); err != nil {
		return false
	}
	return isOwner
}

func (h *CronHandler) PurgeTrash(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	// An org admin purges their own org. The instance owner purges all orgs.
	orgID := claims.OrgID
	if h.isInstanceOwner(r, claims.UserID) {
		orgID = ""
	}

	purged, err := h.Store.PurgeExpiredTrash(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to purge trash")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"purged": purged,
	})
}

// CleanupWebhooks iterates orgs with webhook IDs and removes stale Resend webhooks
// that match our URL pattern but aren't the currently active webhook.
func (h *CronHandler) CleanupWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims := middleware.GetCurrentUser(r.Context())

	// An org admin cleans up their own org's webhooks. The instance owner
	// sweeps every org. Without this scope, any tenant's admin could delete
	// other tenants' Resend webhooks and break their inbound mail.
	query := `SELECT id, resend_webhook_id FROM orgs
	          WHERE resend_webhook_id IS NOT NULL AND resend_webhook_id != ''`
	args := []interface{}{}
	if !h.isInstanceOwner(r, claims.UserID) {
		query += ` AND id = $1`
		args = append(args, claims.OrgID)
	}

	rows, err := h.Store.Q().Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query orgs")
		return
	}
	defer rows.Close()

	var cleaned, failed int
	for rows.Next() {
		var orgID, currentWebhookID string
		if err := rows.Scan(&orgID, &currentWebhookID); err != nil {
			continue
		}

		// List all webhooks from Resend
		data, err := h.ResendSvc.Fetch(ctx, orgID, "GET", "/webhooks", nil)
		if err != nil {
			slog.Error("cleanup-webhooks: list failed", "org_id", orgID, "error", err)
			failed++
			continue
		}

		var webhookList struct {
			Data []struct {
				ID       string `json:"id"`
				Endpoint string `json:"endpoint"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &webhookList); err != nil {
			slog.Error("cleanup-webhooks: parse failed", "org_id", orgID, "error", err)
			failed++
			continue
		}

		for _, wh := range webhookList.Data {
			// Skip the current active webhook
			if wh.ID == currentWebhookID {
				continue
			}
			// Only delete webhooks that match our URL pattern
			if !strings.Contains(wh.Endpoint, "/api/webhooks/resend/") {
				continue
			}
			_, err := h.ResendSvc.Fetch(ctx, orgID, "DELETE", "/webhooks/"+wh.ID, nil)
			if err != nil {
				slog.Error("cleanup-webhooks: delete failed", "org_id", orgID, "webhook_id", wh.ID, "error", err)
				failed++
			} else {
				slog.Info("cleanup-webhooks: deleted stale webhook", "org_id", orgID, "webhook_id", wh.ID)
				cleaned++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cleaned": cleaned,
		"failed":  failed,
	})
}
