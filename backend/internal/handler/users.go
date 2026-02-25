package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	DB        *pgxpool.Pool
	ResendSvc *service.ResendService
	AppURL    string
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

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
	if err := readJSON(r, &req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
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

	// Send invite email via system Resend key
	h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", []byte(fmt.Sprintf(
		`{"from":"noreply@inboxes.app","to":"%s","subject":"You're invited to Inboxes","html":"<p>You've been invited to join an Inboxes workspace.</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>"}`,
		req.Email, h.AppURL, token)))

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

	h.ResendSvc.SystemFetch(r.Context(), "POST", "/emails", []byte(fmt.Sprintf(
		`{"from":"noreply@inboxes.app","to":"%s","subject":"Reminder: You're invited to Inboxes","html":"<p>You've been invited to join an Inboxes workspace.</p><p><a href='%s/claim?token=%s'>Accept Invitation</a></p>"}`,
		email, h.AppURL, token)))

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

	tag, err := h.DB.Exec(r.Context(),
		`UPDATE users SET status = 'disabled', updated_at = now() WHERE id = $1 AND org_id = $2`,
		userID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var id, email, name, role, status string
	var createdAt time.Time
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, email, name, role, status, created_at FROM users WHERE id = $1`,
		claims.UserID).Scan(&id, &email, &name, &role, &status, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "email": email, "name": name, "role": role,
		"status": status, "created_at": createdAt,
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

	h.DB.Exec(r.Context(),
		`UPDATE users SET name = $1, updated_at = now() WHERE id = $2`,
		req.Name, claims.UserID)

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

	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
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

	h.DB.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		string(newHash), claims.UserID)

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

	h.DB.Exec(r.Context(),
		`UPDATE users SET notification_preferences = $1, updated_at = now() WHERE id = $2`,
		prefs, claims.UserID)

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) MyAliases(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT a.id, a.address, a.name, a.domain_id, au.can_send_as, au.is_default
		 FROM aliases a
		 JOIN alias_users au ON au.alias_id = a.id
		 WHERE au.user_id = $1 AND a.org_id = $2
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

func generateToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
