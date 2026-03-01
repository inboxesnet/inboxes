package handler

import (
	"log/slog"
	"net/http"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/subscription"
)

type OrgHandler struct {
	DB         *pgxpool.Pool
	RDB        *redis.Client
	EncSvc     *service.EncryptionService
	ResendSvc  *service.ResendService
	Bus        *event.Bus
	StripeKey  string
	LimiterMap *queue.OrgLimiterMap
}

func (h *OrgHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var name string
	var onboardingCompleted bool
	var hasAPIKey bool
	var resendRPS int

	err := h.DB.QueryRow(r.Context(),
		`SELECT name, onboarding_completed, (resend_api_key_encrypted IS NOT NULL) as has_key, resend_rps
		 FROM orgs WHERE id = $1`, claims.OrgID,
	).Scan(&name, &onboardingCompleted, &hasAPIKey, &resendRPS)
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
		"resend_rps":           resendRPS,
	})
}

func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Name      string `json:"name"`
		APIKey    string `json:"api_key"`
		ResendRPS *int   `json:"resend_rps"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.ResendRPS != nil {
		rps := *req.ResendRPS
		if rps < 1 || rps > 100 {
			writeError(w, http.StatusBadRequest, "resend_rps must be between 1 and 100")
			return
		}
		if _, err := h.DB.Exec(r.Context(),
			`UPDATE orgs SET resend_rps = $1, updated_at = now() WHERE id = $2`,
			rps, claims.OrgID); err != nil {
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
		if _, err := h.DB.Exec(r.Context(),
			`UPDATE orgs SET name = $1, updated_at = now() WHERE id = $2`,
			req.Name, claims.OrgID); err != nil {
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
		if _, err := h.DB.Exec(r.Context(),
			`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2, resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
			ciphertext, iv, tag, claims.OrgID); err != nil {
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
	var isOwner bool
	err := h.DB.QueryRow(r.Context(),
		`SELECT is_owner FROM users WHERE id = $1 AND org_id = $2`,
		claims.UserID, claims.OrgID).Scan(&isOwner)
	if err != nil || !isOwner {
		writeError(w, http.StatusForbidden, "only the organization owner can delete it")
		return
	}

	ctx := r.Context()

	// Cancel Stripe subscription if in commercial mode
	if h.StripeKey != "" {
		var stripeSubID *string
		if err := h.DB.QueryRow(ctx,
			`SELECT stripe_subscription_id FROM orgs WHERE id = $1`, claims.OrgID,
		).Scan(&stripeSubID); err == nil && stripeSubID != nil && *stripeSubID != "" {
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
	var webhookID *string
	if err := h.DB.QueryRow(ctx,
		"SELECT resend_webhook_id FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&webhookID); err == nil && webhookID != nil && *webhookID != "" {
		if _, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "DELETE", "/webhooks/"+*webhookID, nil); err != nil {
			slog.Error("org: failed to unregister Resend webhook", "org_id", claims.OrgID, "webhook_id", *webhookID, "error", err)
		} else {
			slog.Info("org: unregistered Resend webhook", "org_id", claims.OrgID, "webhook_id", *webhookID)
		}
	}

	// Soft-delete: cascade to all child entities in a transaction
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE orgs SET deleted_at = now(), stripe_subscription_id = NULL,
		 resend_webhook_id = NULL, resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = NULL, resend_webhook_secret_iv = NULL, resend_webhook_secret_tag = NULL,
		 updated_at = now() WHERE id = $1`,
		claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET status = 'disabled', updated_at = now() WHERE org_id = $1`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to disable users on delete", "error", err, "org_id", claims.OrgID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE domains SET status = 'deleted', hidden = true, updated_at = now() WHERE org_id = $1`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to mark domains deleted", "error", err, "org_id", claims.OrgID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE aliases SET deleted_at = now() WHERE org_id = $1 AND deleted_at IS NULL`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to soft-delete aliases", "error", err, "org_id", claims.OrgID)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE threads SET deleted_at = now(), updated_at = now() WHERE org_id = $1 AND deleted_at IS NULL`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to soft-delete threads", "error", err, "org_id", claims.OrgID)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM discovered_addresses WHERE domain_id IN (SELECT id FROM domains WHERE org_id = $1)`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to delete discovered_addresses", "error", err, "org_id", claims.OrgID)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM thread_labels WHERE org_id = $1`,
		claims.OrgID); err != nil {
		slog.Error("orgs: failed to delete thread_labels", "error", err, "org_id", claims.OrgID)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	// Revoke all tokens for every user in the org and clear status caches
	rows, err := h.DB.Query(ctx, `SELECT id FROM users WHERE org_id = $1`, claims.OrgID)
	if err == nil {
		defer rows.Close()
		orgBlacklist := service.NewTokenBlacklist(h.RDB)
		for rows.Next() {
			var uid string
			if rows.Scan(&uid) == nil {
				if err := orgBlacklist.RevokeAllForUser(ctx, uid); err != nil {
					slog.Error("orgs: session revocation failed on org delete", "user_id", uid, "error", err)
				}
				orgBlacklist.ClearSessions(ctx, uid)
				if h.RDB != nil {
					h.RDB.Del(ctx, "user:status:"+uid)
				}
			}
		}
	}

	// Cancel all pending/running email jobs and remove from Redis queue
	jobRows, jobErr := h.DB.Query(ctx,
		`UPDATE email_jobs SET status='cancelled', error_message='org deleted', updated_at=now()
		 WHERE org_id = $1 AND status IN ('pending', 'running')
		 RETURNING id`,
		claims.OrgID,
	)
	if jobErr == nil {
		defer jobRows.Close()
		for jobRows.Next() {
			var jobID string
			if jobRows.Scan(&jobID) == nil {
				h.RDB.LRem(ctx, "email:jobs", 0, jobID)
			}
		}
	}

	slog.Info("org: soft-deleted", "org_id", claims.OrgID, "by", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *OrgHandler) HardDelete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	// Only the owner can hard-delete
	var isOwner bool
	err := h.DB.QueryRow(r.Context(),
		`SELECT is_owner FROM users WHERE id = $1 AND org_id = $2`,
		claims.UserID, claims.OrgID).Scan(&isOwner)
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
	var orgName string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT name FROM orgs WHERE id = $1`, claims.OrgID,
	).Scan(&orgName); err != nil {
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
		var stripeSubID *string
		if err := h.DB.QueryRow(ctx,
			`SELECT stripe_subscription_id FROM orgs WHERE id = $1`, claims.OrgID,
		).Scan(&stripeSubID); err == nil && stripeSubID != nil && *stripeSubID != "" {
			stripe.Key = h.StripeKey
			if _, err := subscription.Cancel(*stripeSubID, nil); err != nil {
				slog.Error("org: hard-delete: failed to cancel Stripe subscription", "org_id", claims.OrgID, "error", err)
			}
		}
	}

	// Unregister Resend webhook (best-effort)
	var webhookID *string
	if err := h.DB.QueryRow(ctx,
		"SELECT resend_webhook_id FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&webhookID); err == nil && webhookID != nil && *webhookID != "" {
		if _, err := h.ResendSvc.Fetch(ctx, claims.OrgID, "DELETE", "/webhooks/"+*webhookID, nil); err != nil {
			slog.Error("org: hard-delete: failed to unregister webhook", "org_id", claims.OrgID, "error", err)
		}
	}

	// Revoke all sessions before deletion
	rows, err := h.DB.Query(ctx, `SELECT id FROM users WHERE org_id = $1`, claims.OrgID)
	if err == nil {
		defer rows.Close()
		bl := service.NewTokenBlacklist(h.RDB)
		for rows.Next() {
			var uid string
			if rows.Scan(&uid) == nil {
				bl.RevokeAllForUser(ctx, uid)
				bl.ClearSessions(ctx, uid)
				if h.RDB != nil {
					h.RDB.Del(ctx, "user:status:"+uid)
				}
			}
		}
		rows.Close()
	}

	// Hard-delete all org data in a transaction
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	orgID := claims.OrgID

	// Delete in dependency order (children first)
	// attachments reference emails (no cascade)
	if _, err := tx.Exec(ctx, `DELETE FROM attachments WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: attachments", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_jobs WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: email_jobs", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sync_jobs WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: sync_jobs", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM events WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: events", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM drafts WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: drafts", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM thread_labels WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: thread_labels", "error", err)
	}
	// emails cascade from threads, but delete explicitly for safety
	if _, err := tx.Exec(ctx, `DELETE FROM emails WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: emails", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM threads WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: threads", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM discovered_addresses WHERE domain_id IN (SELECT id FROM domains WHERE org_id = $1)`, orgID); err != nil {
		slog.Error("org: hard-delete: discovered_addresses", "error", err)
	}
	// alias_users cascade from aliases, but delete explicitly
	if _, err := tx.Exec(ctx, `DELETE FROM alias_users WHERE alias_id IN (SELECT id FROM aliases WHERE org_id = $1)`, orgID); err != nil {
		slog.Error("org: hard-delete: alias_users", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM aliases WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: aliases", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_reassignments WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: user_reassignments", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM org_labels WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: org_labels", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM domains WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: domains", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE org_id = $1`, orgID); err != nil {
		slog.Error("org: hard-delete: users", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit hard-delete")
		return
	}

	slog.Info("org: hard-deleted (GDPR)", "org_id", orgID, "by", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

