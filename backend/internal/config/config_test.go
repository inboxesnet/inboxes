package config

import (
	"strings"
	"testing"
)

// Valid base64-encoded 32 bytes (all zeros)
const testEncryptionKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
const testSessionSecret = "test-secret-that-is-long-enough!"

// setValidEnv sets the minimum valid environment for config.Load() to succeed.
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SESSION_SECRET", testSessionSecret)
	t.Setenv("ENCRYPTION_KEY", testEncryptionKey)
}

func TestLoad_MissingSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing SESSION_SECRET, got nil")
	}
}

func TestLoad_ShortSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "short")
	t.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short SESSION_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "at least 32 characters") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_MissingEncryptionKey(t *testing.T) {
	t.Setenv("SESSION_SECRET", testSessionSecret)
	t.Setenv("ENCRYPTION_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ENCRYPTION_KEY, got nil")
	}
}

func TestLoad_InvalidBase64EncryptionKey(t *testing.T) {
	t.Setenv("SESSION_SECRET", testSessionSecret)
	t.Setenv("ENCRYPTION_KEY", "not-valid-base64!!!")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid base64 ENCRYPTION_KEY, got nil")
	}
	if !strings.Contains(err.Error(), "valid base64") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_WrongLengthEncryptionKey(t *testing.T) {
	t.Setenv("SESSION_SECRET", testSessionSecret)
	// Valid base64 but only 16 bytes
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAA==")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wrong-length ENCRYPTION_KEY, got nil")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_InvalidAPIPort(t *testing.T) {
	setValidEnv(t)
	t.Setenv("API_PORT", "abc")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid API_PORT, got nil")
	}
	if !strings.Contains(err.Error(), "API_PORT") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_PortOutOfRange(t *testing.T) {
	setValidEnv(t)
	t.Setenv("API_PORT", "99999")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for out-of-range API_PORT, got nil")
	}
}

func TestLoad_InvalidAppURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("APP_URL", "not-a-url")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid APP_URL, got nil")
	}
	if !strings.Contains(err.Error(), "APP_URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_InvalidPublicURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PUBLIC_URL", "ftp://invalid")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid PUBLIC_URL, got nil")
	}
	if !strings.Contains(err.Error(), "PUBLIC_URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_InvalidDatabaseURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DATABASE_URL", "mysql://localhost/db")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DATABASE_URL, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_InvalidRedisURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("REDIS_URL", "http://localhost:6379")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid REDIS_URL, got nil")
	}
	if !strings.Contains(err.Error(), "REDIS_URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SessionSecret != testSessionSecret {
		t.Errorf("SessionSecret: got %q, want %q", cfg.SessionSecret, testSessionSecret)
	}
	if cfg.EncryptionKey != testEncryptionKey {
		t.Errorf("EncryptionKey: got %q, want %q", cfg.EncryptionKey, testEncryptionKey)
	}
}

func TestLoad_DefaultDatabaseURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DATABASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL: got %q, want %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DATABASE_URL", "postgres://custom:5432/db")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://custom:5432/db" {
		t.Errorf("DatabaseURL: got %q, want %q", cfg.DatabaseURL, "postgres://custom:5432/db")
	}
}

func TestLoad_WorkerIntervalDefaults(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DomainHeartbeatInterval.Hours() != 6 {
		t.Errorf("DomainHeartbeatInterval: got %v, want 6h", cfg.DomainHeartbeatInterval)
	}
	if cfg.TrashCollectorInterval.Hours() != 1 {
		t.Errorf("TrashCollectorInterval: got %v, want 1h", cfg.TrashCollectorInterval)
	}
	if cfg.StatusRecoveryInterval.Minutes() != 5 {
		t.Errorf("StatusRecoveryInterval: got %v, want 5m", cfg.StatusRecoveryInterval)
	}
}

func TestLoad_CustomWorkerInterval(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DOMAIN_HEARTBEAT_INTERVAL", "2h")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DomainHeartbeatInterval.Hours() != 2 {
		t.Errorf("DomainHeartbeatInterval: got %v, want 2h", cfg.DomainHeartbeatInterval)
	}
}

func TestGetEnv_FallbackUsed(t *testing.T) {
	t.Setenv("NONEXISTENT_VAR_FOR_TEST", "")
	got := getEnv("NONEXISTENT_VAR_FOR_TEST", "fallback")
	if got != "fallback" {
		t.Errorf("getEnv: got %q, want %q", got, "fallback")
	}
}
