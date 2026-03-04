package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DomainHeartbeat struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	Bus       *event.Bus
	Interval  time.Duration
}

func NewDomainHeartbeat(db *pgxpool.Pool, resendSvc *service.ResendService, bus *event.Bus, interval time.Duration) *DomainHeartbeat {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &DomainHeartbeat{
		DB:        db,
		ResendSvc: resendSvc,
		Bus:       bus,
		Interval:  interval,
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
		`SELECT id FROM orgs WHERE resend_api_key_encrypted IS NOT NULL`)
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
		// 403 = API key revoked. Mark all domains disconnected.
		if ok := isResendErr(err, &resendErr); ok && resendErr.StatusCode == 403 {
			tag, err := dh.DB.Exec(ctx,
				`UPDATE domains SET status = 'disconnected', updated_at = now()
				 WHERE org_id = $1 AND status != 'disconnected'`, orgID)
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

	var resendResp struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Records []struct {
				Type   string `json:"record"`
				Name   string `json:"name"`
				Value  string `json:"value"`
				Status string `json:"status"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resendResp); err != nil {
		slog.Error("domain heartbeat: failed to parse response", "org_id", orgID, "error", err)
		return
	}

	// Build map of domain info from Resend
	type resendDomainInfo struct {
		status string
		spf    bool
		dkim   bool
	}
	resendDomains := make(map[string]resendDomainInfo, len(resendResp.Data))
	for _, d := range resendResp.Data {
		info := resendDomainInfo{status: d.Status}
		for _, rec := range d.Records {
			if rec.Status == "verified" || rec.Status == "success" {
				switch rec.Type {
				case "SPF", "MX":
					info.spf = true
				case "DKIM":
					info.dkim = true
				}
			}
		}
		resendDomains[d.Name] = info
	}

	// Check local domains against Resend
	localRows, err := dh.DB.Query(ctx,
		`SELECT id, domain, status, COALESCE(spf_verified, false), COALESCE(dkim_verified, false) FROM domains WHERE org_id = $1`, orgID)
	if err != nil {
		slog.Error("domain heartbeat: failed to list local domains", "org_id", orgID, "error", err)
		return
	}
	defer localRows.Close()

	localDomainNames := make(map[string]bool)
	for localRows.Next() {
		var id, domain, status string
		var localSPF, localDKIM bool
		if localRows.Scan(&id, &domain, &status, &localSPF, &localDKIM) != nil {
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
			// Domain reappeared in Resend — mark active (self-healing)
			if _, err := dh.DB.Exec(ctx,
				`UPDATE domains SET status = 'active', updated_at = now() WHERE id = $1`, id); err != nil {
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

		// PRD-075: Check DNS verification status changes
		if inResend {
			dnsChanged := false
			var degraded []string
			if info.spf != localSPF {
				dnsChanged = true
				if !info.spf && localSPF {
					degraded = append(degraded, "SPF")
				}
			}
			if info.dkim != localDKIM {
				dnsChanged = true
				if !info.dkim && localDKIM {
					degraded = append(degraded, "DKIM")
				}
			}
			if dnsChanged {
				if _, err := dh.DB.Exec(ctx,
					`UPDATE domains SET spf_verified = $2, dkim_verified = $3, updated_at = now() WHERE id = $1`,
					id, info.spf, info.dkim); err != nil {
					slog.Error("heartbeat: failed to update DNS status", "error", err, "domain_id", id)
				}
				if len(degraded) > 0 {
					slog.Warn("domain heartbeat: DNS verification degraded",
						"org_id", orgID, "domain_id", id, "domain", domain, "degraded", degraded)
					dh.Bus.Publish(ctx, event.Event{
						EventType: event.DomainDNSDegraded,
						OrgID:     orgID,
						DomainID:  id,
						Payload: map[string]interface{}{
							"domain":   domain,
							"degraded": degraded,
						},
					})
				}
			}
		}
	}

	// Detect Resend domains not in our DB
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

func isResendErr(err error, target **service.ResendError) bool {
	var re *service.ResendError
	if ok := errors.As(err, &re); ok {
		*target = re
		return true
	}
	return false
}
