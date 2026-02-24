package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DomainHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, org_id, domain, resend_domain_id, status, mx_verified, spf_verified, dkim_verified,
		        catch_all_enabled, display_order, dns_records, verified_at, created_at
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
		var mxVerified, spfVerified, dkimVerified, catchAll bool
		var displayOrder int
		var dnsRecords json.RawMessage
		var verifiedAt, createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&mxVerified, &spfVerified, &dkimVerified, &catchAll, &displayOrder,
			&dnsRecords, &verifiedAt, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":               id,
			"org_id":           orgID,
			"domain":           domain,
			"resend_domain_id": resendDomainID,
			"status":           status,
			"mx_verified":      mxVerified,
			"spf_verified":     spfVerified,
			"dkim_verified":    dkimVerified,
			"catch_all_enabled": catchAll,
			"display_order":    displayOrder,
			"dns_records":      dnsRecords,
			"verified_at":      verifiedAt,
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
		`SELECT id, org_id, domain, resend_domain_id, status, mx_verified, spf_verified, dkim_verified,
		        catch_all_enabled, display_order, dns_records, hidden, verified_at, created_at
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
		var mxVerified, spfVerified, dkimVerified, catchAll, hidden bool
		var displayOrder int
		var dnsRecords json.RawMessage
		var verifiedAt, createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&mxVerified, &spfVerified, &dkimVerified, &catchAll, &displayOrder,
			&dnsRecords, &hidden, &verifiedAt, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":                id,
			"org_id":            orgID,
			"domain":            domain,
			"resend_domain_id":  resendDomainID,
			"status":            status,
			"mx_verified":       mxVerified,
			"spf_verified":      spfVerified,
			"dkim_verified":     dkimVerified,
			"catch_all_enabled": catchAll,
			"display_order":     displayOrder,
			"dns_records":       dnsRecords,
			"hidden":            hidden,
			"verified_at":       verifiedAt,
			"created_at":        createdAt,
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

	// Create domain via Resend API
	body := map[string]string{"name": req.Domain}
	bodyBytes, _ := json.Marshal(body)
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
	json.Unmarshal(respBytes, &resendDomain)

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
	json.Unmarshal(respBytes, &result)

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

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())

	for _, item := range req.Order {
		tx.Exec(r.Context(),
			`UPDATE domains SET display_order = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
			item.Order, item.ID, claims.OrgID)
	}

	if err := tx.Commit(r.Context()); err != nil {
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
		 JOIN domains d ON d.id = t.domain_id
		 WHERE d.org_id = $1 AND t.user_id = $2 AND t.folder = 'inbox' AND t.deleted_at IS NULL
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

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE domains SET hidden = true, updated_at = now() WHERE org_id = $1`, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domains")
		return
	}

	if len(req.Visible) > 0 {
		_, err = tx.Exec(r.Context(),
			`UPDATE domains SET hidden = false, updated_at = now() WHERE org_id = $1 AND id = ANY($2)`,
			claims.OrgID, req.Visible)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update domains")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
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
	json.Unmarshal(respBytes, &resendResp)

	for _, rd := range resendResp.Data {
		h.DB.Exec(r.Context(),
			`INSERT INTO domains (org_id, domain, resend_domain_id, status, hidden)
			 VALUES ($1, $2, $3, $4, true)
			 ON CONFLICT (domain) DO UPDATE SET status = EXCLUDED.status, updated_at = now()`,
			claims.OrgID, rd.Name, rd.ID, rd.Status)
	}

	// Return full domain list (including hidden) so frontend can update without a second fetch
	rows, err := h.DB.Query(r.Context(),
		`SELECT id, org_id, domain, resend_domain_id, status, mx_verified, spf_verified, dkim_verified,
		        catch_all_enabled, display_order, dns_records, hidden, verified_at, created_at
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
		var mxVerified, spfVerified, dkimVerified, catchAll, hidden bool
		var displayOrder int
		var dnsRecords json.RawMessage
		var verifiedAt, createdAt interface{}

		if err := rows.Scan(&id, &orgID, &domain, &resendDomainID, &status,
			&mxVerified, &spfVerified, &dkimVerified, &catchAll, &displayOrder,
			&dnsRecords, &hidden, &verifiedAt, &createdAt); err != nil {
			continue
		}

		d := map[string]interface{}{
			"id":                id,
			"org_id":            orgID,
			"domain":            domain,
			"resend_domain_id":  resendDomainID,
			"status":            status,
			"mx_verified":       mxVerified,
			"spf_verified":      spfVerified,
			"dkim_verified":     dkimVerified,
			"catch_all_enabled": catchAll,
			"display_order":     displayOrder,
			"dns_records":       dnsRecords,
			"hidden":            hidden,
			"verified_at":       verifiedAt,
			"created_at":        createdAt,
		}
		domains = append(domains, d)
	}

	if domains == nil {
		domains = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, domains)
}
