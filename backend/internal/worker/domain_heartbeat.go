package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DomainHeartbeat struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	EncSvc    *service.EncryptionService
	Bus       *event.Bus
	Interval  time.Duration
	PublicURL string
}

func NewDomainHeartbeat(db *pgxpool.Pool, resendSvc *service.ResendService, encSvc *service.EncryptionService, bus *event.Bus, interval time.Duration, publicURL string) *DomainHeartbeat {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &DomainHeartbeat{
		DB:        db,
		ResendSvc: resendSvc,
		EncSvc:    encSvc,
		Bus:       bus,
		Interval:  interval,
		PublicURL: publicURL,
	}
}

func (dh *DomainHeartbeat) Run(ctx context.Context) {
	slog.Info("domain heartbeat: starting", "interval", dh.Interval)

	// Run once on startup after a short delay (let migrations finish)
	select {
	case <-time.After(30 * time.Second):
		func() {
			defer util.RecoverWorker("domain-heartbeat")
			dh.checkAll(ctx)
		}()
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(dh.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				defer util.RecoverWorker("domain-heartbeat")
				dh.checkAll(ctx)
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (dh *DomainHeartbeat) checkAll(ctx context.Context) {
	// Get all orgs that have an API key configured
	rows, err := dh.DB.Query(ctx,
		`SELECT id FROM orgs WHERE resend_api_key_encrypted IS NOT NULL AND deleted_at IS NULL`)
	if err != nil {
		slog.Error("domain heartbeat: failed to list orgs", "error", err)
		return
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var orgID string
		if rows.Scan(&orgID) == nil {
			orgIDs = append(orgIDs, orgID)
		}
	}
	rows.Close()

	for _, orgID := range orgIDs {
		dh.checkOrg(ctx, orgID)
	}

	dh.repairWebhooks(ctx)
}

// repairWebhooks registers a Resend webhook for orgs that have an API key
// but no stored webhook secret. This covers the onboarding path where the
// webhook step was skipped or its call failed silently. Localhost public
// URLs are skipped: Resend cannot reach them.
func (dh *DomainHeartbeat) repairWebhooks(ctx context.Context) {
	if dh.EncSvc == nil || dh.PublicURL == "" ||
		strings.Contains(dh.PublicURL, "localhost") || strings.Contains(dh.PublicURL, "127.0.0.1") {
		return
	}

	rows, err := dh.DB.Query(ctx,
		`SELECT id FROM orgs
		 WHERE resend_api_key_encrypted IS NOT NULL AND deleted_at IS NULL
		 AND api_key_status != 'invalid'
		 AND (resend_webhook_secret_encrypted IS NULL OR resend_webhook_secret_encrypted = '')`)
	if err != nil {
		slog.Error("domain heartbeat: failed to list orgs for webhook repair", "error", err)
		return
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var orgID string
		if rows.Scan(&orgID) == nil {
			orgIDs = append(orgIDs, orgID)
		}
	}
	rows.Close()

	for _, orgID := range orgIDs {
		dh.repairOrgWebhook(ctx, orgID)
	}
}

func (dh *DomainHeartbeat) repairOrgWebhook(ctx context.Context, orgID string) {
	webhookURL := dh.PublicURL + "/api/webhooks/resend/" + orgID

	data, err := dh.ResendSvc.Fetch(ctx, orgID, "POST", "/webhooks", map[string]interface{}{
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
		slog.Warn("domain heartbeat: webhook auto-repair failed", "org_id", orgID, "error", err)
		return
	}

	var webhookResp struct {
		ID     string `json:"id"`
		Secret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(data, &webhookResp); err != nil || webhookResp.Secret == "" {
		slog.Warn("domain heartbeat: webhook auto-repair parse failed", "org_id", orgID, "error", err)
		return
	}

	encSecret, encIV, encTag, err := dh.EncSvc.Encrypt(webhookResp.Secret)
	if err != nil {
		slog.Error("domain heartbeat: webhook secret encrypt failed", "org_id", orgID, "error", err)
		return
	}

	if _, err := dh.DB.Exec(ctx,
		`UPDATE orgs SET resend_webhook_id = $1,
		 resend_webhook_secret = NULL,
		 resend_webhook_secret_encrypted = $2, resend_webhook_secret_iv = $3, resend_webhook_secret_tag = $4,
		 updated_at = now() WHERE id = $5`,
		webhookResp.ID, encSecret, encIV, encTag, orgID); err != nil {
		slog.Error("domain heartbeat: webhook auto-repair store failed", "org_id", orgID, "error", err)
		return
	}

	slog.Info("domain heartbeat: webhook auto-repaired", "org_id", orgID, "webhook_id", webhookResp.ID)
}

func (dh *DomainHeartbeat) checkOrg(ctx context.Context, orgID string) {
	// Fetch domains from Resend
	respBytes, err := dh.ResendSvc.Fetch(ctx, orgID, "GET", "/domains", nil)
	if err != nil {
		// Don't mark anything disconnected on API failure — could be transient
		var resendErr *service.ResendError
		if ok := isResendErr(err, &resendErr); ok && resendErr.IsRetryable() {
			slog.Warn("domain heartbeat: transient error fetching domains, skipping org",
				"org_id", orgID, "status", resendErr.StatusCode)
			return
		}
		// 401/403 = API key revoked or invalid. Mark all domains disconnected
		// and record the key status so the UI can show it.
		if ok := isResendErr(err, &resendErr); ok && (resendErr.StatusCode == 401 || resendErr.StatusCode == 403) {
			dh.setAPIKeyStatus(ctx, orgID, "invalid")
			tag, err := dh.DB.Exec(ctx,
				`UPDATE domains SET status = 'disconnected', updated_at = now()
				 WHERE org_id = $1 AND status NOT IN ('disconnected', 'deleted')`, orgID)
			if err != nil {
				slog.Error("heartbeat: failed to mark domains disconnected", "error", err, "org_id", orgID)
			} else if tag.RowsAffected() > 0 {
				slog.Warn("domain heartbeat: API key invalid, all domains disconnected",
					"org_id", orgID, "count", tag.RowsAffected())
				dh.Bus.Publish(ctx, event.Event{
					EventType: event.DomainDisconnected,
					OrgID:     orgID,
					Payload: map[string]interface{}{
						"reason": "api_key_revoked",
					},
				})
			}
			return
		}
		slog.Warn("domain heartbeat: failed to fetch domains", "org_id", orgID, "error", err)
		return
	}

	// The key works — record that.
	dh.setAPIKeyStatus(ctx, orgID, "valid")

	// The list endpoint returns id/name/status only. It carries NO records
	// array — DNS records come from the per-domain detail endpoint. The DNS
	// check below must never read record state from this response.
	var resendResp struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resendResp); err != nil {
		slog.Error("domain heartbeat: failed to parse response", "org_id", orgID, "error", err)
		return
	}

	// Build map of domain info from Resend
	type resendDomainInfo struct {
		id     string
		status string
	}
	resendDomains := make(map[string]resendDomainInfo, len(resendResp.Data))
	for _, d := range resendResp.Data {
		resendDomains[d.Name] = resendDomainInfo{id: d.ID, status: d.Status}
	}

	// Check local domains against Resend. Deleted domains stay deleted —
	// the heartbeat must never resurrect them.
	localRows, err := dh.DB.Query(ctx,
		`SELECT id, domain, status, dns_ok FROM domains WHERE org_id = $1 AND status != 'deleted'`, orgID)
	if err != nil {
		slog.Error("domain heartbeat: failed to list local domains", "org_id", orgID, "error", err)
		return
	}
	defer localRows.Close()

	localDomainNames := make(map[string]bool)
	for localRows.Next() {
		var id, domain, status string
		var dnsOK bool
		if localRows.Scan(&id, &domain, &status, &dnsOK) != nil {
			continue
		}
		localDomainNames[domain] = true

		info, inResend := resendDomains[domain]

		if !inResend && status != "disconnected" {
			// Domain gone from Resend — mark disconnected
			if _, err := dh.DB.Exec(ctx,
				`UPDATE domains SET status = 'disconnected', updated_at = now() WHERE id = $1`, id); err != nil {
				slog.Error("heartbeat: failed to disconnect domain", "error", err, "domain_id", id)
			}
			slog.Warn("domain heartbeat: domain disconnected",
				"org_id", orgID, "domain_id", id, "domain", domain)
			dh.Bus.Publish(ctx, event.Event{
				EventType: event.DomainDisconnected,
				OrgID:     orgID,
				DomainID:  id,
				Payload: map[string]interface{}{
					"domain": domain,
				},
			})
		} else if inResend && status == "disconnected" {
			// Domain reappeared in Resend — mark it with the status Resend
			// reports (self-healing). Only a verified domain becomes active.
			newStatus := "pending"
			if info.status == "verified" || info.status == "active" {
				newStatus = "active"
			}
			if _, err := dh.DB.Exec(ctx,
				`UPDATE domains SET status = $1, updated_at = now() WHERE id = $2`, newStatus, id); err != nil {
				slog.Error("heartbeat: failed to reconnect domain", "error", err, "domain_id", id)
			}
			slog.Info("domain heartbeat: domain reconnected",
				"org_id", orgID, "domain_id", id, "domain", domain)
			dh.Bus.Publish(ctx, event.Event{
				EventType: event.DomainReconnected,
				OrgID:     orgID,
				DomainID:  id,
				Payload: map[string]interface{}{
					"domain": domain,
				},
			})
		}

		// DNS verification needs the per-domain detail endpoint — the list
		// response carries no records. Only a previously verified domain is
		// checked: a pending domain has incomplete DNS by definition.
		if inResend && (status == "active" || status == "verified") {
			dh.checkDomainDNS(ctx, orgID, id, domain, info.id, dnsOK)
		}
	}

	// Detect Resend domains not in our DB
	dh.detectUnknownDomains(ctx, orgID, resendDomainNamesOnly(resendDomains), localDomainNames)
}

func resendDomainNamesOnly[V any](m map[string]V) map[string]bool {
	names := make(map[string]bool, len(m))
	for name := range m {
		names[name] = true
	}
	return names
}

// checkDomainDNS fetches one domain's records from the Resend detail endpoint
// and verifies SPF and DKIM. Two hard rules: a response without record data
// gives no verdict (absent data must never alert), and the alert fires only
// on the ok -> degraded transition, tracked in domains.dns_ok.
func (dh *DomainHeartbeat) checkDomainDNS(ctx context.Context, orgID, domainID, domain, resendID string, dnsOK bool) {
	if resendID == "" {
		return
	}
	respBytes, err := dh.ResendSvc.Fetch(ctx, orgID, "GET", "/domains/"+resendID, nil)
	if err != nil {
		slog.Warn("domain heartbeat: domain detail fetch failed, skipping DNS check",
			"org_id", orgID, "domain", domain, "error", err)
		return
	}
	var detail struct {
		Records []struct {
			Type   string `json:"record"`
			Status string `json:"status"`
		} `json:"records"`
	}
	if err := json.Unmarshal(respBytes, &detail); err != nil || len(detail.Records) == 0 {
		slog.Warn("domain heartbeat: no record data in domain detail, skipping DNS check",
			"org_id", orgID, "domain", domain)
		return
	}

	var spf, dkim bool
	for _, rec := range detail.Records {
		if rec.Status == "verified" || rec.Status == "success" {
			switch rec.Type {
			case "SPF", "MX":
				spf = true
			case "DKIM":
				dkim = true
			}
		}
	}
	var degraded []string
	if !spf {
		degraded = append(degraded, "SPF")
	}
	if !dkim {
		degraded = append(degraded, "DKIM")
	}

	if len(degraded) == 0 {
		if !dnsOK {
			if _, err := dh.DB.Exec(ctx,
				`UPDATE domains SET dns_ok = true, updated_at = now() WHERE id = $1`, domainID); err != nil {
				slog.Error("heartbeat: failed to store dns_ok", "error", err, "domain_id", domainID)
			}
			slog.Info("domain heartbeat: DNS verification recovered", "org_id", orgID, "domain", domain)
		}
		return
	}

	if !dnsOK {
		// Already alerted for this degradation — stay quiet until it recovers.
		return
	}
	if _, err := dh.DB.Exec(ctx,
		`UPDATE domains SET dns_ok = false, updated_at = now() WHERE id = $1`, domainID); err != nil {
		slog.Error("heartbeat: failed to store dns_ok", "error", err, "domain_id", domainID)
	}
	slog.Warn("domain heartbeat: DNS verification missing",
		"org_id", orgID, "domain_id", domainID, "domain", domain, "missing", degraded)
	dh.Bus.Publish(ctx, event.Event{
		EventType: event.DomainDnsDegraded,
		OrgID:     orgID,
		DomainID:  domainID,
		Payload: map[string]interface{}{
			"domain":   domain,
			"degraded": degraded,
		},
	})
}

func (dh *DomainHeartbeat) detectUnknownDomains(ctx context.Context, orgID string, resendDomains map[string]bool, localDomainNames map[string]bool) {
	for resendDomain := range resendDomains {
		if localDomainNames[resendDomain] {
			continue
		}
		var discoveredID string
		err := dh.DB.QueryRow(ctx,
			`INSERT INTO discovered_domains (org_id, domain)
			 VALUES ($1, $2)
			 ON CONFLICT (org_id, domain) DO NOTHING
			 RETURNING id`,
			orgID, resendDomain,
		).Scan(&discoveredID)
		if err == nil && discoveredID != "" {
			slog.Info("domain heartbeat: discovered unknown Resend domain",
				"org_id", orgID, "domain", resendDomain)
			dh.Bus.Publish(ctx, event.Event{
				EventType: event.DomainDiscovered,
				OrgID:     orgID,
				Payload:   map[string]interface{}{"domain": resendDomain},
			})
		}
	}

	// Clean up: remove discovered domains that have since been added locally
	dh.DB.Exec(ctx,
		`DELETE FROM discovered_domains
		 WHERE org_id = $1 AND domain IN (
		     SELECT d2.domain FROM domains d2 WHERE d2.org_id = $1 AND d2.status != 'deleted'
		 )`,
		orgID,
	)
}

// setAPIKeyStatus records the Resend key health on the org. Only changed
// values are written, to keep updated_at meaningful.
func (dh *DomainHeartbeat) setAPIKeyStatus(ctx context.Context, orgID, status string) {
	if _, err := dh.DB.Exec(ctx,
		`UPDATE orgs SET api_key_status = $1, api_key_checked_at = now() WHERE id = $2`,
		status, orgID); err != nil {
		slog.Error("heartbeat: failed to update api key status", "error", err, "org_id", orgID)
	}
}

func isResendErr(err error, target **service.ResendError) bool {
	var re *service.ResendError
	if ok := errors.As(err, &re); ok {
		*target = re
		return true
	}
	return false
}
