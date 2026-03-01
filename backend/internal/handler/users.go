package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	DB        *pgxpool.Pool
	RDB       *redis.Client
	Secret    string
	ResendSvc *service.ResendService
	AppURL    string
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" && claims.Role != "owner" {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, email, name, role, status, created_at FROM users WHERE org_id = $1 ORDER BY created_at`,
		claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, email, name, role, status string
		var createdAt time.Time
		if rows.Scan(&id, &email, &name, &role, &status, &createdAt) == nil {
			users = append(users, map[string]interface{}{
				"id": id, "email": email, "name": name, "role": role,
				"status": status, "created_at": createdAt,
			})
		}
	}

	if users == nil {
		users = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *UserHandler) Invite(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if err := validateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLength(req.Name, "name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	token := generateToken(32)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	var userID string
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO users (org_id, email, name, role, status, invite_token, invite_expires_at)
		 VALUES ($1, $2, $3, $4, 'invited', $5, $6)
		 ON CONFLICT (email) DO UPDATE SET
		   status = CASE WHEN users.status = 'placeholder' THEN 'invited' ELSE users.status END,
		   invite_token = $5,
		   invite_expires_at = $6,
		   updated_at = now()
		 RETURNING id`,
		claims.OrgID, req.Email, req.Name, req.Role, token, expiresAt,
	).Scan(&userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to invite user")
		return
	}

	// Look up org name and inviter name for the invite email
	var orgName string
	warnIfErr(h.DB.QueryRow(r.Context(), `SELECT name FROM orgs WHERE id = $1`, claims.OrgID).Scan(&orgName),
		"users: org name lookup failed", "org_id", claims.OrgID)
	if orgName == "" {
		orgName = "an Inboxes workspace"
	}
	var inviterName string
	warnIfErr(h.DB.QueryRow(r.Context(), `SELECT name FROM users WHERE id = $1`, claims.UserID).Scan(&inviterName),
		"users: inviter name lookup failed", "user_id", claims.UserID)

	// Send invite email via system Resend key
	from := h.ResendSvc.GetSystemFrom(r.Context())
	if from == "" {
		from = "noreply@inboxes.net"
		slog.Warn("users: using hardcoded noreply fallback — configure system email in settings")
	}
	inviteIntro := fmt.Sprintf("You've been invited to join <strong>%s</strong> on Inboxes.", orgName)
	if inviterName != "" {
		inviteIntro = fmt.Sprintf("<strong>%s</strong> has invited you to join <strong>%s</strong> on Inboxes.", inviterName, orgName)
	}
	h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      []string{req.Email},
		"subject": fmt.Sprintf("You're invited to %s on Inboxes", orgName),
		"html":    fmt.Sprintf("<p>%s</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>", inviteIntro, h.AppURL, token),
	})

	writeJSON(w, http.StatusCreated, map[string]string{"id": userID, "status": "invited"})
}

func (h *UserHandler) Reinvite(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	userID := chi.URLParam(r, "id")

	var email string
	token := generateToken(32)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	err := h.DB.QueryRow(r.Context(),
		`UPDATE users SET invite_token = $1, invite_expires_at = $2, updated_at = now()
		 WHERE id = $3 AND org_id = $4 AND status IN ('invited', 'placeholder')
		 RETURNING email`,
		token, expiresAt, userID, claims.OrgID).Scan(&email)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found or already active")
		return
	}

	var reinviteOrgName string
	warnIfErr(h.DB.QueryRow(r.Context(), `SELECT name FROM orgs WHERE id = $1`, claims.OrgID).Scan(&reinviteOrgName),
		"users: org name lookup failed for reinvite", "org_id", claims.OrgID)
	if reinviteOrgName == "" {
		reinviteOrgName = "an Inboxes workspace"
	}

	from := h.ResendSvc.GetSystemFrom(r.Context())
	if from == "" {
		from = "noreply@inboxes.net"
		slog.Warn("users: using hardcoded noreply fallback — configure system email in settings")
	}
	h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      []string{email},
		"subject": fmt.Sprintf("Reminder: You're invited to %s on Inboxes", reinviteOrgName),
		"html":    fmt.Sprintf("<p>You've been invited to join <strong>%s</strong> on Inboxes.</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>", reinviteOrgName, h.AppURL, token),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "reinvited"})
}

func (h *UserHandler) Disable(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	userID := chi.URLParam(r, "id")
	if userID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot disable yourself")
		return
	}

	// Check if target is an admin — if so, ensure at least 2 active admins remain
	var targetRole string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT role FROM users WHERE id = $1 AND org_id = $2`, userID, claims.OrgID,
	).Scan(&targetRole); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if targetRole == "admin" {
		var adminCount int
		if err := h.DB.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active'`,
			claims.OrgID,
		).Scan(&adminCount); err == nil && adminCount <= 2 {
			writeError(w, http.StatusBadRequest, "cannot disable: at least 2 active admins required")
			return
		}
	}

	// Optional reassignment target
	var req struct {
		TargetUserID string `json:"target_user_id"`
	}
	// Body is optional — ignore parse errors
	readJSON(r, &req)

	ctx := r.Context()

	blacklist := service.NewTokenBlacklist(h.RDB)

	if req.TargetUserID != "" {
		result, err := h.reassignAndDisable(ctx, claims.OrgID, claims.UserID, userID, req.TargetUserID)
		if err != nil {
			slog.Error("user: reassign and disable failed", "source", userID, "target", req.TargetUserID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to disable user")
			return
		}
		if err := blacklist.RevokeAllForUser(ctx, userID); err != nil {
			slog.Error("users: session revocation failed on disable", "user_id", userID, "error", err)
		}
		blacklist.ClearSessions(ctx, userID)
		h.clearStatusCache(ctx, userID)
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Simple disable without reassignment — also clears invite tokens
	tag, err := h.DB.Exec(ctx,
		`UPDATE users SET status = 'disabled', invite_token = NULL, invite_expires_at = NULL, updated_at = now() WHERE id = $1 AND org_id = $2`,
		userID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Clean up alias_users so emails route to org admin catch-all
	if _, err := h.DB.Exec(ctx, `DELETE FROM alias_users WHERE user_id = $1`, userID); err != nil {
		slog.Error("users: failed to delete alias_users on disable", "user_id", userID, "error", err)
	}

	if err := blacklist.RevokeAllForUser(ctx, userID); err != nil {
		slog.Error("users: session revocation failed on disable", "user_id", userID, "error", err)
	}
	blacklist.ClearSessions(ctx, userID)
	h.clearStatusCache(ctx, userID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "disabled",
		"threads_moved": 0,
		"aliases_moved": 0,
		"drafts_moved":  0,
	})
}

func (h *UserHandler) reassignAndDisable(ctx context.Context, orgID, adminID, sourceID, targetID string) (map[string]interface{}, error) {
	// Validate target user exists, is active, same org
	var targetStatus string
	err := h.DB.QueryRow(ctx,
		`SELECT status FROM users WHERE id = $1 AND org_id = $2`,
		targetID, orgID,
	).Scan(&targetStatus)
	if err != nil {
		return nil, fmt.Errorf("target user not found")
	}
	if targetStatus != "active" {
		return nil, fmt.Errorf("target user is not active")
	}

	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction")
	}
	defer tx.Rollback(ctx)

	// Transfer threads
	threadTag, err := tx.Exec(ctx,
		`UPDATE threads SET user_id = $1, updated_at = now() WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer threads: %w", err)
	}
	threadsMoved := threadTag.RowsAffected()

	// Copy alias_users to target (skip conflicts where target already has access)
	aliasTag, err := tx.Exec(ctx,
		`INSERT INTO alias_users (alias_id, user_id, can_send_as, is_default)
		 SELECT alias_id, $1, can_send_as, false
		 FROM alias_users WHERE user_id = $2
		 ON CONFLICT (alias_id, user_id) DO NOTHING`,
		targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer aliases: %w", err)
	}
	aliasesMoved := aliasTag.RowsAffected()

	// Delete source's alias_users
	if _, err := tx.Exec(ctx, `DELETE FROM alias_users WHERE user_id = $1`, sourceID); err != nil {
		slog.Error("user: failed to delete source alias_users", "source_user_id", sourceID, "error", err)
	}

	// Transfer drafts
	draftTag, err := tx.Exec(ctx,
		`UPDATE drafts SET user_id = $1, updated_at = now() WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer drafts: %w", err)
	}
	draftsMoved := draftTag.RowsAffected()

	// Reassign sync_jobs
	if _, err := tx.Exec(ctx,
		`UPDATE sync_jobs SET user_id = $1 WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID); err != nil {
		return nil, fmt.Errorf("failed to reassign sync_jobs: %w", err)
	}

	// Reassign email_jobs
	if _, err := tx.Exec(ctx,
		`UPDATE email_jobs SET user_id = $1 WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID); err != nil {
		return nil, fmt.Errorf("failed to reassign email_jobs: %w", err)
	}

	// Reassign discovered_addresses
	if _, err := tx.Exec(ctx,
		`UPDATE discovered_addresses SET user_id = $1 WHERE user_id = $2`,
		targetID, sourceID); err != nil {
		return nil, fmt.Errorf("failed to reassign discovered_addresses: %w", err)
	}

	// Disable user
	tag, err := tx.Exec(ctx,
		`UPDATE users SET status = 'disabled', updated_at = now() WHERE id = $1 AND org_id = $2`,
		sourceID, orgID)
	if err != nil || tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("user not found or already disabled")
	}

	// Insert audit row (best-effort — don't fail the parent operation)
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_reassignments (org_id, source_user_id, target_user_id, reassigned_by, threads_moved, aliases_moved, drafts_moved)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		orgID, sourceID, targetID, adminID, threadsMoved, aliasesMoved, draftsMoved); err != nil {
		slog.Error("user: failed to insert audit row", "source", sourceID, "target", targetID, "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	slog.Info("user: disabled with reassignment",
		"source", sourceID, "target", targetID,
		"threads", threadsMoved, "aliases", aliasesMoved, "drafts", draftsMoved)

	return map[string]interface{}{
		"status":        "disabled",
		"threads_moved": threadsMoved,
		"aliases_moved": aliasesMoved,
		"drafts_moved":  draftsMoved,
	}, nil
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var id, email, name, role, status string
	var createdAt time.Time
	var isOwner bool
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, email, name, role, status, created_at, is_owner FROM users WHERE id = $1`,
		claims.UserID).Scan(&id, &email, &name, &role, &status, &createdAt, &isOwner)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "email": email, "name": name, "role": role,
		"status": status, "created_at": createdAt, "is_owner": isOwner,
	})
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validateLength(req.Name, "name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`UPDATE users SET name = $1, updated_at = now() WHERE id = $2`,
		req.Name, claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := validatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var hash string
	err := h.DB.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, claims.UserID).Scan(&hash)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)) != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		string(newHash), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Revoke all old tokens, then re-issue a fresh one for the current session
	pwBlacklist := service.NewTokenBlacklist(h.RDB)
	revErr := pwBlacklist.RevokeAllForUser(r.Context(), claims.UserID)
	pwBlacklist.ClearSessions(r.Context(), claims.UserID)

	newToken, newJTI, err := middleware.GenerateToken(h.Secret, claims.UserID, claims.OrgID, claims.Role)
	if err == nil {
		middleware.SetTokenCookie(w, newToken, h.AppURL)
		pwBlacklist.RegisterSession(r.Context(), claims.UserID, newJTI)
	}

	if revErr != nil {
		slog.Error("users: session revocation failed during password change", "user_id", claims.UserID, "error", revErr)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"warning": "password changed but other sessions may still be active",
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var prefs []byte
	err := h.DB.QueryRow(r.Context(),
		`SELECT COALESCE(notification_preferences, '{}') FROM users WHERE id = $1`,
		claims.UserID).Scan(&prefs)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(prefs)
}

func (h *UserHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var prefs map[string]interface{}
	if err := readJSON(r, &prefs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`UPDATE users SET notification_preferences = $1, updated_at = now() WHERE id = $2`,
		prefs, claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update preferences")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) MyAliases(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT a.id, a.address, a.name, a.domain_id, au.can_send_as, au.is_default
		 FROM aliases a
		 JOIN alias_users au ON au.alias_id = a.id
		 WHERE au.user_id = $1 AND a.org_id = $2 AND a.deleted_at IS NULL
		 ORDER BY au.is_default DESC, a.address`,
		claims.UserID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list aliases")
		return
	}
	defer rows.Close()

	var aliases []map[string]interface{}
	for rows.Next() {
		var id, address, name, domainID string
		var canSendAs, isDefault bool
		if rows.Scan(&id, &address, &name, &domainID, &canSendAs, &isDefault) == nil {
			aliases = append(aliases, map[string]interface{}{
				"id": id, "address": address, "name": name,
				"domain_id": domainID, "can_send_as": canSendAs,
				"is_default": isDefault,
			})
		}
	}

	if aliases == nil {
		aliases = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (h *UserHandler) ChangeRole(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	userID := chi.URLParam(r, "id")
	if userID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil || (req.Role != "admin" && req.Role != "member") {
		writeError(w, http.StatusBadRequest, "role must be 'admin' or 'member'")
		return
	}

	// Don't allow changing the owner's role
	var isOwner bool
	var currentRole string
	err := h.DB.QueryRow(r.Context(),
		`SELECT is_owner, role FROM users WHERE id = $1 AND org_id = $2`,
		userID, claims.OrgID).Scan(&isOwner, &currentRole)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if isOwner {
		writeError(w, http.StatusBadRequest, "cannot change the owner's role")
		return
	}

	// Prevent demoting an admin if it would leave fewer than 2 active admins
	if currentRole == "admin" && req.Role == "member" {
		var adminCount int
		if err := h.DB.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active'`,
			claims.OrgID,
		).Scan(&adminCount); err == nil && adminCount <= 2 {
			writeError(w, http.StatusBadRequest, "cannot demote: at least 2 active admins required")
			return
		}
	}

	tag, err := h.DB.Exec(r.Context(),
		`UPDATE users SET role = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
		req.Role, userID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	slog.Info("user: role changed", "user_id", userID, "new_role", req.Role, "by", claims.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"role": req.Role})
}

func (h *UserHandler) Enable(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	userID := chi.URLParam(r, "id")

	tag, err := h.DB.Exec(r.Context(),
		`UPDATE users SET status = 'active', updated_at = now() WHERE id = $1 AND org_id = $2 AND status = 'disabled'`,
		userID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found or not disabled")
		return
	}

	h.clearStatusCache(r.Context(), userID)
	slog.Info("user: re-enabled", "user_id", userID, "by", claims.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// clearStatusCache removes the cached user status from Redis so the next
// auth middleware check re-queries the database.
func (h *UserHandler) clearStatusCache(ctx context.Context, userID string) {
	if h.RDB != nil {
		h.RDB.Del(ctx, "user:status:"+userID)
	}
}

func (h *UserHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	bl := service.NewTokenBlacklist(h.RDB)
	sessions := bl.ListSessions(r.Context(), claims.UserID)
	if sessions == nil {
		sessions = []service.Session{}
	}

	// Mark which session is the current one
	result := make([]map[string]interface{}, len(sessions))
	for i, s := range sessions {
		result[i] = map[string]interface{}{
			"jti":        s.JTI,
			"created_at": s.CreatedAt,
			"current":    s.JTI == claims.ID,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *UserHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	jti := chi.URLParam(r, "jti")
	if jti == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}
	if jti == claims.ID {
		writeError(w, http.StatusBadRequest, "cannot revoke current session")
		return
	}

	bl := service.NewTokenBlacklist(h.RDB)
	if err := bl.RevokeSession(r.Context(), claims.UserID, jti); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generateToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
