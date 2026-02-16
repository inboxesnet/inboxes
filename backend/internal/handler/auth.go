package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB        *pgxpool.Pool
	Secret    string
	AppURL    string
	ResendSvc *service.ResendService
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
	var req signupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.OrgName == "" {
		writeError(w, http.StatusBadRequest, "email, password, and org_name are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var orgID string
	err = tx.QueryRow(ctx,
		"INSERT INTO orgs (name) VALUES ($1) RETURNING id", req.OrgName,
	).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create org")
		return
	}

	var userID string
	name := req.Name
	if name == "" {
		name = strings.Split(req.Email, "@")[0]
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role, status)
		 VALUES ($1, $2, $3, $4, 'admin', 'active') RETURNING id`,
		orgID, req.Email, name, string(hash),
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	token, err := middleware.GenerateToken(h.Secret, userID, orgID, "admin")
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

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	ctx := r.Context()
	var userID, orgID, name, role, passwordHash string
	var status string
	err := h.DB.QueryRow(ctx,
		`SELECT id, org_id, name, role, status, password_hash FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &orgID, &name, &role, &status, &passwordHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if status != "active" {
		writeError(w, http.StatusUnauthorized, "account is not active")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := middleware.GenerateToken(h.Secret, userID, orgID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	var onboardingCompleted bool
	h.DB.QueryRow(ctx, "SELECT onboarding_completed FROM orgs WHERE id = $1", orgID).Scan(&onboardingCompleted)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  req.Email,
			"name":   name,
			"role":   role,
		},
		"onboarding_completed": onboardingCompleted,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.ClearTokenCookie(w, h.AppURL)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	ctx := r.Context()
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	resetToken := hex.EncodeToString(tokenBytes)
	expires := time.Now().Add(1 * time.Hour)

	tag, err := h.DB.Exec(ctx,
		`UPDATE users SET reset_token = $1, reset_expires_at = $2 WHERE email = $3 AND status = 'active'`,
		resetToken, expires, req.Email,
	)
	if err != nil || tag.RowsAffected() == 0 {
		// Don't reveal whether email exists
		writeJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
		return
	}

	// Send reset email via system Resend key
	resetURL := h.AppURL + "/reset-password?token=" + resetToken
	h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
		"from":    "noreply@inboxes.app",
		"to":      []string{req.Email},
		"subject": "Reset your password",
		"html":    "<p>Click <a href=\"" + resetURL + "\">here</a> to reset your password. This link expires in 1 hour.</p>",
	})

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
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()
	tag, err := h.DB.Exec(ctx,
		`UPDATE users SET password_hash = $1, reset_token = NULL, reset_expires_at = NULL
		 WHERE reset_token = $2 AND reset_expires_at > now()`,
		string(hash), req.Token,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

func (h *AuthHandler) Claim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()
	name := req.Name
	var userID, orgID, email, role string
	err = h.DB.QueryRow(ctx,
		`UPDATE users SET password_hash = $1, name = CASE WHEN $2 = '' THEN name ELSE $2 END,
		 status = 'active', invite_token = NULL, invite_expires_at = NULL
		 WHERE invite_token = $3 AND invite_expires_at > now() AND status IN ('placeholder', 'invited')
		 RETURNING id, org_id, email, role`,
		string(hash), name, req.Token,
	).Scan(&userID, &orgID, &email, &role)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired invite token")
		return
	}

	token, err := middleware.GenerateToken(h.Secret, userID, orgID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetTokenCookie(w, token, h.AppURL)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]string{
			"id":     userID,
			"org_id": orgID,
			"email":  email,
			"role":   role,
		},
	})
}

func (h *AuthHandler) ValidateClaim(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	ctx := r.Context()
	var email, name, status string
	err := h.DB.QueryRow(ctx,
		`SELECT email, name, status FROM users
		 WHERE invite_token = $1 AND invite_expires_at > now()`,
		token,
	).Scan(&email, &name, &status)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusBadRequest, "invalid or expired invite token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to validate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"email":  email,
		"name":   name,
		"status": status,
	})
}
