package queue

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"github.com/inboxes/backend/internal/service"
)

// inboundRuleInput carries the facts the rules engine needs about one
// freshly stored inbound email.
type inboundRuleInput struct {
	orgID          string
	matchedAliases []string
	fromAddr       string
	subject        string
	bodyHTML       string
	bodyPlain      string
	isSpam         bool
	isBounce       bool
	senderIsAuto   bool
}

// autoSenderLocalParts never receive forwards or auto-replies: they are
// machine senders and replying to them loops or bounces.
var autoSenderLocalParts = []string{
	"mailer-daemon", "postmaster", "no-reply", "noreply", "donotreply", "do-not-reply", "bounce",
}

func isAutomatedSender(addr string) bool {
	local := strings.ToLower(addr)
	if at := strings.IndexByte(local, '@'); at > 0 {
		local = local[:at]
	}
	for _, p := range autoSenderLocalParts {
		if strings.Contains(local, p) {
			return true
		}
	}
	return false
}

// ApplyInboundRulesForTest exposes the rules engine for integration tests.
func (w *EmailWorker) ApplyInboundRulesForTest(ctx context.Context, orgID string, matchedAliases []string, fromAddr, subject, bodyHTML, bodyPlain string, isSpam, isBounce, senderIsAuto bool) {
	w.applyInboundRules(ctx, inboundRuleInput{
		orgID: orgID, matchedAliases: matchedAliases, fromAddr: fromAddr,
		subject: subject, bodyHTML: bodyHTML, bodyPlain: bodyPlain,
		isSpam: isSpam, isBounce: isBounce, senderIsAuto: senderIsAuto,
	})
}

// applyInboundRules runs forwarding rules and auto-replies for one inbound
// email. It runs after the email commit and never fails the fetch job: every
// error only logs.
func (w *EmailWorker) applyInboundRules(ctx context.Context, in inboundRuleInput) {
	if in.isSpam || in.isBounce || in.senderIsAuto || len(in.matchedAliases) == 0 {
		return
	}
	fromClean := strings.ToLower(strings.TrimSpace(service.ExtractEmail(in.fromAddr)))
	if fromClean == "" || isAutomatedSender(fromClean) {
		return
	}

	var forwardingEnabled, autoReplyEnabled, externalAllowed bool
	if err := w.store.Q().QueryRow(ctx,
		`SELECT forwarding_enabled, auto_reply_enabled, external_forwarding_allowed
		 FROM orgs WHERE id = $1`, in.orgID,
	).Scan(&forwardingEnabled, &autoReplyEnabled, &externalAllowed); err != nil {
		slog.Error("inbound rules: org policy lookup failed", "org_id", in.orgID, "error", err)
		return
	}

	if forwardingEnabled {
		w.applyForwardingRules(ctx, in, fromClean, externalAllowed)
	}
	if autoReplyEnabled {
		w.applyAutoReplies(ctx, in, fromClean)
	}
}

func (w *EmailWorker) applyForwardingRules(ctx context.Context, in inboundRuleInput, fromClean string, externalAllowed bool) {
	rows, err := w.store.Q().Query(ctx,
		`SELECT fr.id, fr.forward_to, a.address
		 FROM forwarding_rules fr
		 JOIN aliases a ON a.id = fr.alias_id AND a.deleted_at IS NULL
		 WHERE fr.org_id = $1 AND fr.enabled AND a.address = ANY($2)`,
		in.orgID, in.matchedAliases,
	)
	if err != nil {
		slog.Error("inbound rules: forwarding query failed", "org_id", in.orgID, "error", err)
		return
	}
	defer rows.Close()

	type fwd struct{ ruleID, target, aliasAddr string }
	var fwds []fwd
	seen := map[string]bool{}
	for rows.Next() {
		var f fwd
		if rows.Scan(&f.ruleID, &f.target, &f.aliasAddr) == nil {
			// One forward per target even when several aliases match.
			key := strings.ToLower(f.target)
			if !seen[key] {
				seen[key] = true
				fwds = append(fwds, f)
			}
		}
	}
	rows.Close()

	for _, f := range fwds {
		target := strings.ToLower(strings.TrimSpace(f.target))
		if target == fromClean {
			// Never forward a message back to its sender: instant loop.
			continue
		}
		if !externalAllowed && !w.isOrgDomainAddress(ctx, in.orgID, target) {
			slog.Warn("inbound rules: external forward blocked by policy", "rule_id", f.ruleID, "target", target)
			continue
		}

		if err := w.limiter.WaitForOrg(ctx, in.orgID); err != nil {
			return
		}
		body := forwardBodyHTML(in.fromAddr, in.subject, in.bodyHTML, in.bodyPlain)
		payload := map[string]interface{}{
			"from":     f.aliasAddr,
			"to":       []string{f.target},
			"subject":  "Fwd: " + in.subject,
			"html":     body,
			"reply_to": in.fromAddr,
			"headers":  map[string]string{"Auto-Submitted": "auto-forwarded"},
		}
		if _, err := w.resendSvc.Fetch(ctx, in.orgID, "POST", "/emails", payload); err != nil {
			slog.Error("inbound rules: forward send failed", "rule_id", f.ruleID, "target", f.target, "error", err)
			continue
		}
		slog.Info("inbound rules: forwarded", "rule_id", f.ruleID, "alias", f.aliasAddr, "target", f.target)
	}
}

func forwardBodyHTML(from, subject, bodyHTML, bodyPlain string) string {
	inner := bodyHTML
	if inner == "" {
		inner = "<pre>" + html.EscapeString(bodyPlain) + "</pre>"
	}
	return fmt.Sprintf(
		`<p>---------- Forwarded message ----------<br>From: %s<br>Subject: %s</p>%s`,
		html.EscapeString(from), html.EscapeString(subject), inner,
	)
}

func (w *EmailWorker) applyAutoReplies(ctx context.Context, in inboundRuleInput, fromClean string) {
	rows, err := w.store.Q().Query(ctx,
		`SELECT ar.id, a.address, ar.subject, ar.body_html, ar.body_plain
		 FROM auto_replies ar
		 JOIN aliases a ON a.id = ar.alias_id AND a.deleted_at IS NULL
		 WHERE ar.org_id = $1 AND ar.enabled AND a.address = ANY($2)
		 AND (ar.starts_at IS NULL OR ar.starts_at <= now())
		 AND (ar.ends_at IS NULL OR ar.ends_at >= now())`,
		in.orgID, in.matchedAliases,
	)
	if err != nil {
		slog.Error("inbound rules: auto-reply query failed", "org_id", in.orgID, "error", err)
		return
	}
	defer rows.Close()

	type reply struct{ id, aliasAddr, subject, bodyHTML, bodyPlain string }
	var replies []reply
	for rows.Next() {
		var r reply
		if rows.Scan(&r.id, &r.aliasAddr, &r.subject, &r.bodyHTML, &r.bodyPlain) == nil {
			replies = append(replies, r)
		}
	}
	rows.Close()

	for _, r := range replies {
		// Rate limit: one reply per sender per rule per 24 hours. The upsert
		// wins only when there is no fresh log row.
		var sentAt *string
		err := w.store.Q().QueryRow(ctx,
			`INSERT INTO auto_reply_log (auto_reply_id, sender, sent_at) VALUES ($1, $2, now())
			 ON CONFLICT (auto_reply_id, sender) DO UPDATE SET sent_at = now()
			 WHERE auto_reply_log.sent_at < now() - interval '24 hours'
			 RETURNING sent_at::text`,
			r.id, fromClean,
		).Scan(&sentAt)
		if err != nil {
			// No row returned: this sender already got a reply in the window.
			continue
		}

		if err := w.limiter.WaitForOrg(ctx, in.orgID); err != nil {
			return
		}
		subject := r.subject
		if subject == "" {
			subject = "Re: " + in.subject
		}
		bodyHTML := r.bodyHTML
		if bodyHTML == "" {
			bodyHTML = "<pre>" + html.EscapeString(r.bodyPlain) + "</pre>"
		}
		payload := map[string]interface{}{
			"from":    r.aliasAddr,
			"to":      []string{fromClean},
			"subject": subject,
			"html":    bodyHTML,
			"headers": map[string]string{"Auto-Submitted": "auto-replied"},
		}
		if _, err := w.resendSvc.Fetch(ctx, in.orgID, "POST", "/emails", payload); err != nil {
			slog.Error("inbound rules: auto-reply send failed", "rule_id", r.id, "to", fromClean, "error", err)
			continue
		}
		slog.Info("inbound rules: auto-replied", "rule_id", r.id, "alias", r.aliasAddr, "to", fromClean)
	}
}

// isOrgDomainAddress reports whether an address belongs to one of the org's
// own domains.
func (w *EmailWorker) isOrgDomainAddress(ctx context.Context, orgID, addr string) bool {
	at := strings.LastIndexByte(addr, '@')
	if at < 0 {
		return false
	}
	var exists bool
	if err := w.store.Q().QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM domains WHERE org_id = $1 AND domain = $2 AND status != 'deleted')`,
		orgID, strings.ToLower(addr[at+1:]),
	).Scan(&exists); err != nil {
		return false
	}
	return exists
}
