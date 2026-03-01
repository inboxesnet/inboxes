package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type SetupHandler struct {
	DB        *pgxpool.Pool
	EncSvc    *service.EncryptionService
	ResendSvc *service.ResendService
	Secret    string
	AppURL    string
	StripeKey string
}

// Status returns whether the self-hosted instance needs initial setup.
func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.StripeKey != "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"needs_setup":             false,
			"commercial":              true,
			"system_email_configured": true,
		})
		return
	}

	ctx := r.Context()
	var count int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		slog.Error("setup: failed to count users", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"needs_setup":             count == 0,
		"commercial":              false,
		"system_email_configured": h.ResendSvc.HasSystemKey(ctx),
	})
}

// ValidateKey checks a Resend API key and returns available domains.
func (h *SetupHandler) ValidateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := readJSON(r, &req); err != nil || req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	data, err := service.ResendDirectFetch(req.APIKey, "GET", "/domains", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":       false,
			"full_access": false,
			"domains":     []interface{}{},
			"error":       "Failed to fetch domains. Key may be send-only or invalid.",
		})
		return
	}

	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":       true,
			"full_access": false,
			"domains":     []interface{}{},
			"error":       "Key accepted but could not parse domain list.",
		})
		return
	}

	domains := make([]map[string]string, 0, len(resp.Data))
	for _, d := range resp.Data {
		domains = append(domains, map[string]string{
			"id":     d.ID,
			"name":   d.Name,
			"status": d.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":       true,
		"full_access": true,
		"domains":     domains,
	})
}

// Setup creates the first admin account on a self-hosted instance.
func (h *SetupHandler) Setup(w http.ResponseWriter, r *http.Request) {
	if h.StripeKey != "" {
		writeError(w, http.StatusForbidden, "setup not available in commercial mode")
		return
	}

	ctx := r.Context()

	// Guard: already set up
	var count int
	if err := h.DB.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		slog.Error("setup: failed to count users", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	if count > 0 {
		writeError(w, http.StatusForbidden, "setup already completed")
		return
	}

	var req struct {
		Name              string `json:"name"`
		Email             string `json:"email"`
		Password          string `json:"password"`
		SystemResendKey   string `json:"system_resend_key"`
		SystemFromAddress string `json:"system_from_address"`
		SystemFromName    string `json:"system_from_name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
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

	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := h.DB.Begin(dbCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(dbCtx)

	// Create org
	var orgID string
	if err := tx.QueryRow(dbCtx,
		"INSERT INTO orgs (name) VALUES ($1) RETURNING id", name+"'s Org",
	).Scan(&orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create org")
		return
	}

	// Create admin user with is_owner = true
	var userID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO users (org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, 'admin', 'active', true, true)
		 RETURNING id`,
		orgID, req.Email, name, string(hash),
	).Scan(&userID); err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Store system Resend key if provided
	if req.SystemResendKey != "" {
		encrypted, iv, tag, err := h.EncSvc.Encrypt(req.SystemResendKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt system key")
			return
		}
		if _, err := tx.Exec(dbCtx,
			`INSERT INTO system_settings (key, value, iv, tag, encrypted)
			 VALUES ('resend_system_api_key', $1, $2, $3, true)`,
			encrypted, iv, tag,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store system key")
			return
		}
	}

	// Store from address/name if provided (plain text, not encrypted)
	if req.SystemFromAddress != "" {
		if _, err := tx.Exec(dbCtx,
			`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_address', $1, false)`,
			req.SystemFromAddress,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store from address")
			return
		}
	}
	if req.SystemFromName != "" {
		if _, err := tx.Exec(dbCtx,
			`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_name', $1, false)`,
			req.SystemFromName,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store from name")
			return
		}
	}

	if err := tx.Commit(dbCtx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	slog.Info("setup: admin account created", "email", req.Email, "org_id", orgID)

	// Set JWT cookie so user goes straight to onboarding without login
	token, _, err := middleware.GenerateToken(h.Secret, userID, orgID, "admin")
	if err != nil {
		slog.Error("setup: failed to generate token", "error", err)
	} else {
		middleware.SetTokenCookie(w, token, h.AppURL)
	}

	// Invalidate caches so new values are picked up
	h.ResendSvc.InvalidateSystemKeyCache()
	h.ResendSvc.InvalidateFromCache()

	// Send welcome email if key + from address are configured
	if req.SystemResendKey != "" && req.SystemFromAddress != "" {
		fromAddr := req.SystemFromAddress
		fromName := req.SystemFromName
		var from string
		if fromName != "" {
			from = fromName + " <" + fromAddr + ">"
		} else {
			from = fromAddr
		}

		util.SafeGo("setup-welcome-email", func() {
			h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
				"from":    from,
				"to":      []string{req.Email},
				"subject": "Welcome to Inboxes",
				"html":    welcomeEmailHTML(name, h.AppURL),
			})
		})
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "setup completed",
	})
}

// GetSystemEmail returns the current system from-address configuration.
func (h *SetupHandler) GetSystemEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from_address": h.ResendSvc.GetSystemFromAddress(ctx),
		"from_name":    h.ResendSvc.GetSystemFromName(ctx),
	})
}

// UpdateSystemEmail updates the system from-address configuration.
func (h *SetupHandler) UpdateSystemEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		FromAddress string `json:"from_address"`
		FromName    string `json:"from_name"`
		SendTest    bool   `json:"send_test"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FromAddress != "" {
		if err := validateEmail(req.FromAddress); err != nil {
			writeError(w, http.StatusBadRequest, "invalid from address")
			return
		}
	}
	if err := validateLength(req.FromName, "from name", 255); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// UPSERT from address
	if _, err := h.DB.Exec(ctx,
		`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_address', $1, false)
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`,
		req.FromAddress,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save from address")
		return
	}

	// UPSERT from name
	if _, err := h.DB.Exec(ctx,
		`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_name', $1, false)
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`,
		req.FromName,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save from name")
		return
	}

	h.ResendSvc.InvalidateFromCache()

	// Send test email if requested
	if req.SendTest {
		claims := middleware.GetCurrentUser(ctx)
		if claims != nil {
			var email string
			warnIfErr(h.DB.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", claims.UserID).Scan(&email),
				"setup: failed to look up user email for test")
			if email != "" {
				from := h.ResendSvc.GetSystemFrom(ctx)
				if from != "" {
					_, err := h.ResendSvc.SystemFetch(ctx, "POST", "/emails", map[string]interface{}{
						"from":    from,
						"to":      []string{email},
						"subject": "Inboxes Test Email",
						"html":    "<p>This is a test email from your Inboxes instance. If you received this, your system email is configured correctly.</p>",
					})
					if err != nil {
						writeJSON(w, http.StatusOK, map[string]interface{}{
							"saved":      true,
							"test_sent":  false,
							"test_error": err.Error(),
						})
						return
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"saved":     true,
			"test_sent": true,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"saved": true,
	})
}

func welcomeEmailHTML(name, appURL string) string {
	if name == "" {
		name = "there"
	}
	return fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 480px; margin: 0 auto; padding: 20px;">
  <h2>Welcome to Inboxes, %s!</h2>
  <p>Your instance is set up and ready to go. Here are some next steps:</p>
  <ul>
    <li><strong>Log in</strong> at <a href="%s/login">%s/login</a></li>
    <li><strong>Connect your Resend account</strong> during onboarding to start receiving email</li>
    <li><strong>Add domains</strong> and create aliases for your team</li>
    <li><strong>Invite team members</strong> from Settings &rarr; Team</li>
  </ul>
  <p>This email confirms your system email is working correctly.</p>
  <p style="color: #666; font-size: 13px;">— Inboxes</p>
</div>`, name, appURL, appURL)
}
