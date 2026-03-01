# Operations Guide

Everything an admin needs to know for running and maintaining an Inboxes instance.

## Background Workers

The backend starts 6 background workers alongside the HTTP server. All workers respect graceful shutdown via context cancellation (SIGINT/SIGTERM triggers a 30-second shutdown window).

### Sync Worker

- **Purpose:** Processes email sync jobs from the Redis queue
- **Queue:** `sync:jobs` (Redis list, BRPOP with 5s timeout)
- **Heartbeat:** Updates `heartbeat_at` every 10 seconds while processing
- **Retry:** Failed jobs are re-enqueued up to `max_retries` (default 3)
- **Stale recovery:** A companion goroutine checks for running jobs with stale heartbeats (>90s) every 60 seconds and re-enqueues them

### Email Worker

- **Purpose:** Processes outbound email send jobs
- **Queue:** Redis-backed job queue with per-org rate limiting
- **Rate limit:** Configurable per-org (default 2 requests/second to Resend API)
- **Stale recovery:** Same pattern as sync worker — recovers stuck jobs

### Trash Collector

- **Purpose:** Permanently deletes threads that have been in trash beyond their expiry
- **Schedule:** Every 1 hour (when enabled)
- **Enable:** Set `TRASH_COLLECTOR_ENABLED=true`
- **Behavior:** Soft-deletes threads (`deleted_at = now()`), cleans up labels, publishes `thread.deleted` events

### Domain Heartbeat

- **Purpose:** Verifies domain status against Resend API
- **Schedule:** Every 6 hours (first run 30 seconds after startup)
- **Behavior:**
  - Fetches domains from Resend for each org
  - Marks domains `disconnected` if missing from Resend or API key is revoked (403)
  - Self-heals: reconnects domains that reappear in Resend
  - Transient errors (5xx) are skipped — no false disconnections

### Event Pruner

- **Purpose:** Removes old events from the `events` table
- **Schedule:** Every 6 hours (first run 1 minute after startup)
- **Retention:** `EVENT_RETENTION_DAYS` env var (default: 90)
- **Batch size:** 5,000 rows per delete, with 100ms pause between batches
- **Disable:** Set `EVENT_RETENTION_DAYS=0`

### Status Recovery

- **Purpose:** Polls Resend for the true status of outbound emails stuck in "received" state
- **Schedule:** Every 5 minutes (first run 2 minutes after startup)
- **Scope:** Outbound emails with `status = 'received'` older than 10 minutes but newer than 24 hours
- **Batch size:** 50 emails per run
- **Rate-limited:** Uses per-org Resend rate limiter

## Health Monitoring

### Health Endpoint

```
GET /api/health
```

Returns:
```json
{ "status": "ok", "db": true, "redis": true }
```

- `200` — all dependencies healthy
- `503` — one or more dependencies down (`"status": "degraded"`)

### Docker HEALTHCHECK

The backend Dockerfile includes a health check:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/health || exit 1
```

### Structured Logging

All backend logs use Go's `slog` package with structured key-value pairs:

```
level=INFO msg="server starting" port=8080
level=INFO msg="sync worker: job completed" job_id=abc-123
level=ERROR msg="status recovery: fetch from Resend failed" email_id=xyz error="..."
```

Worker logs are prefixed with the worker name (e.g., `sync worker:`, `trash collector:`, `domain heartbeat:`).

## Database

### Schema Management

Migrations are managed by [goose](https://github.com/pressly/goose) and run automatically on server startup. There are 31 migration files in `backend/internal/db/migrations/`.

Key tables:

| Table | Purpose |
|-------|---------|
| `orgs` | Organizations (multi-tenant root) |
| `users` | User accounts (belong to an org) |
| `domains` | Email domains (belong to an org) |
| `aliases` | Email aliases (belong to a domain) |
| `alias_users` | Many-to-many: which users can send from which aliases |
| `threads` | Email threads (belong to a domain) |
| `thread_labels` | Labels/folders on threads (inbox, sent, archive, trash, spam, starred, muted, custom, alias:*) |
| `emails` | Individual email messages (belong to a thread) |
| `email_jobs` | Outbound email send queue |
| `drafts` | Saved drafts |
| `attachments` | File attachments (BYTEA storage) |
| `events` | Durable event log for WebSocket catch-up |
| `sync_jobs` | Email sync job tracking |
| `labels` | Custom label definitions |
| `contacts` / `discovered_addresses` | Contact autocomplete data |
| `system_settings` | Instance-wide settings (encrypted values supported) |

### Migration Rollback

Migrations auto-run on startup. If a migration breaks production, follow this procedure:

**1. Stop the backend** to prevent auto-migration on restart:
```bash
# Docker
docker compose stop backend

# Bare metal
kill $(lsof -ti :8080) 2>/dev/null
```

**2. Back up the database** before rolling back:
```bash
# Docker
docker exec $(docker ps -qf name=postgres) \
  pg_dump -U inboxes -Fc inboxes > pre_rollback_$(date +%Y%m%d_%H%M%S).dump

# Bare metal
pg_dump -Fc inboxes > pre_rollback_$(date +%Y%m%d_%H%M%S).dump
```

**3. Check current migration version:**
```bash
psql -U inboxes -d inboxes -c "SELECT * FROM goose_db_version ORDER BY id DESC LIMIT 5;"
```

**4. Roll back one migration:**
```bash
cd backend && goose -dir internal/db/migrations postgres "$DATABASE_URL" down
```

**5. Deploy the matching backend version.** The binary must match the schema — if you rolled back migration N, deploy the binary from before migration N was added. Then restart.

**Important warnings:**
- Migrations with `DROP TABLE` or `DROP COLUMN` in their `-- +goose Down` section will cause **data loss** on rollback.
- Additive migrations (CREATE TABLE, ADD COLUMN) are safe to roll back.
- Rows created after the migration may reference columns/tables that no longer exist after rollback.
- Always test rollbacks in staging before running in production.

### Backup & Restore

**What to back up:**
1. **PostgreSQL database** — contains all emails, users, settings, encrypted API keys
2. **`ENCRYPTION_KEY` value** — without it, stored Resend API keys are unrecoverable

**What NOT to back up:**
- Redis data (rate limit counters, job queues — all self-healing)
- `.next` cache (frontend build artifacts)
- Container images (rebuilt from source)

#### Bare Metal

```bash
# Backup
pg_dump -Fc inboxes > inboxes_$(date +%Y%m%d).dump

# Restore
pg_restore -d inboxes --clean --if-exists inboxes_20260227.dump
```

#### Docker

```bash
# Backup
docker exec $(docker ps -qf name=postgres) \
  pg_dump -U inboxes -Fc inboxes > inboxes_$(date +%Y%m%d).dump

# Restore
cat inboxes_20260227.dump | docker exec -i $(docker ps -qf name=postgres) \
  pg_restore -U inboxes -d inboxes --clean --if-exists
```

#### Automated Backups

Set up a cron job:

```bash
# Daily backup at 3 AM, keep 7 days
0 3 * * * pg_dump -Fc inboxes > /backups/inboxes_$(date +\%Y\%m\%d).dump && find /backups -name "inboxes_*.dump" -mtime +7 -delete
```

### Redis Data Loss

If Redis data is lost (restart, crash), the impact is minimal:

- **Rate limit counters** reset (briefly allows extra requests)
- **Queued sync/email jobs** may need to be re-triggered by the user
- **Stale recovery workers** will detect and re-enqueue stuck jobs within 60-90 seconds
- **WebSocket pub/sub** reconnects automatically

No emails, users, or settings are lost — all durable data is in PostgreSQL.

## Encryption Key Management

Stored Resend API keys (per-org) and system settings are encrypted with AES-256-GCM using the `ENCRYPTION_KEY` environment variable.

- Key format: base64-encoded 32 bytes (`openssl rand -base64 32`)
- Each encrypted value has its own IV and authentication tag
- Stored in the database as separate columns (`value`, `iv`, `tag`)

**If you lose the ENCRYPTION_KEY:**
- All stored Resend API keys become unrecoverable
- Users must re-enter their Resend API keys via onboarding or settings
- No email data is lost (emails are stored in plaintext)
- Generate a new key and have each org reconnect their Resend account

## Environment Variables (Workers)

| Variable | Default | Notes |
|----------|---------|-------|
| `EVENT_RETENTION_DAYS` | `90` | How long to keep events in the `events` table. Set to `0` to disable pruning |
| `TRASH_COLLECTOR_ENABLED` | *(empty)* | Set to `true` to enable automatic permanent deletion of expired trash |

## Per-Org Rate Limiting

Outbound Resend API calls are rate-limited per organization to prevent any single org from exhausting the shared Resend rate limit.

- Default: 2 requests/second per org
- Applies to: email sending, sync operations, status recovery polling
- Implementation: Token bucket via `queue.OrgLimiterMap`
- Rate limits are loaded from the `orgs` table (`resend_rate_limit` column) and can be adjusted per org

## Graceful Shutdown

On SIGINT or SIGTERM:

1. HTTP server stops accepting new connections
2. In-flight requests have 30 seconds to complete
3. Context cancellation propagates to all background workers
4. Workers finish their current operation and exit
5. Database and Redis connections are closed
