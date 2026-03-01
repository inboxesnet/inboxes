package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DomainHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	EncSvc    *service.EncryptionService
	PublicURL string
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, org_id, domain, resend_domain_id, status,
		        display_order, dns_records, created_at
		 FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order, created_at`, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	defer rows.Close()

	var domains []map[string]interface{}
	for rows.Next() {
		var id, orgID, domain string
		var resendDomainID *string
		var status string
		var displayOrder int
		var dnsRecords json.RawMessage
		var createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&displayOrder, &dnsRecords, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":               id,
			"org_id":           orgID,
			"domain":           domain,
			"resend_domain_id": resendDomainID,
			"status":           status,
			"display_order":    displayOrder,
			"dns_records":      dnsRecords,
			"created_at":       createdAt,
		}
		domains = append(domains, d)
	}

	if domains == nil {
		domains = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, org_id, domain, resend_domain_id, status,
		        display_order, dns_records, hidden, created_at
		 FROM domains WHERE org_id = $1 ORDER BY display_order, created_at`, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	defer rows.Close()

	var domains []map[string]interface{}
	for rows.Next() {
		var id, orgID, domain string
		var resendDomainID *string
		var status string
		var hidden bool
		var displayOrder int
		var dnsRecords json.RawMessage
		var createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&displayOrder, &dnsRecords, &hidden, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":               id,
			"org_id":           orgID,
			"domain":           domain,
			"resend_domain_id": resendDomainID,
			"status":           status,
			"display_order":    displayOrder,
			"dns_records":      dnsRecords,
			"hidden":           hidden,
			"created_at":       createdAt,
		}
		domains = append(domains, d)
	}

	if domains == nil {
		domains = []map[string]interface{}{}
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

	var domainID string
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO domains (org_id, domain, resend_domain_id, status, dns_records)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		claims.OrgID, req.Domain, resendDomain.ID, "pending", resendDomain.Records,
	).Scan(&domainID)
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

	var resendDomainID string
	err := h.DB.QueryRow(r.Context(),
		`SELECT resend_domain_id FROM domains WHERE id = $1 AND org_id = $2`,
		domainID, claims.OrgID).Scan(&resendDomainID)
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
	if _, err := h.DB.Exec(r.Context(),
		`UPDATE domains SET status = $1, dns_records = $2, updated_at = now() WHERE id = $3`,
		result.Status, result.Records, domainID); err != nil {
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

	dbCtx, dbCancel := util.DBCtx(r.Context())
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtx)

	for _, item := range req.Order {
		if _, err := tx.Exec(dbCtx,
			`UPDATE domains SET display_order = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
			item.Order, item.ID, claims.OrgID); err != nil {
			slog.Error("domain: reorder update failed", "domain_id", item.ID, "error", err)
		}
	}

	if err := tx.Commit(dbCtx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save order")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) UnreadCounts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT t.domain_id, COALESCE(SUM(t.unread_count), 0)
		 FROM threads t
		 JOIN thread_labels tl ON tl.thread_id = t.id
		 JOIN domains d ON d.id = t.domain_id
		 WHERE d.org_id = $1 AND t.user_id = $2 AND tl.label = 'inbox' AND t.deleted_at IS NULL
		 AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))
		 GROUP BY t.domain_id`, claims.OrgID, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get unread counts")
		return
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var domainID string
		var count int
		if rows.Scan(&domainID, &count) == nil {
			counts[domainID] = count
		}
	}

	writeJSON(w, http.StatusOK, counts)
}

func (h *DomainHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

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

	dbCtxV, dbCancelV := util.DBCtx(r.Context())
	defer dbCancelV()

	tx, err := h.DB.Begin(dbCtxV)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtxV)

	_, err = tx.Exec(dbCtxV,
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	if len(req.Visible) > 0 {
		_, err = tx.Exec(dbCtxV,
			`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
			claims.OrgID, req.Visible)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update domains")
			return
		}
	}

	if err := tx.Commit(dbCtxV); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save visibility")
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

	// Build set of Resend domain names for disconnect detection
	resendDomainNames := make(map[string]bool, len(resendResp.Data))
	for _, rd := range resendResp.Data {
		resendDomainNames[rd.Name] = true
		if _, err := h.DB.Exec(r.Context(),
			`INSERT INTO domains (org_id, domain, resend_domain_id, status, hidden)
			 VALUES ($1, $2, $3, $4, true)
			 ON CONFLICT (domain) WHERE status NOT IN ('deleted') DO UPDATE SET status = EXCLUDED.status, updated_at = now()`,
			claims.OrgID, rd.Name, rd.ID, rd.Status); err != nil {
			slog.Error("domain: sync upsert failed", "domain", rd.Name, "error", err)
		}
	}

	// Mark local domains not in Resend response as disconnected
	localRows, err := h.DB.Query(r.Context(),
		`SELECT id, domain, status FROM domains WHERE org_id = $1`, claims.OrgID)
	if err == nil {
		defer localRows.Close()
		for localRows.Next() {
			var localID, localDomain, localStatus string
			if localRows.Scan(&localID, &localDomain, &localStatus) == nil {
				if !resendDomainNames[localDomain] && localStatus != "disconnected" {
					if _, err := h.DB.Exec(r.Context(),
						`UPDATE domains SET status = 'disconnected', updated_at = now() WHERE id = $1`,
						localID); err != nil {
						slog.Error("domain: disconnect update failed", "domain_id", localID, "error", err)
					}
					slog.Info("domain: marked as disconnected", "domain_id", localID, "domain", localDomain)
				}
			}
		}
		localRows.Close()
	}

	// Return full domain list (including hidden) so frontend can update without a second fetch
	rows, err := h.DB.Query(r.Context(),
		`SELECT id, org_id, domain, resend_domain_id, status,
		        display_order, dns_records, hidden, created_at
		 FROM domains WHERE org_id = $1 ORDER BY display_order, created_at`, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains after sync")
		return
	}
	defer rows.Close()

	var domains []map[string]interface{}
	for rows.Next() {
		var id, orgID, domain string
		var resendDomainID *string
		var status string
		var hidden bool
		var displayOrder int
		var dnsRecords json.RawMessage
		var createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&displayOrder, &dnsRecords, &hidden, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":               id,
			"org_id":           orgID,
			"domain":           domain,
			"resend_domain_id": resendDomainID,
			"status":           status,
			"display_order":    displayOrder,
			"dns_records":      dnsRecords,
			"hidden":           hidden,
			"created_at":       createdAt,
		}
		domains = append(domains, d)
	}

	if domains == nil {
		domains = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	domainID := chi.URLParam(r, "id")
	ctx := r.Context()

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE domains SET status = 'deleted', hidden = true, updated_at = now() WHERE id = $1 AND org_id = $2`,
		domainID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}

	// Cascade soft-delete to child entities
	if _, err := tx.Exec(ctx, `UPDATE aliases SET deleted_at = now() WHERE domain_id = $1 AND deleted_at IS NULL`, domainID); err != nil {
		slog.Error("domain: cascade delete aliases failed", "domain_id", domainID, "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM alias_users WHERE alias_id IN (SELECT id FROM aliases WHERE domain_id = $1)`, domainID); err != nil {
		slog.Error("domain: cascade delete alias_users failed", "domain_id", domainID, "error", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE threads SET deleted_at = now(), updated_at = now() WHERE domain_id = $1 AND deleted_at IS NULL`, domainID); err != nil {
		slog.Error("domain: cascade delete threads failed", "domain_id", domainID, "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM discovered_addresses WHERE domain_id = $1`, domainID); err != nil {
		slog.Error("domain: cascade delete discovered_addresses failed", "domain_id", domainID, "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}

	slog.Info("domain: soft-deleted with cascade", "domain_id", domainID, "admin", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) ReregisterWebhook(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

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
		Secret string `json:"secret"`
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

	_, err = h.DB.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1,
		 resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = $2, resend_webhook_secret_iv = $3, resend_webhook_secret_tag = $4,
		 updated_at = now() WHERE id = $5`,
		webhookResp.ID, encSecret, encIV, encTag, claims.OrgID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store webhook config")
		return
	}

	slog.Info("domain: webhook re-registered", "org_id", claims.OrgID, "webhook_id", webhookResp.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_id":  webhookResp.ID,
		"webhook_url": webhookURL,
	})
}
