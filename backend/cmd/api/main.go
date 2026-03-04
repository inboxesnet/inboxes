package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"

	"github.com/inboxes/backend/internal/config"
	"github.com/inboxes/backend/internal/db"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/queue"
	"github.com/inboxes/backend/internal/router"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
	"github.com/inboxes/backend/internal/util"
	"github.com/inboxes/backend/internal/worker"
	"github.com/inboxes/backend/internal/ws"
)

func main() {
	// Load .env file if present (ignored in production)
	// Try root .env first (where setup.sh creates it), then CWD
	_ = godotenv.Load("../.env")
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Migrations
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("failed to set goose dialect", "error", err)
		os.Exit(1)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// Store
	st := store.NewPgStore(pool)

	// Services
	encSvc, err := service.NewEncryptionService(cfg.EncryptionKey)
	if err != nil {
		slog.Error("failed to init encryption", "error", err)
		os.Exit(1)
	}
	resendSvc := service.NewResendService(encSvc, pool, cfg.ResendSystemKey, cfg.SystemFromAddress)

	// Event Bus
	bus := event.NewBus(pool, rdb)

	// Per-org rate limiter map for Resend API calls (default 2 RPS per org)
	orgLimiterMap := queue.NewOrgLimiterMap(pool, 2)

	// Sync service + worker (uses per-org rate limiter)
	syncSvc := service.NewSyncService(pool, resendSvc, bus, orgLimiterMap)
	syncWorker := worker.NewSyncWorker(pool, rdb, syncSvc, resendSvc, bus)
	util.SafeGo("sync-worker", func() { syncWorker.Run(ctx) })
	util.SafeGo("sync-stale-recovery", func() { syncWorker.RunStaleRecovery(ctx) })

	// Email worker
	emailWorker := queue.NewEmailWorker(st, rdb, resendSvc, bus, orgLimiterMap, cfg.StripeKey)
	util.SafeGo("email-worker", func() { emailWorker.Run(ctx) })
	util.SafeGo("email-stale-recovery", func() { emailWorker.RunStaleRecovery(ctx) })

	// Trash collector
	trashCollector := worker.NewTrashCollector(pool, bus, cfg.TrashCollectorEnabled, cfg.TrashCollectorInterval)
	util.SafeGo("trash-collector", func() { trashCollector.Run(ctx) })

	// Domain heartbeat (checks Resend periodically)
	domainHeartbeat := worker.NewDomainHeartbeat(pool, resendSvc, bus, cfg.DomainHeartbeatInterval)
	util.SafeGo("domain-heartbeat", func() { domainHeartbeat.Run(ctx) })

	// Event pruner (removes events older than retention period)
	eventPruner := worker.NewEventPruner(pool, cfg.EventRetentionDays, cfg.EventPrunerInterval)
	util.SafeGo("event-pruner", func() { eventPruner.Run(ctx) })

	// Grace period worker (transitions expired cancelled/past_due plans to free)
	gracePeriodWorker := worker.NewGracePeriodWorker(pool, bus, cfg.GracePeriodInterval)
	util.SafeGo("grace-period", func() { gracePeriodWorker.Run(ctx) })

	// Stripe event dedup pruner (removes events older than 7 days)
	stripeEventPruner := worker.NewStripeEventPruner(pool, cfg.StripeEventPrunerInterval)
	util.SafeGo("stripe-event-pruner", func() { stripeEventPruner.Run(ctx) })

	// Status recovery (polls Resend for stale outbound email statuses)
	statusRecovery := worker.NewStatusRecovery(pool, resendSvc, orgLimiterMap, cfg.StatusRecoveryInterval)
	util.SafeGo("status-recovery", func() { statusRecovery.Run(ctx) })

	// Inbox poller (auto-sync for self-hosted / no-webhook environments)
	inboxPoller := worker.NewInboxPoller(st, rdb, resendSvc, orgLimiterMap)
	util.SafeGo("inbox-poller", func() { inboxPoller.Run(ctx) })

	// WebSocket Hub
	wsHub := ws.NewHub(rdb, st, cfg.WSMaxConnsPerUser, cfg.WSTokenCheckInterval)
	util.SafeGo("ws-hub", func() { wsHub.Run(ctx) })

	// Router
	r := router.New(pool, rdb, encSvc, resendSvc, bus, wsHub, orgLimiterMap, router.Config{
		Secret:              cfg.SessionSecret,
		AppURL:              cfg.AppURL,
		PublicURL:           cfg.PublicURL,
		StripeKey:           cfg.StripeKey,
		StripePriceID:       cfg.StripePriceID,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		EventCatchupMaxAge:  cfg.EventCatchupMaxAge,
		AppCtx:              ctx,
	})

	// Server
	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	util.SafeGo("signal-handler", func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
		cancel()
	})

	slog.Info("server starting", "port", cfg.APIPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
