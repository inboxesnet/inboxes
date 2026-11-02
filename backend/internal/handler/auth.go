package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	Store     store.Store
	RDB       *redis.Client
	Secret    string
	AppURL    string
	ResendSvc *service.ResendService
	StripeKey string
}

// dummyHash is used for constant-time comparison when user is not found,
// preventing timing attacks that could reveal whether an email exists.
var dummyHash []byte

func init() {
	dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
}

type signupRequest struct {
	OrgName  string `json:"org_name"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type claimRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Solo mode: block signups when at least one user exists
	if h.StripeKey == "" {
		count, err := h.Store.CountUsers(ctx)
		if err == nil && count > 0 {
			writeError(w, http.StatusForbidden, "registration is closed")
			return
		}
	}

	var req signupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" || req.OrgName == "" {
		writeError(w, http.StatusBadRequest, "email, password, and org_name are required")
		return
	}
	if err := validateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLength(req.OrgName, "org_name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLength(req.Name, "name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateNewPassword(r.Context(), req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// A verified account can register another workspace with its existing
	// password. This creates a new organization-scoped user record and never
	// copies organization integrations such as the Resend API key.
	if h.StripeKey != "" {
		existing, lookupErr := h.Store.GetLoginMemberships(ctx, req.Email)
		if lookupErr != nil {
			slog.Error("auth: account lookup failed during workspace signup", "error", lookupErr)
			writeError(w, http.StatusInternalServerError, "failed to create workspace")
			return
		}
		if len(existing) > 0 {
			passwordHash := existing[0].PasswordHash
			if passwordHash == "" || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
				h.respondExistingAccountSignup(ctx, w, req.Email)
				return
			}
			if !existing[0].EmailVerified {
				code := generateVerificationCode()
				expires := time.Now().Add(15 * time.Minute)
				updated, err := h.Store.ResendVerificationCode(ctx, req.Email, code, expires)
				if err != nil {
					slog.Error("auth: failed to refresh verification code", "email", req.Email, "error", err)
				}
				if err != nil || updated == 0 {
					h.respondExistingAccountSignup(ctx, w, req.Email)
					return
				}
				from := h.ResendSvc.GetSystemFrom(ctx)
				if from == "" {
					from = "noreply@inboxes.net"
				}
				if _, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
					"from": from, "to": []string{req.Email}, "subject": "Verify your email",
					"html": fmt.Sprintf("<p>Your verification code is: <strong>%s</strong></p><p>This code expires in 15 minutes.</p>", code),
				}); err != nil {
					slog.Error("auth: failed to send verification email", "email", req.Email, "error", err)
				}
				writeJSON(w, http.StatusCreated, map[string]interface{}{
					"requires_verification": true,
					"email":                 req.Email,
				})
				return
			}
			active := false
			for _, membership := range existing {
				if membership.Status == "active" {
					active = true
					break
				}
			}
			if !active {
				h.respondExistingAccountSignup(ctx, w, req.Email)
				return
			}

			var orgID, userID string
			if err := h.Store.WithTx(ctx, func(tx store.Store) error {
				var createErr error
				orgID, userID, createErr = tx.CreateOrgForAccount(ctx, req.OrgName, req.Email)
				return createErr
			}); err != nil {
				slog.Error("auth: failed to add workspace to account", "email", req.Email, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create workspace")
				return
			}

			token, jti, err := middleware.GenerateToken(h.Secret, userID, orgID, "admin")
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to create session")
				return
			}
			middleware.SetTokenCookie(w, token, h.AppURL)
			service.NewTokenBlacklist(h.RDB).RegisterSession(ctx, userID, jti)
			slog.Info("auth: workspace added to account", "email", req.Email, "org_id", orgID)
			writeJSON(w, http.StatusCreated, map[string]any{
				"user": map[string]string{
					"id": userID, "org_id": orgID, "email": req.Email,
					"name": existing[0].Name, "role": "admin",
				},
				"onboarding_completed": false,
			})
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	name := req.Name
	if name == "" {
		name = strings.Split(req.Email, "@")[0]
	}
	// In hosted mode, new users need email verification
	emailVerified := true
	if h.StripeKey != "" {
		emailVerified = false
	}

	// In self-hosted mode, the first user (count == 0) becomes the instance owner
	isSelfHostedOwner := h.StripeKey == ""

	var orgID, userID string
	var verificationCode string
	txErr := h.Store.WithTx(ctx, func(tx store.Store) error {
		var createErr error
		orgID, userID, createErr = tx.CreateOrgAndAdmin(ctx, req.OrgName, req.Email, name, string(hash), emailVerified, isSelfHostedOwner)
		if createErr != nil {
			return createErr
		}

		// If hosted, generate verification code within the same transaction
		if h.StripeKey != "" {
			verificationCode = generateVerificationCode()
			expires := time.Now().Add(15 * time.Minute)
			if err := tx.SetVerificationCode(ctx, userID, verificationCode, expires); err != nil {
				return fmt.Errorf("set verification code: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		if strings.Contains(txErr.Error(), "email already registered") {
			// Another signup may have created the account after the lookup above.
			if h.StripeKey != "" {
				h.respondExistingAccountSignup(ctx, w, req.Email)
				return
			}
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("auth: signup transaction failed", "error", txErr)
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	slog.Info("auth: signup", "email", req.Email, "org_id", orgID)

	// If hosted, send verification email and return early
	if h.StripeKey != "" {
		from := h.ResendSvc.GetSystemFrom(ctx)
		if from == "" {
			from = "noreply@inboxes.net"
			slog.Warn("auth: using hardcoded noreply fallback — configure system email in settings")
		}
		if _, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
			"from":    from,
			"to":      []string{req.Email},
			"subject": "Verify your email",
			"html":    fmt.Sprintf("<p>Your verification code is: <strong>%s</strong></p><p>This code expires in 15 minutes.</p>", verificationCode),
		}); err != nil {
			slog.Error("auth: failed to send verification email", "email", req.Email, "error", err)
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"requires_verification": true,
			"email":                 req.Email,
		})
		return
	}

	token, _, err := middleware.GenerateToken(h.Secret, userID, orgID, "admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  req.Email,
			"name":   name,
			"role":   "admin",
		},
	})
}

// respondExistingAccountSignup keeps public signup responses consistent for
// unknown emails and existing accounts whose credentials are not valid.
func (h *AuthHandler) respondExistingAccountSignup(ctx context.Context, w http.ResponseWriter, email string) {
	if h.ResendSvc != nil {
		from := h.ResendSvc.GetSystemFrom(ctx)
		if from == "" {
			from = "noreply@inboxes.net"
		}
		if _, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
			"from": from, "to": []string{email}, "subject": "Your Inboxes account",
			"html": "<p>If you already have an Inboxes account, sign in with its existing password to create a workspace. If this wasn't you, you can ignore this message.</p>",
		}); err != nil {
			slog.Error("auth: failed to send existing-account notice", "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"requires_verification": true,
		"email":                 email,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if err := validateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Password) > 128 {
		writeError(w, http.StatusBadRequest, "password must be 128 characters or fewer")
		return
	}

	ctx := r.Context()
	memberships, err := h.Store.GetLoginMemberships(ctx, req.Email)
	if err != nil {
		// Constant-time comparison to prevent timing attacks revealing email existence.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		slog.Warn("auth: login failed", "email", req.Email, "reason", "user not found")
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if len(memberships) == 0 || memberships[0].PasswordHash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// The account has one shared password hash; compare it once before
	// returning any organization names or membership details.
	if err := bcrypt.CompareHashAndPassword([]byte(memberships[0].PasswordHash), []byte(req.Password)); err != nil {
		slog.Warn("auth: login failed", "email", req.Email, "reason", "bad password")
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// In hosted mode, require email verification
	if h.StripeKey != "" && !memberships[0].EmailVerified {
		slog.Warn("auth: login failed", "email", req.Email, "reason", "email not verified")
		writeError(w, http.StatusForbidden, "email_not_verified")
		return
	}

	active := make([]store.LoginMembership, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Status == "active" {
			active = append(active, membership)
		}
	}
	if req.OrgID != "" {
		selected := active[:0]
		for _, membership := range active {
			if membership.OrgID == req.OrgID {
				selected = append(selected, membership)
			}
		}
		active = selected
	}
	if len(active) == 0 {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if req.OrgID == "" && len(active) > 1 {
		organizations := make([]map[string]string, 0, len(active))
		for _, membership := range active {
			organizations = append(organizations, map[string]string{
				"id": membership.OrgID, "name": membership.OrgName,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"requires_organization_selection": true,
			"organizations":                   organizations,
		})
		return
	}
	membership := active[0]
	userID, orgID, name, role := membership.UserID, membership.OrgID, membership.Name, membership.Role

	token, jti, err := middleware.GenerateToken(h.Secret, userID, orgID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	// Track session
	loginBlacklist := service.NewTokenBlacklist(h.RDB)
	loginBlacklist.RegisterSession(ctx, userID, jti)

	slog.Info("auth: login", "email", req.Email)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  req.Email,
			"name":   name,
			"role":   role,
		},
		"onboarding_completed": membership.OnboardingCompleted,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Revoke the current token so it can't be reused
	claims := middleware.GetCurrentUser(r.Context())
	if claims != nil && claims.ID != "" && claims.ExpiresAt != nil {
		blacklist := service.NewTokenBlacklist(h.RDB)
		if err := blacklist.RevokeToken(r.Context(), claims.ID, claims.ExpiresAt.Time); err != nil {
			slog.Error("auth: failed to revoke token on logout", "error", err)
		}
		blacklist.ClearSessions(r.Context(), claims.UserID)
	}
	middleware.ClearTokenCookie(w, h.AppURL)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	ctx := r.Context()

	// Self-hosted: early return if no system email key is available
	if h.StripeKey == "" && !h.ResendSvc.HasSystemKey(ctx) {
		writeError(w, http.StatusServiceUnavailable, "email_not_configured")
		return
	}

	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	resetToken := hex.EncodeToString(tokenBytes)
	expires := time.Now().Add(1 * time.Hour)

	rowsAffected, err := h.Store.SetResetToken(ctx, req.Email, resetToken, expires)
	if err != nil || rowsAffected == 0 {
		// Don't reveal whether email exists
		writeJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
		return
	}

	slog.Info("auth: password reset requested", "email", req.Email)

	// Send reset email via system Resend key
	resetURL := h.AppURL + "/reset-password?token=" + resetToken
	from := h.ResendSvc.GetSystemFrom(ctx)
	if from == "" {
		from = "noreply@inboxes.net"
		slog.Warn("auth: using hardcoded noreply fallback — configure system email in settings")
	}
	if _, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
		"from":    from,
		"to":      []string{req.Email},
		"subject": "Reset your password",
		"html":    "<p>Click <a href=\"" + resetURL + "\">here</a> to reset your password. This link expires in 1 hour.</p>",
	}); err != nil {
		slog.Error("auth: failed to send password reset email", "email", req.Email, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password are required")
		return
	}
	if err := validateNewPassword(r.Context(), req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()
	resetUserIDs, err := h.Store.ResetPasswordMemberships(ctx, string(hash), req.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}

	// Revoke all existing tokens — password reset implies possible compromise
	blacklist := service.NewTokenBlacklist(h.RDB)
	for _, userID := range resetUserIDs {
		if err := blacklist.RevokeAllForUser(ctx, userID); err != nil {
			slog.Error("auth: session revocation failed during password reset", "user_id", userID, "error", err)
		}
		blacklist.ClearSessions(ctx, userID)
		// MCP agent tokens mint fresh JWTs and are not covered by the Redis
		// session blacklist, so revoke every membership's agent credentials.
		if err := h.Store.RevokeAgentTokensForUser(ctx, userID); err != nil {
			slog.Error("auth: agent token revocation failed during password reset", "user_id", userID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

func (h *AuthHandler) Claim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Password != "" {
		if err := validateNewPassword(r.Context(), req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	var hash []byte
	if req.Password != "" {
		generatedHash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		hash = generatedHash
	}

	ctx := r.Context()
	userID, orgID, email, role, err := h.Store.ClaimInvite(ctx, string(hash), req.Name, req.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired invite token")
		return
	}

	token, jti, err := middleware.GenerateToken(h.Secret, userID, orgID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	// Track session
	claimBlacklist := service.NewTokenBlacklist(h.RDB)
	claimBlacklist.RegisterSession(ctx, userID, jti)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  email,
			"role":   role,
		},
	})
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "email and code are required")
		return
	}

	ctx := r.Context()
	userID, orgID, name, role, err := h.Store.VerifyEmail(ctx, req.Email, req.Code)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired verification code")
		return
	}

	token, _, err := middleware.GenerateToken(h.Secret, userID, orgID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  req.Email,
			"name":   name,
			"role":   role,
		},
	})
}

func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	ctx := r.Context()
	code := generateVerificationCode()
	expires := time.Now().Add(15 * time.Minute)

	rowsAffected, err := h.Store.ResendVerificationCode(ctx, req.Email, code, expires)
	if err != nil || rowsAffected == 0 {
		// Don't reveal whether user exists
		writeJSON(w, http.StatusOK, map[string]string{"message": "if that email needs verification, a new code has been sent"})
		return
	}

	from2 := h.ResendSvc.GetSystemFrom(ctx)
	if from2 == "" {
		from2 = "noreply@inboxes.net"
		slog.Warn("auth: using hardcoded noreply fallback — configure system email in settings")
	}
	if _, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
		"from":    from2,
		"to":      []string{req.Email},
		"subject": "Verify your email",
		"html":    fmt.Sprintf("<p>Your verification code is: <strong>%s</strong></p><p>This code expires in 15 minutes.</p>", code),
	}); err != nil {
		slog.Error("auth: failed to resend verification email", "email", req.Email, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send verification email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "if that email needs verification, a new code has been sent"})
}

// generateVerificationCode returns a cryptographically random 6-digit code.
func generateVerificationCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func (h *AuthHandler) ValidateClaim(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	ctx := r.Context()
	email, name, status, hasAccount, err := h.Store.ValidateInviteToken(ctx, token)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusBadRequest, "invalid or expired invite token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to validate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email":       email,
		"name":        name,
		"status":      status,
		"has_account": hasAccount,
	})
}
