package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CronHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
}

func (h *CronHandler) PurgeTrash(w http.ResponseWriter, r *http.Request) {
	// Delete threads that have been in trash for more than 30 days
	tag, err := h.DB.Exec(r.Context(),
		`DELETE FROM threads WHERE folder = 'trash' AND trash_expires_at IS NOT NULL AND trash_expires_at < now()`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to purge trash")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"purged": tag.RowsAffected(),
	})
}

// CleanupWebhooks iterates orgs with webhook IDs and removes stale Resend webhooks
// that match our URL pattern but aren't the currently active webhook.
func (h *CronHandler) CleanupWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.DB.Query(ctx,
		`SELECT id, resend_webhook_id FROM orgs WHERE resend_webhook_id IS NOT NULL AND resend_webhook_id != ''`,
	)
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
