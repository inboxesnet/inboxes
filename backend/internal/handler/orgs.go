package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrgHandler struct {
	DB        *pgxpool.Pool
	EncSvc    *service.EncryptionService
	ResendSvc *service.ResendService
	SyncSvc   *service.SyncService
	Bus       *event.Bus
}

func (h *OrgHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var name string
	var onboardingCompleted bool
	var hasAPIKey bool

	err := h.DB.QueryRow(r.Context(),
		`SELECT name, onboarding_completed, (resend_api_key_encrypted IS NOT NULL) as has_key
		 FROM orgs WHERE id = $1`, claims.OrgID,
	).Scan(&name, &onboardingCompleted, &hasAPIKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   claims.OrgID,
		"name":                 name,
		"onboarding_completed": onboardingCompleted,
		"has_api_key":          hasAPIKey,
	})
}

func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}

	var req struct {
		Name   string `json:"name"`
		APIKey string `json:"api_key"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.Name != "" {
		h.DB.Exec(r.Context(),
			`UPDATE orgs SET name = $1, updated_at = now() WHERE id = $2`,
			req.Name, claims.OrgID)
	}

	if req.APIKey != "" {
		ciphertext, iv, tag, err := h.EncSvc.Encrypt(req.APIKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
			return
		}
		h.DB.Exec(r.Context(),
			`UPDATE orgs SET resend_api_key_encrypted = $1, resend_api_key_iv = $2, resend_api_key_tag = $3, updated_at = now() WHERE id = $4`,
			ciphertext, iv, tag, claims.OrgID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncStream re-runs the email sync with SSE progress — callable anytime from settings.
func (h *OrgHandler) SyncStream(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Use request context only for the initial domain query.
	// The sync itself gets a background context so it runs to completion
	// even if the user closes the browser tab.
	rows, err := h.DB.Query(r.Context(),
		"SELECT id, domain FROM domains WHERE org_id = $1 AND hidden = false", claims.OrgID,
	)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to fetch domains\"}\n\n")
		flusher.Flush()
		return
	}
	defer rows.Close()

	domains := make(map[string]string)
	for rows.Next() {
		var id, domain string
		rows.Scan(&id, &domain)
		domains[domain] = id
	}

	if h.SyncSvc == nil {
		h.SyncSvc = service.NewSyncService(h.DB, h.ResendSvc, h.Bus)
	}

	progress := make(chan service.SyncProgress, 10)

	// Sync runs on a background context — not tied to the HTTP request.
	go func() {
		bgCtx := context.Background()
		_, err := h.SyncSvc.SyncEmailsWithProgress(bgCtx, claims.OrgID, claims.UserID, domains, progress)
		if err != nil {
			slog.Error("sync: background sync failed", "error", err)
		}
		close(progress)
	}()

	// Stream progress to the client. If client disconnects, we just
	// drain the channel — the sync goroutine keeps running.
	clientGone := r.Context().Done()
	for p := range progress {
		select {
		case <-clientGone:
			// Client disconnected. Drain remaining progress events
			// so the sync goroutine doesn't block on channel sends.
			for range progress {
			}
			slog.Info("sync: client disconnected, sync continues in background")
			return
		default:
			data, _ := json.Marshal(p)
			if p.Phase == "done" {
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			} else {
				fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			}
			flusher.Flush()
		}
	}
}
