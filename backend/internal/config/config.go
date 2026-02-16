package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL       string
	RedisURL          string
	EncryptionKey     string
	ResendSystemKey   string
	SessionSecret     string
	AppURL            string
	APIPort           string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", "redis://localhost:6379"),
		EncryptionKey:   os.Getenv("ENCRYPTION_KEY"),
		ResendSystemKey: os.Getenv("RESEND_SYSTEM_API_KEY"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		AppURL:          getEnv("APP_URL", "http://localhost:3000"),
		APIPort:         getEnv("API_PORT", "8080"),
	}

	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
