package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
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
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()

	// Check if API key is stored
	var hasAPIKey bool
	h.DB.QueryRow(ctx,
		"SELECT (resend_api_key_encrypted IS NOT NULL) FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&hasAPIKey)

	if !hasAPIKey {
		writeJSON(w, http.StatusOK, map[string]string{"step": "connect"})
		return
	}

	// Check if domains exist (non-hidden)
	var domainCount int
	h.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM domains WHERE org_id = $1 AND hidden = false", claims.OrgID,
	).Scan(&domainCount)

	if domainCount == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"step": "domains"})
		return
	}

	// Check if emails have been imported
	var emailCount int
	h.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM emails WHERE org_id = $1", claims.OrgID,
	).Scan(&emailCount)

	if emailCount == 0 {
		// Check if a sync is already in progress
		var syncJobID string
		err := h.DB.QueryRow(ctx,
			`SELECT id FROM sync_jobs WHERE org_id = $1 AND status IN ('pending', 'running')
			 ORDER BY created_at DESC LIMIT 1`, claims.OrgID,
		).Scan(&syncJobID)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"step":             "sync",
				"sync_in_progress": true,
				"sync_job_id":      syncJobID,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"step": "sync"})
		return
	}

	// Emails exist — go to addresses step
	writeJSON(w, http.StatusOK, map[string]string{"step": "addresses"})
}

func (h *OnboardingHandler) Connect(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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
	_, err = h.DB.Exec(ctx,
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
		err := h.DB.QueryRow(ctx,
			`INSERT INTO domains (org_id, domain, resend_domain_id, status, region, dns_records, display_order)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (domain) DO UPDATE SET resend_domain_id = $3, status = $4, region = $5, dns_records = $6, updated_at = now()
			 RETURNING id`,
			claims.OrgID, d.Name, d.ID, status, d.Region, d.Records, i,
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"domains": domains,
	})
}

func (h *OnboardingHandler) SelectDomains(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		DomainIDs []string `json:"domain_ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Hide all, then unhide selected
	_, err := h.DB.Exec(ctx,
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}
	_, err = h.DB.Exec(ctx,
		`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
		claims.OrgID, req.DomainIDs,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "domains updated"})
}

func (h *OnboardingHandler) SetupWebhook(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()

	webhookURL := h.PublicURL + "/api/webhooks/resend/" + claims.OrgID

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
		writeError(w, http.StatusInternalServerError, "failed to register webhook: "+err.Error())
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

	_, err = h.DB.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1, resend_webhook_secret = $2, updated_at = now() WHERE id = $3`,
		webhookResp.ID, webhookResp.Secret, claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store webhook config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_id":  webhookResp.ID,
		"webhook_url": webhookURL,
	})
}

func (h *OnboardingHandler) GetAddresses(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()
	rows, err := h.DB.Query(ctx,
		`SELECT da.id, da.address, da.local_part, da.type, da.email_count
		 FROM discovered_addresses da
		 JOIN domains d ON d.id = da.domain_id
		 WHERE d.org_id = $1 AND d.hidden = false
		 ORDER BY da.email_count DESC`,
		claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch addresses")
		return
	}
	defer rows.Close()

	var addresses []map[string]interface{}
	for rows.Next() {
		var id, address, localPart, addrType string
		var emailCount int
		rows.Scan(&id, &address, &localPart, &addrType, &emailCount)
		addresses = append(addresses, map[string]interface{}{
			"id":          id,
			"address":     address,
			"local_part":  localPart,
			"type":        addrType,
			"email_count": emailCount,
		})
	}
	if addresses == nil {
		addresses = []map[string]interface{}{}
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
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req setupAddressesRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	for _, addr := range req.Addresses {
		parts := strings.Split(addr.Address, "@")
		if len(parts) != 2 {
			continue
		}
		domain := parts[1]

		var domainID string
		err := h.DB.QueryRow(ctx,
			"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
			claims.OrgID, domain,
		).Scan(&domainID)
		if err != nil {
			continue
		}

		switch addr.Type {
		case "person":
			// Create placeholder user
			name := addr.Name
			if name == "" {
				name = parts[0]
			}
			var userID string
			err := h.DB.QueryRow(ctx,
				`INSERT INTO users (org_id, email, name, status)
				 VALUES ($1, $2, $3, 'placeholder')
				 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
				 RETURNING id`,
				claims.OrgID, addr.Address, name,
			).Scan(&userID)
			if err != nil {
				continue
			}
			// Update discovered address
			h.DB.Exec(ctx,
				`UPDATE discovered_addresses SET type = 'user', user_id = $1
				 WHERE domain_id = $2 AND address = $3`,
				userID, domainID, addr.Address,
			)
			// Reassign emails from admin to this user
			h.DB.Exec(ctx,
				`UPDATE emails SET user_id = $1 WHERE domain_id = $2 AND from_address = $3`,
				userID, domainID, addr.Address,
			)
			h.DB.Exec(ctx,
				`UPDATE threads SET user_id = $1 WHERE domain_id = $2 AND id IN (
					SELECT DISTINCT thread_id FROM emails WHERE domain_id = $2 AND user_id = $1
				)`, userID, domainID,
			)

		case "alias":
			name := addr.Name
			if name == "" {
				name = parts[0]
			}
			var aliasID string
			h.DB.QueryRow(ctx,
				`INSERT INTO aliases (org_id, domain_id, address, name)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (address) DO UPDATE SET name = EXCLUDED.name
				 RETURNING id`,
				claims.OrgID, domainID, addr.Address, name,
			).Scan(&aliasID)
			if aliasID != "" {
				h.DB.Exec(ctx,
					`UPDATE discovered_addresses SET type = 'alias', alias_id = $1
					 WHERE domain_id = $2 AND address = $3`,
					aliasID, domainID, addr.Address,
				)
			}

		case "skip":
			h.DB.Exec(ctx,
				`UPDATE discovered_addresses SET type = 'unclaimed'
				 WHERE domain_id = $1 AND address = $2`,
				domainID, addr.Address,
			)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "addresses configured"})
}

func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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
	h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		claims.OrgID,
	).Scan(&firstDomainID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":         "onboarding completed",
		"first_domain_id": firstDomainID,
	})
}
