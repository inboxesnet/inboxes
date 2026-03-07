package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL       string
	RedisURL          string
	EncryptionKey     string
	ResendSystemKey   string
	SessionSecret     string
	AppURL            string
	PublicURL         string
	APIPort           string

	// Stripe (optional — when empty, billing is disabled for self-hosted)
	StripeKey           string
	StripeWebhookSecret string
	StripePriceID       string

	// System email sender (required in commercial mode)
	SystemFromAddress string

	EventRetentionDays int

	// Worker intervals (configurable via env vars, Go duration strings)
	DomainHeartbeatInterval  time.Duration
	TrashCollectorInterval   time.Duration
	EventPrunerInterval      time.Duration
	StatusRecoveryInterval   time.Duration
	StripeEventPrunerInterval time.Duration
	GracePeriodInterval      time.Duration

	// Trash collector toggle
	TrashCollectorEnabled bool

	// WebSocket settings
	WSMaxConnsPerUser    int
	WSTokenCheckInterval time.Duration

	// Event catchup max age
	EventCatchupMaxAge time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", "redis://localhost:6379"),
		EncryptionKey:   os.Getenv("ENCRYPTION_KEY"),
		ResendSystemKey: os.Getenv("RESEND_SYSTEM_API_KEY"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		AppURL:          getEnv("APP_URL", "http://localhost:3000"),
		PublicURL:       getEnv("PUBLIC_URL", "http://localhost:8080"),
		APIPort:         getEnv("API_PORT", "8080"),

		StripeKey:           os.Getenv("STRIPE_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceID:       os.Getenv("STRIPE_PRICE_ID"),

		SystemFromAddress: os.Getenv("SYSTEM_FROM_ADDRESS"),

		EventRetentionDays: GetEnvInt("EVENT_RETENTION_DAYS", 90),

		DomainHeartbeatInterval:   getEnvDuration("DOMAIN_HEARTBEAT_INTERVAL", 6*time.Hour),
		TrashCollectorInterval:    getEnvDuration("TRASH_COLLECTOR_INTERVAL", 1*time.Hour),
		EventPrunerInterval:       getEnvDuration("EVENT_PRUNER_INTERVAL", 6*time.Hour),
		StatusRecoveryInterval:    getEnvDuration("STATUS_RECOVERY_INTERVAL", 5*time.Minute),
		StripeEventPrunerInterval: getEnvDuration("STRIPE_EVENT_PRUNER_INTERVAL", 6*time.Hour),
		GracePeriodInterval:       getEnvDuration("GRACE_PERIOD_INTERVAL", 1*time.Hour),

		TrashCollectorEnabled: os.Getenv("TRASH_COLLECTOR_ENABLED") == "true",

		WSMaxConnsPerUser:    GetEnvInt("WS_MAX_CONNECTIONS_PER_USER", 5),
		WSTokenCheckInterval: getEnvDuration("WS_TOKEN_CHECK_INTERVAL", 1*time.Minute),
		EventCatchupMaxAge:   getEnvDuration("EVENT_CATCHUP_MAX_AGE", 48*time.Hour),
	}

	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET must be at least 32 characters")
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}

	// Validate ENCRYPTION_KEY format: must be valid base64, must decode to 32 bytes
	keyBytes, err := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be valid base64: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(keyBytes))
	}

	// Validate URLs
	for _, pair := range []struct{ name, val string }{
		{"APP_URL", cfg.AppURL},
		{"PUBLIC_URL", cfg.PublicURL},
	} {
		u, err := url.Parse(pair.val)
		if err != nil || !strings.HasPrefix(pair.val, "http") || u.Host == "" {
			return nil, fmt.Errorf("%s must be a valid HTTP/HTTPS URL, got: %s", pair.name, pair.val)
		}
	}

	// Warn if PUBLIC_URL points to localhost (won't work for webhooks in production)
	if strings.Contains(cfg.PublicURL, "localhost") || strings.Contains(cfg.PublicURL, "127.0.0.1") {
		slog.Warn("PUBLIC_URL points to localhost — webhooks will not work in production", "public_url", cfg.PublicURL)
	}

	// Validate API_PORT
	port, err := strconv.Atoi(cfg.APIPort)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("API_PORT must be 1-65535, got: %s", cfg.APIPort)
	}

	// Validate DATABASE_URL prefix
	if !strings.HasPrefix(cfg.DatabaseURL, "postgres://") && !strings.HasPrefix(cfg.DatabaseURL, "postgresql://") {
		return nil, fmt.Errorf("DATABASE_URL must start with postgres:// or postgresql://, got: %s", cfg.DatabaseURL)
	}

	// Validate REDIS_URL prefix
	if !strings.HasPrefix(cfg.RedisURL, "redis://") && !strings.HasPrefix(cfg.RedisURL, "rediss://") {
		return nil, fmt.Errorf("REDIS_URL must start with redis:// or rediss://, got: %s", cfg.RedisURL)
	}

	// PRD-016: Warn if sslmode=disable with a non-local database host
	if strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
		if u, err := url.Parse(cfg.DatabaseURL); err == nil {
			host := u.Hostname()
			if host != "localhost" && host != "127.0.0.1" && host != "postgres" && host != "" {
				slog.Warn("database connection has sslmode=disable with non-local host — credentials may be transmitted in plain text",
					"host", host)
			}
		}
	}

	// PRD-017: Warn if Redis URL has no password with a non-local host
	if u, err := url.Parse(cfg.RedisURL); err == nil {
		host := u.Hostname()
		hasPassword := u.User != nil && func() bool { _, set := u.User.Password(); return set }()
		if !hasPassword && host != "localhost" && host != "127.0.0.1" && host != "redis" && host != "" {
			slog.Warn("Redis connection has no password with non-local host — data may be accessible to anyone on the network",
				"host", host)
		}
	}

	// Stripe cross-validation: if commercial mode is enabled, all Stripe vars are required
	if cfg.StripeKey != "" {
		if cfg.StripeWebhookSecret == "" {
			return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRET is required when STRIPE_KEY is set")
		}
		if cfg.StripePriceID == "" {
			return nil, fmt.Errorf("STRIPE_PRICE_ID is required when STRIPE_KEY is set")
		}
		if cfg.ResendSystemKey == "" {
			return nil, fmt.Errorf("RESEND_SYSTEM_API_KEY is required when STRIPE_KEY is set (commercial mode)")
		}
		if cfg.SystemFromAddress == "" {
			return nil, fmt.Errorf("SYSTEM_FROM_ADDRESS is required when STRIPE_KEY is set (commercial mode)")
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			return d
		}
		slog.Warn("invalid duration for env var, using default", "key", key, "value", val, "default", fallback)
	}
	return fallback
}
