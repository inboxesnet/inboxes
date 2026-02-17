package router

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/ws"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Secret              string
	AppURL              string
	StripeKey           string
	StripePriceID       string
	StripeWebhookSecret string
}

func New(db *pgxpool.Pool, rdb *redis.Client, encSvc *service.EncryptionService, resendSvc *service.ResendService, bus *event.Bus, wsHub *ws.Hub, cfg Config) *chi.Mux {
	secret := cfg.Secret
	appURL := cfg.AppURL
	stripeKey := cfg.StripeKey
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.CORSMiddleware(appURL))
	r.Use(chiMiddleware.Recoverer)

	auth := &handler.AuthHandler{DB: db, Secret: secret, AppURL: appURL, ResendSvc: resendSvc, StripeKey: stripeKey}
	threads := &handler.ThreadHandler{DB: db, Bus: bus}
	emails := &handler.EmailHandler{DB: db, ResendSvc: resendSvc, Bus: bus}
	webhooks := &handler.WebhookHandler{DB: db, Bus: bus}
	onboarding := &handler.OnboardingHandler{DB: db, ResendSvc: resendSvc, EncSvc: encSvc, Bus: bus}
	users := &handler.UserHandler{DB: db, ResendSvc: resendSvc}
	aliases := &handler.AliasHandler{DB: db}
	domains := &handler.DomainHandler{DB: db, ResendSvc: resendSvc}
	contacts := &handler.ContactHandler{DB: db}
	attachments := &handler.AttachmentHandler{DB: db}
	drafts := &handler.DraftHandler{DB: db, ResendSvc: resendSvc, Bus: bus}
	orgs := &handler.OrgHandler{DB: db, EncSvc: encSvc, ResendSvc: resendSvc, Bus: bus, StripeKey: stripeKey}
	syncH := &handler.SyncHandler{DB: db, RDB: rdb}
	events := &handler.EventHandler{DB: db}
	cron := &handler.CronHandler{DB: db, ResendSvc: resendSvc}
	billing := &handler.BillingHandler{
		DB:                  db,
		StripeKey:           stripeKey,
		StripePriceID:       cfg.StripePriceID,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		AppURL:              appURL,
	}

	// Health
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public runtime config (self-host: no rebuild needed per deployment)
	r.Get("/api/config", func(w http.ResponseWriter, r *http.Request) {
		wsURL := appURL
		if len(wsURL) > 4 && wsURL[:5] == "https" {
			wsURL = "wss" + wsURL[5:]
		} else if len(wsURL) > 4 && wsURL[:4] == "http" {
			wsURL = "ws" + wsURL[4:]
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(map[string]string{
			"api_url": appURL,
			"ws_url":  wsURL,
		})
	})

	// Public auth (with optional rate limiting when hosted)
	if stripeKey != "" {
		r.With(middleware.RateLimitByIP(rdb, 5, 1*60*60)).Post("/api/auth/signup", auth.Signup)
		r.With(middleware.RateLimitByIP(rdb, 10, 15*60)).Post("/api/auth/login", auth.Login)
		r.With(middleware.RateLimitByIP(rdb, 3, 1*60*60)).Post("/api/auth/forgot-password", auth.ForgotPassword)
	} else {
		r.Post("/api/auth/signup", auth.Signup)
		r.Post("/api/auth/login", auth.Login)
		r.Post("/api/auth/forgot-password", auth.ForgotPassword)
	}
	r.Post("/api/auth/reset-password", auth.ResetPassword)
	r.Post("/api/auth/claim", auth.Claim)
	r.Get("/api/auth/claim/validate", auth.ValidateClaim)
	r.Post("/api/auth/verify-email", auth.VerifyEmail)
	r.Post("/api/auth/resend-verification", auth.ResendVerification)

	// Webhooks (signature-verified, not JWT)
	r.Post("/api/webhooks/resend/{orgId}", webhooks.HandleResend)
	if stripeKey != "" {
		r.Post("/api/webhooks/stripe", billing.HandleStripeWebhook)
	}

	// WebSocket
	r.Get("/api/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(wsHub, secret, w, r)
	})

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(secret))

		// Always accessible (even without active plan)
		r.Post("/api/auth/logout", auth.Logout)

		// Onboarding
		r.Get("/api/onboarding/status", onboarding.Status)
		r.Post("/api/onboarding/connect", onboarding.Connect)
		r.Post("/api/onboarding/domains", onboarding.SelectDomains)
		r.Post("/api/onboarding/webhook", onboarding.SetupWebhook)
		r.Get("/api/onboarding/addresses", onboarding.GetAddresses)
		r.Post("/api/onboarding/addresses", onboarding.SetupAddresses)
		r.Post("/api/onboarding/complete", onboarding.Complete)

		// Org settings
		r.Get("/api/orgs/settings", orgs.GetSettings)
		r.Patch("/api/orgs/settings", orgs.UpdateSettings)

		// User profile
		r.Get("/api/users/me", users.Me)
		r.Patch("/api/users/me", users.UpdateMe)
		r.Patch("/api/users/me/password", users.UpdatePassword)
		r.Get("/api/users/me/preferences", users.GetPreferences)
		r.Patch("/api/users/me/preferences", users.UpdatePreferences)

		// Billing
		r.Get("/api/billing", billing.GetBilling)
		r.Post("/api/billing/checkout", billing.Checkout)
		r.Post("/api/billing/portal", billing.Portal)

		// Sync jobs
		r.Post("/api/sync", syncH.StartSync)
		r.Get("/api/sync/{id}", syncH.GetSync)

		// Events catchup (for WS reconnection)
		r.Get("/api/events", events.Since)

		// Cron
		r.Post("/api/cron/purge-trash", cron.PurgeTrash)
		r.Post("/api/cron/cleanup-webhooks", cron.CleanupWebhooks)

		// Require active plan for feature routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePlan(stripeKey, db))

			// Threads
			r.Get("/api/threads", threads.List)
			r.Get("/api/threads/unread-count", threads.UnreadCount)
			r.Patch("/api/threads/bulk", threads.BulkAction)
			r.Get("/api/threads/{id}", threads.Get)
			r.Patch("/api/threads/{id}/read", threads.MarkRead)
			r.Patch("/api/threads/{id}/unread", threads.MarkUnread)
			r.Patch("/api/threads/{id}/star", threads.Star)
			r.Patch("/api/threads/{id}/archive", threads.Archive)
			r.Patch("/api/threads/{id}/trash", threads.Trash)
			r.Patch("/api/threads/{id}/spam", threads.Spam)
			r.Patch("/api/threads/{id}/move", threads.Move)
			r.Delete("/api/threads/{id}", threads.Delete)

			// Emails
			r.Post("/api/emails/send", emails.Send)
			r.Get("/api/emails/search", emails.Search)

			// Domains
			r.Get("/api/domains", domains.List)
			r.Get("/api/domains/all", domains.ListAll)
			r.Post("/api/domains", domains.Create)
			r.Post("/api/domains/{id}/verify", domains.Verify)
			r.Patch("/api/domains/reorder", domains.Reorder)
			r.Patch("/api/domains/visibility", domains.UpdateVisibility)
			r.Get("/api/domains/unread-counts", domains.UnreadCounts)
			r.Post("/api/domains/sync", domains.Sync)

			// Users (management)
			r.Get("/api/users", users.List)
			r.Post("/api/users/invite", users.Invite)
			r.Get("/api/users/{id}/reinvite", users.Reinvite)
			r.Patch("/api/users/{id}/disable", users.Disable)
			r.Get("/api/users/me/aliases", users.MyAliases)

			// Aliases
			r.Get("/api/aliases", aliases.List)
			r.Post("/api/aliases", aliases.Create)
			r.Delete("/api/aliases/{id}", aliases.Delete)
			r.Post("/api/aliases/{id}/users", aliases.AddUser)
			r.Delete("/api/aliases/{id}/users/{userId}", aliases.RemoveUser)

			// Drafts
			r.Get("/api/drafts", drafts.List)
			r.Post("/api/drafts", drafts.Create)
			r.Patch("/api/drafts/{id}", drafts.Update)
			r.Delete("/api/drafts/{id}", drafts.Delete)
			r.Post("/api/drafts/{id}/send", drafts.Send)

			// Contacts
			r.Get("/api/contacts/suggest", contacts.Suggest)
			r.Get("/api/contacts/lookup", contacts.Lookup)

			// Attachments
			r.Post("/api/attachments/upload", attachments.Upload)
			r.Get("/api/attachments/{id}", attachments.Download)
		})
	})

	return r
}
