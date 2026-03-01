package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OnboardingHandler struct {
	DB        *pgxpool.Pool
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
	var hasAPIKey bool
	if err := h.DB.QueryRow(ctx,
		"SELECT (resend_api_key_encrypted IS NOT NULL) FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&hasAPIKey); err != nil {
		slog.Error("onboarding: failed to check API key", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check onboarding status")
		return
	}

	if !hasAPIKey {
		writeJSON(w, http.StatusOK, map[string]string{"step": "connect"})
		return
	}

	// Check if domains exist (non-hidden)
	var domainCount int
	if err := h.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM domains WHERE org_id = $1 AND hidden = false", claims.OrgID,
	).Scan(&domainCount); err != nil {
		slog.Error("onboarding: failed to count domains", "org_id", claims.OrgID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check onboarding status")
		return
	}

	if domainCount == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"step": "domains"})
		return
	}

	// Check if a sync is still running
	var syncJobID string
	err := h.DB.QueryRow(ctx,
		`SELECT id FROM sync_jobs WHERE org_id = $1 AND status IN ('pending', 'running')
		 ORDER BY created_at DESC LIMIT 1`, claims.OrgID,
	).Scan(&syncJobID)
	if err == nil {
		// Check the phase — if aliases are ready, let user configure them while import continues
		var phase string
		warnIfErr(h.DB.QueryRow(ctx, `SELECT phase FROM sync_jobs WHERE id = $1`, syncJobID).Scan(&phase),
			"onboarding: sync job phase lookup failed", "job_id", syncJobID)
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
	var emailCount int
	if err := h.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM emails WHERE org_id = $1", claims.OrgID,
	).Scan(&emailCount); err != nil {
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
	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtx)

	_, err = tx.Exec(dbCtx,
		`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2,
		 resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
		ciphertext, iv, tag, claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store API key")
		return
	}

	// Upsert domains
	var domains []map[string]interface{}
	for i, d := range domainList.Data {
		status := "pending"
		if d.Status == "verified" || d.Status == "active" {
			status = "active"
		}
		var domainID string
		err := tx.QueryRow(dbCtx,
			`INSERT INTO domains (org_id, domain, resend_domain_id, status, dns_records, display_order)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (domain) DO UPDATE SET resend_domain_id = $3, status = $4, dns_records = $5, updated_at = now()
			 RETURNING id`,
			claims.OrgID, d.Name, d.ID, status, d.Records, i,
		).Scan(&domainID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store domain: "+d.Name)
			return
		}
		domains = append(domains, map[string]interface{}{
			"id":               domainID,
			"domain":           d.Name,
			"resend_domain_id": d.ID,
			"status":           status,
		})
	}

	if err := tx.Commit(dbCtx); err != nil {
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

	// Wrap in transaction so a partial failure doesn't leave all domains hidden
	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtx)

	// Hide all, then unhide selected
	_, err = tx.Exec(dbCtx,
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}
	_, err = tx.Exec(dbCtx,
		`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
		claims.OrgID, req.DomainIDs,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	if err := tx.Commit(dbCtx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save domain selection")
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
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"webhook_skipped": true,
			"reason":          "PUBLIC_URL points to localhost — Resend cannot deliver webhooks here. Set up a tunnel (ngrok, Cloudflare Tunnel) and update PUBLIC_URL to receive incoming emails in real time.",
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
		Secret string `json:"secret"`
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

	_, err = h.DB.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1,
		 resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = $2, resend_webhook_secret_iv = $3, resend_webhook_secret_tag = $4,
		 updated_at = now() WHERE id = $5`,
		webhookResp.ID, encSecret, encIV, encTag, claims.OrgID,
	)
	if err != nil {
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
	rows, err := h.DB.Query(ctx,
		`SELECT da.id, da.address, da.local_part, da.type,
		        (SELECT COUNT(*) FROM emails e
		         WHERE e.domain_id = da.domain_id
		           AND (e.from_address = da.address
		                OR e.to_addresses @> to_jsonb(da.address)
		                OR e.cc_addresses @> to_jsonb(da.address))
		        ) as email_count
		 FROM discovered_addresses da
		 JOIN domains d ON d.id = da.domain_id
		 WHERE d.org_id = $1 AND d.hidden = false
		 ORDER BY email_count DESC
		 LIMIT 500`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	addresses, err := scanMaps(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	writeJSON(w, http.StatusOK, addresses)
}

type addressSetup struct {
	Address string `json:"address"`
	Type    string `json:"type"` // "person", "alias", "skip"
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
	for _, addr := range req.Addresses {
		if err := h.setupOneAddress(ctx, claims.OrgID, addr); err != nil {
			slog.Error("onboarding: setup address failed", "address", addr.Address, "error", err)
			// Continue to next address — partial success is acceptable
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "addresses configured"})
}

// setupOneAddress processes a single address within its own transaction.
func (h *OnboardingHandler) setupOneAddress(ctx context.Context, orgID string, addr addressSetup) error {
	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 {
		return nil
	}
	domain := parts[1]

	var domainID string
	if err := h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
		orgID, domain,
	).Scan(&domainID); err != nil {
		return nil // unknown domain, skip
	}

	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(dbCtx)

	switch addr.Type {
	case "person":
		name := addr.Name
		if name == "" {
			name = parts[0]
		}
		var userID string
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO users (org_id, email, name, status)
			 VALUES ($1, $2, $3, 'placeholder')
			 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			orgID, addr.Address, name,
		).Scan(&userID); err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}
		if _, err := tx.Exec(dbCtx,
			`UPDATE discovered_addresses SET type = 'user', user_id = $1
			 WHERE domain_id = $2 AND address = $3`,
			userID, domainID, addr.Address,
		); err != nil {
			return fmt.Errorf("update discovered_addresses: %w", err)
		}
		if _, err := tx.Exec(dbCtx,
			`UPDATE emails SET user_id = $1 WHERE domain_id = $2 AND from_address = $3`,
			userID, domainID, addr.Address,
		); err != nil {
			return fmt.Errorf("reassign emails: %w", err)
		}
		if _, err := tx.Exec(dbCtx,
			`UPDATE threads SET user_id = $1 WHERE domain_id = $2 AND id IN (
				SELECT DISTINCT thread_id FROM emails WHERE domain_id = $2 AND user_id = $1
			)`, userID, domainID,
		); err != nil {
			return fmt.Errorf("reassign threads: %w", err)
		}

	case "alias":
		name := addr.Name
		if name == "" {
			name = parts[0]
		}
		var aliasID string
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO aliases (org_id, domain_id, address, name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (address) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			orgID, domainID, addr.Address, name,
		).Scan(&aliasID); err != nil {
			return fmt.Errorf("upsert alias: %w", err)
		}
		if _, err := tx.Exec(dbCtx,
			`UPDATE discovered_addresses SET type = 'alias', alias_id = $1
			 WHERE domain_id = $2 AND address = $3`,
			aliasID, domainID, addr.Address,
		); err != nil {
			return fmt.Errorf("update discovered_addresses: %w", err)
		}

	case "skip":
		if _, err := tx.Exec(dbCtx,
			`UPDATE discovered_addresses SET type = 'unclaimed'
			 WHERE domain_id = $1 AND address = $2`,
			domainID, addr.Address,
		); err != nil {
			return fmt.Errorf("mark unclaimed: %w", err)
		}
	}

	return tx.Commit(dbCtx)
}

func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	ctx := r.Context()
	_, err := h.DB.Exec(ctx,
		"UPDATE orgs SET onboarding_completed = true, updated_at = now() WHERE id = $1",
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete onboarding")
		return
	}

	// Get first domain for redirect
	var firstDomainID string
	warnIfErr(h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		claims.OrgID,
	).Scan(&firstDomainID), "onboarding: failed to look up first domain", "org_id", claims.OrgID)

	slog.Info("onboarding: completed", "org_id", claims.OrgID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":         "onboarding completed",
		"first_domain_id": firstDomainID,
	})
}
