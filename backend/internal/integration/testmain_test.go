//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"

	"github.com/inboxes/backend/internal/db"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/store"
)

var (
	testPool   *pgxpool.Pool
	testStore  store.Store
	testRDB    *redis.Client
	testEncSvc *service.EncryptionService
)

// fixed 32-byte key for testing
var testEncKeyBase64 = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))

func TestMain(m *testing.M) {
	ctx := context.Background()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inboxes:inboxes@localhost:5432/inboxes_test?sslmode=disable"
	}
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/1"
	}

	// Connect to test DB
	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: connect to test DB: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	// Run migrations via goose using the embedded SQL files
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintf(os.Stderr, "integration: goose dialect: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: sql.Open: %v\n", err)
		os.Exit(1)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		fmt.Fprintf(os.Stderr, "integration: goose up: %v\n", err)
		os.Exit(1)
	}
	sqlDB.Close()

	// Create store backed by real Postgres pool
	testStore = store.NewPgStore(pool)

	// Connect to Redis (DB 1 to avoid conflicts with dev on DB 0)
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: parse redis URL: %v\n", err)
		os.Exit(1)
	}
	testRDB = redis.NewClient(opts)
	if err := testRDB.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: connect to redis: %v\n", err)
		os.Exit(1)
	}

	// Create encryption service with a fixed test key
	testEncSvc, err = service.NewEncryptionService(testEncKeyBase64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: encryption service: %v\n", err)
		os.Exit(1)
	}

	// Truncate all tables before the test suite runs
	truncateAll(ctx)

	code := m.Run()

	pool.Close()
	testRDB.Close()
	os.Exit(code)
}

// truncateAll deletes rows from every application table in FK-safe order.
func truncateAll(ctx context.Context) {
	tables := []string{
		"events",
		"thread_labels",
		"email_bounces",
		"email_jobs",
		"sync_jobs",
		"drafts",
		"attachments",
		"alias_users",
		"discovered_addresses",
		"emails",
		"threads",
		"aliases",
		"domains",
		"org_labels",
		"stripe_events",
		"user_reassignments",
		"system_settings",
		"users",
		"orgs",
	}
	for _, t := range tables {
		testPool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", t))
	}
}
