package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
	"github.com/microcosm-cc/bluemonday"
)

// RuleHandler serves forwarding rules and auto-replies. Admins set the org
// policy (what is allowed); users manage rules for their own aliases only.
type RuleHandler struct {
	Store store.Store
}

// canManageAlias reports whether the caller may attach rules to an alias.
// Admins manage any org alias; members only aliases assigned to them.
func (h *RuleHandler) canManageAlias(ctx context.Context, claims *middleware.Claims, aliasID string) bool {
	if claims.Role == "admin" {
		var ok bool
		err := h.Store.Q().QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM aliases WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL)`,
			aliasID, claims.OrgID).Scan(&ok)
		return err == nil && ok
	}
	var ok bool
	err := h.Store.Q().QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM alias_users au
			JOIN aliases a ON a.id = au.alias_id
			WHERE au.alias_id = $1 AND au.user_id = $2 AND a.org_id = $3 AND a.deleted_at IS NULL
		)`, aliasID, claims.UserID, claims.OrgID).Scan(&ok)
	return err == nil && ok
}

func (h *RuleHandler) orgPolicy(ctx context.Context, orgID string) (forwarding, autoReply, external bool) {
	h.Store.Q().QueryRow(ctx,
		`SELECT forwarding_enabled, auto_reply_enabled, external_forwarding_allowed FROM orgs WHERE id = $1`,
		orgID).Scan(&forwarding, &autoReply, &external)
	return
}

// ── Forwarding rules ──

func (h *RuleHandler) ListForwardingRules(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	query := `SELECT fr.id, fr.alias_id, a.address, fr.forward_to, fr.enabled, fr.user_id, fr.created_at
		 FROM forwarding_rules fr
		 JOIN aliases a ON a.id = fr.alias_id
		 WHERE fr.org_id = $1`
	args := []interface{}{claims.OrgID}
	if claims.Role != "admin" {
		query += ` AND fr.alias_id IN (SELECT alias_id FROM alias_users WHERE user_id = $2)`
		args = append(args, claims.UserID)
	}
	query += ` ORDER BY a.address, fr.forward_to`

	rows, err := h.Store.Q().Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list forwarding rules")
		return
	}
	defer rows.Close()

	rules := []map[string]interface{}{}
	for rows.Next() {
		var id, aliasID, aliasAddr, forwardTo, userID string
		var enabled bool
		var createdAt time.Time
		if rows.Scan(&id, &aliasID, &aliasAddr, &forwardTo, &enabled, &userID, &createdAt) == nil {
			rules = append(rules, map[string]interface{}{
				"id": id, "alias_id": aliasID, "alias_address": aliasAddr,
				"forward_to": forwardTo, "enabled": enabled, "user_id": userID,
				"created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *RuleHandler) CreateForwardingRule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	var req struct {
		AliasID   string `json:"alias_id"`
		ForwardTo string `json:"forward_to"`
	}
	if err := readJSON(r, &req); err != nil || req.AliasID == "" || req.ForwardTo == "" {
		writeError(w, http.StatusBadRequest, "alias_id and forward_to are required")
		return
	}
	req.ForwardTo = normalizeEmail(req.ForwardTo)
	if err := validateEmail(req.ForwardTo); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	forwardingAllowed, _, externalAllowed := h.orgPolicy(ctx, claims.OrgID)
	if !forwardingAllowed {
		writeError(w, http.StatusForbidden, "forwarding is disabled for this organization")
		return
	}
	if !h.canManageAlias(ctx, claims, req.AliasID) {
		writeError(w, http.StatusForbidden, "you can only add rules for your own aliases")
		return
	}
	if !externalAllowed {
		at := strings.LastIndexByte(req.ForwardTo, '@')
		var isOrgDomain bool
		h.Store.Q().QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM domains WHERE org_id = $1 AND domain = $2 AND status != 'deleted')`,
			claims.OrgID, req.ForwardTo[at+1:]).Scan(&isOrgDomain)
		if !isOrgDomain {
			writeError(w, http.StatusForbidden, "forwarding to external addresses is disabled for this organization")
			return
		}
	}
	// Forwarding to the alias itself would loop.
	var aliasAddr string
	h.Store.Q().QueryRow(ctx, `SELECT address FROM aliases WHERE id = $1`, req.AliasID).Scan(&aliasAddr)
	if strings.EqualFold(aliasAddr, req.ForwardTo) {
		writeError(w, http.StatusBadRequest, "cannot forward an alias to itself")
		return
	}

	var id string
	err := h.Store.Q().QueryRow(ctx,
		`INSERT INTO forwarding_rules (org_id, user_id, alias_id, forward_to)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (alias_id, forward_to) DO NOTHING
		 RETURNING id`,
		claims.OrgID, claims.UserID, req.AliasID, req.ForwardTo,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusConflict, "this forwarding rule already exists")
		return
	}

	slog.Info("rules: forwarding rule created", "rule_id", id, "alias_id", req.AliasID, "user", claims.UserID)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *RuleHandler) UpdateForwardingRule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ruleID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}

	var aliasID string
	if err := h.Store.Q().QueryRow(ctx,
		`SELECT alias_id FROM forwarding_rules WHERE id = $1 AND org_id = $2`,
		ruleID, claims.OrgID).Scan(&aliasID); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if !h.canManageAlias(ctx, claims, aliasID) {
		writeError(w, http.StatusForbidden, "you can only manage rules for your own aliases")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`UPDATE forwarding_rules SET enabled = $1, updated_at = now() WHERE id = $2`,
		*req.Enabled, ruleID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RuleHandler) DeleteForwardingRule(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ruleID := chi.URLParam(r, "id")
	ctx := r.Context()

	var aliasID string
	if err := h.Store.Q().QueryRow(ctx,
		`SELECT alias_id FROM forwarding_rules WHERE id = $1 AND org_id = $2`,
		ruleID, claims.OrgID).Scan(&aliasID); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if !h.canManageAlias(ctx, claims, aliasID) {
		writeError(w, http.StatusForbidden, "you can only manage rules for your own aliases")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`DELETE FROM forwarding_rules WHERE id = $1 AND org_id = $2`, ruleID, claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Auto-replies ──

func (h *RuleHandler) ListAutoReplies(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	query := `SELECT ar.id, ar.alias_id, a.address, ar.subject, ar.body_html, ar.body_plain,
		 ar.starts_at, ar.ends_at, ar.enabled, ar.user_id, ar.created_at
		 FROM auto_replies ar
		 JOIN aliases a ON a.id = ar.alias_id
		 WHERE ar.org_id = $1`
	args := []interface{}{claims.OrgID}
	if claims.Role != "admin" {
		query += ` AND ar.alias_id IN (SELECT alias_id FROM alias_users WHERE user_id = $2)`
		args = append(args, claims.UserID)
	}
	query += ` ORDER BY a.address`

	rows, err := h.Store.Q().Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list auto-replies")
		return
	}
	defer rows.Close()

	replies := []map[string]interface{}{}
	for rows.Next() {
		var id, aliasID, aliasAddr, subject, bodyHTML, bodyPlain, userID string
		var startsAt, endsAt *time.Time
		var enabled bool
		var createdAt time.Time
		if rows.Scan(&id, &aliasID, &aliasAddr, &subject, &bodyHTML, &bodyPlain,
			&startsAt, &endsAt, &enabled, &userID, &createdAt) == nil {
			replies = append(replies, map[string]interface{}{
				"id": id, "alias_id": aliasID, "alias_address": aliasAddr,
				"subject": subject, "body_html": bodyHTML, "body_plain": bodyPlain,
				"starts_at": startsAt, "ends_at": endsAt, "enabled": enabled,
				"user_id": userID, "created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, replies)
}

func (h *RuleHandler) UpsertAutoReply(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	var req struct {
		AliasID   string     `json:"alias_id"`
		Subject   string     `json:"subject"`
		BodyHTML  string     `json:"body_html"`
		BodyPlain string     `json:"body_plain"`
		StartsAt  *time.Time `json:"starts_at"`
		EndsAt    *time.Time `json:"ends_at"`
		Enabled   *bool      `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil || req.AliasID == "" {
		writeError(w, http.StatusBadRequest, "alias_id is required")
		return
	}
	if req.BodyHTML == "" && req.BodyPlain == "" {
		writeError(w, http.StatusBadRequest, "a reply body is required")
		return
	}
	if err := validateLength(req.Subject, "subject", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLength(req.BodyHTML, "body", 20000); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		writeError(w, http.StatusBadRequest, "ends_at must be after starts_at")
		return
	}

	_, autoReplyAllowed, _ := h.orgPolicy(ctx, claims.OrgID)
	if !autoReplyAllowed {
		writeError(w, http.StatusForbidden, "auto-replies are disabled for this organization")
		return
	}
	if !h.canManageAlias(ctx, claims, req.AliasID) {
		writeError(w, http.StatusForbidden, "you can only add auto-replies for your own aliases")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sanitized := bluemonday.UGCPolicy().Sanitize(req.BodyHTML)

	var id string
	err := h.Store.Q().QueryRow(ctx,
		`INSERT INTO auto_replies (org_id, user_id, alias_id, subject, body_html, body_plain, starts_at, ends_at, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (alias_id) DO UPDATE SET
		   subject = EXCLUDED.subject, body_html = EXCLUDED.body_html, body_plain = EXCLUDED.body_plain,
		   starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at, enabled = EXCLUDED.enabled,
		   user_id = EXCLUDED.user_id, updated_at = now()
		 RETURNING id`,
		claims.OrgID, claims.UserID, req.AliasID, req.Subject, sanitized, req.BodyPlain,
		req.StartsAt, req.EndsAt, enabled,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save auto-reply")
		return
	}

	slog.Info("rules: auto-reply saved", "rule_id", id, "alias_id", req.AliasID, "user", claims.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (h *RuleHandler) DeleteAutoReply(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ruleID := chi.URLParam(r, "id")
	ctx := r.Context()

	var aliasID string
	if err := h.Store.Q().QueryRow(ctx,
		`SELECT alias_id FROM auto_replies WHERE id = $1 AND org_id = $2`,
		ruleID, claims.OrgID).Scan(&aliasID); err != nil {
		writeError(w, http.StatusNotFound, "auto-reply not found")
		return
	}
	if !h.canManageAlias(ctx, claims, aliasID) {
		writeError(w, http.StatusForbidden, "you can only manage auto-replies for your own aliases")
		return
	}

	if _, err := h.Store.Q().Exec(ctx,
		`DELETE FROM auto_replies WHERE id = $1 AND org_id = $2`, ruleID, claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete auto-reply")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
