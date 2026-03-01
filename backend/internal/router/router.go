package router

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/handler"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/ws"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Secret              string
	AppURL              string
	PublicURL           string
	StripeKey           string
	StripePriceID       string
	StripeWebhookSecret string
	EventCatchupMaxAge  time.Duration
}

func New(db *pgxpool.Pool, rdb *redis.Client, encSvc *service.EncryptionService, resendSvc *service.ResendService, bus *event.Bus, wsHub *ws.Hub, limiterMap *queue.OrgLimiterMap, cfg Config) *chi.Mux {
	secret := cfg.Secret
	appURL := cfg.AppURL
	stripeKey := cfg.StripeKey
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.CORSMiddleware(appURL))
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.ValidateContentType)
	r.Use(chiMiddleware.Compress(5))

	auth := &handler.AuthHandler{DB: db, RDB: rdb, Secret: secret, AppURL: appURL, ResendSvc: resendSvc, StripeKey: stripeKey}
	setup := &handler.SetupHandler{DB: db, EncSvc: encSvc, ResendSvc: resendSvc, Secret: secret, AppURL: appURL, StripeKey: stripeKey}
	threads := &handler.ThreadHandler{DB: db, Bus: bus}
	emails := &handler.EmailHandler{DB: db, ResendSvc: resendSvc, Bus: bus, RDB: rdb}
	webhooks := &handler.WebhookHandler{DB: db, Bus: bus, ResendSvc: resendSvc, RDB: rdb, EncSvc: encSvc}
	onboarding := &handler.OnboardingHandler{DB: db, ResendSvc: resendSvc, EncSvc: encSvc, Bus: bus, PublicURL: cfg.PublicURL}
	users := &handler.UserHandler{DB: db, RDB: rdb, Secret: secret, ResendSvc: resendSvc, AppURL: appURL}
	aliases := &handler.AliasHandler{DB: db}
	domains := &handler.DomainHandler{DB: db, ResendSvc: resendSvc, EncSvc: encSvc, PublicURL: cfg.PublicURL}
	contacts := &handler.ContactHandler{DB: db}
	attachments := &handler.AttachmentHandler{DB: db}
	drafts := &handler.DraftHandler{DB: db, ResendSvc: resendSvc, Bus: bus, RDB: rdb}
	orgs := &handler.OrgHandler{DB: db, RDB: rdb, EncSvc: encSvc, ResendSvc: resendSvc, Bus: bus, StripeKey: stripeKey, LimiterMap: limiterMap}
	labels := &handler.LabelHandler{DB: db}
	syncH := &handler.SyncHandler{DB: db, RDB: rdb}
	events := &handler.EventHandler{DB: db, CatchupMaxAge: cfg.EventCatchupMaxAge}
	cron := &handler.CronHandler{DB: db, ResendSvc: resendSvc}
	billing := &handler.BillingHandler{
		DB:                  db,
		Bus:                 bus,
		StripeKey:           stripeKey,
		StripePriceID:       cfg.StripePriceID,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		AppURL:              appURL,
	}

	// Health
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		dbOK := db.Ping(ctx) == nil
		redisOK := rdb.Ping(ctx).Err() == nil

		status := "ok"
		httpStatus := http.StatusOK
		if !dbOK || !redisOK {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": status,
			"db":     dbOK,
			"redis":  redisOK,
		})
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"api_url":    appURL,
			"ws_url":     wsURL,
			"commercial": stripeKey != "",
		})
	})

	// Self-hosted setup (public, no auth — rate-limited for mutations)
	r.With(middleware.RateLimitByIP(rdb, 60, 60)).Get("/api/setup/status", setup.Status)
	r.With(middleware.RateLimitByIP(rdb, 3, 15*60)).Post("/api/setup", setup.Setup)
	r.With(middleware.RateLimitByIP(rdb, 3, 15*60)).Post("/api/setup/validate-key", setup.ValidateKey)

	// Public auth (always rate-limited)
	r.With(middleware.RateLimitByIP(rdb, 5, 1*60*60)).Post("/api/auth/signup", auth.Signup)
	r.With(middleware.RateLimitByIP(rdb, 10, 15*60), middleware.RateLimitByBodyField(rdb, "email", 10, 15*60)).Post("/api/auth/login", auth.Login)
	r.With(middleware.RateLimitByIP(rdb, 3, 1*60*60), middleware.RateLimitByBodyField(rdb, "email", 3, 1*60*60)).Post("/api/auth/forgot-password", auth.ForgotPassword)
	r.With(middleware.RateLimitByIP(rdb, 5, 15*60)).Post("/api/auth/reset-password", auth.ResetPassword)
	r.With(middleware.RateLimitByIP(rdb, 5, 15*60)).Post("/api/auth/claim", auth.Claim)
	r.With(middleware.RateLimitByIP(rdb, 10, 15*60)).Get("/api/auth/claim/validate", auth.ValidateClaim)
	r.With(middleware.RateLimitByIP(rdb, 5, 15*60), middleware.RateLimitByBodyField(rdb, "email", 5, 15*60)).Post("/api/auth/verify-email", auth.VerifyEmail)
	r.With(middleware.RateLimitByIP(rdb, 3, 1*60*60), middleware.RateLimitByBodyField(rdb, "email", 3, 1*60*60)).Post("/api/auth/resend-verification", auth.ResendVerification)

	// Webhooks (signature-verified, not JWT)
	r.Post("/api/webhooks/resend/{orgId}", webhooks.HandleResend)
	if stripeKey != "" {
		r.Post("/api/webhooks/stripe", billing.HandleStripeWebhook)
	}

	// WebSocket
	r.Get("/api/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(wsHub, secret, appURL, w, r)
	})

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(secret, rdb, db))

		// Always accessible (even without active plan)
		r.Post("/api/auth/logout", auth.Logout)

		// Org settings
		r.Get("/api/orgs/settings", orgs.GetSettings)
		r.With(middleware.RequireAdmin).Patch("/api/orgs/settings", orgs.UpdateSettings)

		// User profile
		r.Get("/api/users/me", users.Me)
		r.Patch("/api/users/me", users.UpdateMe)
		r.With(middleware.RateLimitByIP(rdb, 5, 15*60)).Patch("/api/users/me/password", users.UpdatePassword)
		r.Get("/api/users/me/preferences", users.GetPreferences)
		r.Patch("/api/users/me/preferences", users.UpdatePreferences)
		r.Get("/api/users/me/sessions", users.ListSessions)
		r.Delete("/api/users/me/sessions/{jti}", users.RevokeSession)

		// Billing
		r.Get("/api/billing", billing.GetBilling)
		r.With(middleware.RequireAdmin).Post("/api/billing/checkout", billing.Checkout)
		r.With(middleware.RequireAdmin).Post("/api/billing/portal", billing.Portal)

		// System settings (self-hosted owner only)
		if stripeKey == "" {
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOwner(db))
				r.Get("/api/system/email", setup.GetSystemEmail)
				r.Patch("/api/system/email", setup.UpdateSystemEmail)
			})
		}

		// Sync jobs
		r.With(middleware.RequireAdmin).Post("/api/sync", syncH.StartSync)
		r.Get("/api/sync/{id}", syncH.GetSync)

		// Events catchup (for WS reconnection)
		r.Get("/api/events", events.Since)

		// Admin-only routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin)
			r.Use(middleware.RateLimitByIP(rdb, 5, 60))
			r.Post("/api/cron/purge-trash", cron.PurgeTrash)
			r.Post("/api/cron/cleanup-webhooks", cron.CleanupWebhooks)
			r.Get("/api/admin/jobs", emails.AdminJobs)
		})

		// Require active plan for feature routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePlan(stripeKey, db))

			// Onboarding
			r.Get("/api/onboarding/status", onboarding.Status)
			r.Post("/api/onboarding/connect", onboarding.Connect)
			r.Post("/api/onboarding/domains", onboarding.SelectDomains)
			r.Post("/api/onboarding/webhook", onboarding.SetupWebhook)
			r.Get("/api/onboarding/addresses", onboarding.GetAddresses)
			r.Post("/api/onboarding/addresses", onboarding.SetupAddresses)
			r.Post("/api/onboarding/complete", onboarding.Complete)

			// Threads
			r.Get("/api/threads", threads.List)
			r.Patch("/api/threads/bulk", threads.BulkAction)
			r.Get("/api/threads/{id}", threads.Get)
			r.Patch("/api/threads/{id}/read", threads.MarkRead)
			r.Patch("/api/threads/{id}/unread", threads.MarkUnread)
			r.Patch("/api/threads/{id}/star", threads.Star)
			r.Patch("/api/threads/{id}/archive", threads.Archive)
			r.Patch("/api/threads/{id}/trash", threads.Trash)
			r.Patch("/api/threads/{id}/spam", threads.Spam)
			r.Patch("/api/threads/{id}/mute", threads.Mute)
			r.Patch("/api/threads/{id}/move", threads.Move)
			r.Delete("/api/threads/{id}", threads.Delete)

			// Emails
			r.With(middleware.RateLimitByIP(rdb, 20, 60), middleware.RateLimitByUser(rdb, 30, 60)).Post("/api/emails/send", emails.Send)
			r.Get("/api/emails/search", emails.Search)

			// Domains
			r.Get("/api/domains", domains.List)
			r.Get("/api/domains/all", domains.ListAll)
			r.Post("/api/domains", domains.Create)
			r.Post("/api/domains/{id}/verify", domains.Verify)
			r.With(middleware.RequireAdmin).Post("/api/domains/{id}/webhook", domains.ReregisterWebhook)
			r.With(middleware.RequireAdmin).Delete("/api/domains/{id}", domains.Delete)
			r.Patch("/api/domains/reorder", domains.Reorder)
			r.With(middleware.RequireAdmin).Patch("/api/domains/visibility", domains.UpdateVisibility)
			r.Get("/api/domains/unread-counts", domains.UnreadCounts)
			r.Post("/api/domains/sync", domains.Sync)

			r.With(middleware.RequireAdmin).Get("/api/users", users.List)
			r.With(middleware.RequireAdmin).Post("/api/users/invite", users.Invite)
			r.With(middleware.RequireAdmin).Post("/api/users/{id}/reinvite", users.Reinvite)
			r.With(middleware.RequireAdmin).Patch("/api/users/{id}/disable", users.Disable)
			r.With(middleware.RequireAdmin).Patch("/api/users/{id}/role", users.ChangeRole)
			r.With(middleware.RequireAdmin).Patch("/api/users/{id}/enable", users.Enable)
			r.Get("/api/users/me/aliases", users.MyAliases)

			// Org delete
			r.Delete("/api/orgs", orgs.Delete)
			r.Delete("/api/orgs/hard", orgs.HardDelete)

			// Aliases
			r.Get("/api/aliases", aliases.List)
			r.With(middleware.RequireAdmin).Post("/api/aliases", aliases.Create)
			r.With(middleware.RequireAdmin).Patch("/api/aliases/{id}", aliases.Update)
			r.With(middleware.RequireAdmin).Delete("/api/aliases/{id}", aliases.Delete)
			r.With(middleware.RequireAdmin).Post("/api/aliases/{id}/users", aliases.AddUser)
			r.With(middleware.RequireAdmin).Delete("/api/aliases/{id}/users/{userId}", aliases.RemoveUser)
			r.Patch("/api/aliases/{id}/default", aliases.SetDefault)
			r.Get("/api/aliases/discovered", aliases.DiscoveredAddresses)

			// Drafts
			r.Get("/api/drafts", drafts.List)
			r.Post("/api/drafts", drafts.Create)
			r.Patch("/api/drafts/{id}", drafts.Update)
			r.Delete("/api/drafts/{id}", drafts.Delete)
			r.With(middleware.RateLimitByIP(rdb, 20, 60), middleware.RateLimitByUser(rdb, 30, 60)).Post("/api/drafts/{id}/send", drafts.Send)

			// Labels
			r.Get("/api/labels", labels.List)
			r.Post("/api/labels", labels.Create)
			r.Patch("/api/labels/{id}", labels.Rename)
			r.Delete("/api/labels/{id}", labels.Delete)

			// Contacts
			r.Get("/api/contacts/suggest", contacts.Suggest)

			// Attachments
			r.Post("/api/attachments/upload", attachments.Upload)
			r.Get("/api/attachments/{id}/meta", attachments.Meta)
			r.Get("/api/attachments/{id}", attachments.Download)
		})
	})

	return r
}
