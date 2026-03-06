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
	"github.com/inboxes/backend/internal/store"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	Store     store.Store
	RDB       *redis.Client
	Secret    string
	ResendSvc *service.ResendService
	AppURL    string
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	users, err := h.Store.ListUsers(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *UserHandler) Invite(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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

	userID, err := h.Store.InsertInvitedUser(r.Context(), claims.OrgID, req.Email, req.Name, req.Role, token, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to invite user")
		return
	}

	// Look up org name and inviter name for the invite email
	orgName, _ := h.Store.GetOrgName(r.Context(), claims.OrgID)
	if orgName == "" {
		orgName = "an Inboxes workspace"
	}
	inviterName, _ := h.Store.GetUserName(r.Context(), claims.UserID)

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
	if _, err := h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      []string{req.Email},
		"subject": fmt.Sprintf("You're invited to %s on Inboxes", orgName),
		"html":    fmt.Sprintf("<p>%s</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>", inviteIntro, h.AppURL, token),
	}); err != nil {
		slog.Error("users: failed to send invite email", "email", req.Email, "error", err)
		writeError(w, http.StatusInternalServerError, "user created but invite email failed to send")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": userID, "status": "invited"})
}

func (h *UserHandler) Reinvite(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	userID := chi.URLParam(r, "id")

	token := generateToken(32)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	email, err := h.Store.ReinviteUser(r.Context(), userID, claims.OrgID, token, expiresAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found or already active")
		return
	}

	reinviteOrgName, _ := h.Store.GetOrgName(r.Context(), claims.OrgID)
	if reinviteOrgName == "" {
		reinviteOrgName = "an Inboxes workspace"
	}

	from := h.ResendSvc.GetSystemFrom(r.Context())
	if from == "" {
		from = "noreply@inboxes.net"
		slog.Warn("users: using hardcoded noreply fallback — configure system email in settings")
	}
	if _, err := h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      []string{email},
		"subject": fmt.Sprintf("Reminder: You're invited to %s on Inboxes", reinviteOrgName),
		"html":    fmt.Sprintf("<p>You've been invited to join <strong>%s</strong> on Inboxes.</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>", reinviteOrgName, h.AppURL, token),
	}); err != nil {
		slog.Error("users: failed to send reinvite email", "email", email, "error", err)
		writeError(w, http.StatusInternalServerError, "reinvite email failed to send")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reinvited"})
}

func (h *UserHandler) Disable(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	userID := chi.URLParam(r, "id")
	if userID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot disable yourself")
		return
	}

	// Check if target is an admin — if so, ensure at least 2 active admins remain
	targetRole, err := h.Store.GetUserRole(r.Context(), userID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if targetRole == "admin" {
		adminCount, err := h.Store.CountActiveAdmins(r.Context(), claims.OrgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check admin count")
			return
		}
		if adminCount <= 2 {
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
		result, err := h.Store.ReassignAndDisable(ctx, claims.OrgID, claims.UserID, userID, req.TargetUserID)
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
	rows, err := h.Store.DisableUser(ctx, userID, claims.OrgID)
	if err != nil || rows == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Clean up alias_users so emails route to org admin catch-all
	if err := h.Store.DeleteAliasUsers(ctx, userID); err != nil {
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

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	me, err := h.Store.GetMe(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, me)
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

	if err := h.Store.UpdateUserName(r.Context(), claims.UserID, req.Name); err != nil {
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

	hash, err := h.Store.GetPasswordHash(r.Context(), claims.UserID)
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

	if err := h.Store.UpdatePassword(r.Context(), claims.UserID, string(newHash)); err != nil {
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

	prefs, err := h.Store.GetPreferences(r.Context(), claims.UserID)
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

	if err := h.Store.UpdatePreferences(r.Context(), claims.UserID, prefs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update preferences")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) MyAliases(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	aliases, err := h.Store.ListMyAliases(r.Context(), claims.UserID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list aliases")
		return
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (h *UserHandler) ChangeRole(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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
	isOwner, currentRole, err := h.Store.GetUserOwnerAndRole(r.Context(), userID, claims.OrgID)
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
		adminCount, err := h.Store.CountActiveAdmins(r.Context(), claims.OrgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check admin count")
			return
		}
		if adminCount <= 2 {
			writeError(w, http.StatusBadRequest, "cannot demote: at least 2 active admins required")
			return
		}
	}

	rows, err := h.Store.ChangeRole(r.Context(), userID, claims.OrgID, req.Role)
	if err != nil || rows == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	slog.Info("user: role changed", "user_id", userID, "new_role", req.Role, "by", claims.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"role": req.Role})
}

func (h *UserHandler) Enable(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	userID := chi.URLParam(r, "id")

	rows, err := h.Store.EnableUser(r.Context(), userID, claims.OrgID)
	if err != nil || rows == 0 {
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
