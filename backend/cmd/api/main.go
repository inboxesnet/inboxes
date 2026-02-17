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
	"github.com/inboxes/backend/internal/router"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/worker"
	"github.com/inboxes/backend/internal/ws"
)

func main() {
	// Load .env file if present (ignored in production)
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

	// Services
	encSvc, err := service.NewEncryptionService(cfg.EncryptionKey)
	if err != nil {
		slog.Error("failed to init encryption", "error", err)
		os.Exit(1)
	}
	resendSvc := service.NewResendService(encSvc, pool, cfg.ResendSystemKey)

	// Event Bus
	bus := event.NewBus(pool, rdb)

	// Sync service + worker
	syncSvc := service.NewSyncService(pool, resendSvc, bus)
	syncWorker := worker.NewSyncWorker(pool, rdb, syncSvc, resendSvc, bus)
	go syncWorker.Run(ctx)
	go syncWorker.RunStaleRecovery(ctx)

	// WebSocket Hub
	wsHub := ws.NewHub(rdb)
	go wsHub.Run(ctx)

	// Router
	r := router.New(pool, rdb, encSvc, resendSvc, bus, wsHub, router.Config{
		Secret:              cfg.SessionSecret,
		AppURL:              cfg.AppURL,
		StripeKey:           cfg.StripeKey,
		StripePriceID:       cfg.StripePriceID,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
	})

	// Server
	srv := &http.Server{
		Addr:    ":" + cfg.APIPort,
		Handler: r,
	}

	go func() {
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
	}()

	slog.Info("server starting", "port", cfg.APIPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
