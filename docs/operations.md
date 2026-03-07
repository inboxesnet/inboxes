# Operations Guide

Everything an admin needs to know for running and maintaining an Inboxes instance.

## Background Workers

The backend starts 9 background workers (plus 2 stale-recovery companions) alongside the HTTP server. All workers respect graceful shutdown via context cancellation (SIGINT/SIGTERM triggers a 30-second shutdown window).

### Sync Worker

- **Purpose:** Processes email sync jobs from the Redis queue
- **Queue:** `sync:jobs` (Redis list, BRPOP with 5s timeout)
- **Heartbeat:** Updates `heartbeat_at` every 10 seconds while processing
- **Retry:** Failed jobs are re-enqueued up to `max_retries` (default 3)

### Email Worker

- **Purpose:** Processes outbound email send jobs, inbound fetch jobs, and sent-email fetch jobs
- **Queue:** `email:jobs` (Redis list, BRPOP with 5s timeout)
- **Job types:** `send`, `fetch`, `fetch_sent`
- **Heartbeat:** Updates `heartbeat_at` every 10 seconds while processing
- **Retry:** Exponential backoff (2^retry x 5s, capped at 5 minutes), up to `max_retries` (default 5)
- **Rate limit:** Per-org via `OrgLimiterMap` (default 2 RPS to Resend API)
- **Plan enforcement:** Send jobs check subscription status at send time when Stripe is configured
- **Recovery draft:** On permanent send failure, creates a draft from the failed email so the user can retry

### Trash Collector

- **Purpose:** Permanently deletes threads that have been in trash beyond their expiry (`trash_expires_at`)
- **Interval:** `TRASH_COLLECTOR_INTERVAL` (default `1h`)
- **Enable:** Set `TRASH_COLLECTOR_ENABLED=true` (disabled by default)
- **Initial delay:** None (starts on first tick)
- **Behavior:** Soft-deletes threads (`deleted_at = now()`), cleans up `thread_labels`, publishes `thread.deleted` events

### Domain Heartbeat

- **Purpose:** Verifies domain status against Resend API; updates DNS verification fields
- **Interval:** `DOMAIN_HEARTBEAT_INTERVAL` (default `6h`)
- **Initial delay:** 30 seconds after startup
- **Behavior:**
  - Fetches domains from Resend for each org with an API key
  - Marks domains `disconnected` if missing from Resend or API key is revoked (403)
  - Self-heals: reconnects domains that reappear in Resend
  - Tracks SPF/DKIM verification status changes; publishes `domain.dns_degraded` events
  - Transient errors (5xx) are skipped -- no false disconnections

### Event Pruner

- **Purpose:** Removes old events from the `events` table
- **Interval:** `EVENT_PRUNER_INTERVAL` (default `6h`)
- **Initial delay:** 1 minute after startup
- **Retention:** `EVENT_RETENTION_DAYS` (default `90`); set to `0` to disable pruning entirely
- **Batch size:** 5,000 rows per delete, with 100ms pause between batches

### Grace Period Worker

- **Purpose:** Transitions orgs from `cancelled` or `past_due` plan to `free` after their grace period (`plan_expires_at`) has elapsed
- **Interval:** `GRACE_PERIOD_INTERVAL` (default `1h`)
- **Initial delay:** 1 minute after startup
- **Behavior:** Publishes `plan.changed` events for each transitioned org

### Stripe Event Pruner

- **Purpose:** Removes old rows from the `stripe_events` dedup table
- **Interval:** `STRIPE_EVENT_PRUNER_INTERVAL` (default `6h`)
- **Initial delay:** 2 minutes after startup
- **Retention:** 7 days (hardcoded)

### Status Recovery

- **Purpose:** Polls Resend for the true status of outbound emails stuck in `received` state
- **Interval:** `STATUS_RECOVERY_INTERVAL` (default `5m`)
- **Initial delay:** 2 minutes after startup
- **Scope:** Outbound emails with `status = 'received'` older than 10 minutes but newer than 24 hours
- **Batch size:** 50 emails per run
- **Rate-limited:** Uses per-org Resend rate limiter

### Inbox Poller

- **Purpose:** Auto-syncs inbound and sent emails from Resend for orgs that have `auto_poll_enabled = true` -- designed for self-hosted / no-webhook environments
- **Check interval:** 30 seconds (checks which orgs are due based on `auto_poll_interval` and `last_polled_at`)
- **Initial delay:** 1 minute after startup
- **Pagination cap:** 10 pages per poll (100 emails per page)
- **Frontier detection:** Stops paginating when it encounters an email already in the database
- **Rate-limited:** Uses per-org Resend rate limiter
- **Enqueues:** `fetch` jobs (inbound) and `fetch_sent` jobs (outbound) into the `email:jobs` queue

## Stale Job Recovery

Two companion goroutines run alongside the sync and email workers. They detect jobs stuck in `running` state with stale heartbeats (>90 seconds) and either re-enqueue them or mark them permanently failed.

| Companion | Checks every | Stale threshold | Table |
|-----------|-------------|-----------------|-------|
| Sync stale recovery | 60s | 90s | `sync_jobs` |
| Email stale recovery | 60s | 90s | `email_jobs` |

The email stale recovery also recovers **orphaned pending jobs** -- jobs in `pending` state whose `updated_at` is older than 5 minutes (indicating their Redis push failed or was lost).

## Per-Org Rate Limiting

Outbound Resend API calls are rate-limited per organization to prevent any single org from exhausting the shared Resend rate limit.

- Default: 2 requests/second per org (with 15% safety margin)
- Applies to: email sending, sync operations, status recovery polling, inbox polling
- Implementation: Token bucket via `queue.OrgLimiterMap`
- Rate limits are loaded from the `orgs` table (`resend_rps` column) and can be adjusted per org
- Dynamic update: `OrgLimiterMap.UpdateOrgRPS(orgID, rps)` changes the limit at runtime

## Health Checks

### Health Endpoint

```
GET /api/health
```

Returns:
```json
{ "status": "ok" }
```

- `200` -- all dependencies healthy
- `503` -- `{"status": "degraded"}` if Postgres or Redis is unreachable

### Docker HEALTHCHECK

The backend Dockerfile includes a health check:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -sf http://localhost:8080/api/health || exit 1
```

## Schema Management

Migrations are managed by [goose](https://github.com/pressly/goose) and run automatically on server startup. There is 1 consolidated migration file in `backend/internal/db/migrations/`.

### Migration Rollback

If a migration breaks production:

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

**5. Deploy the matching backend version.** The binary must match the schema -- if you rolled back a migration, deploy the binary from before that migration was added. Then restart.

**Important warnings:**
- Migrations with `DROP TABLE` or `DROP COLUMN` in their `-- +goose Down` section will cause **data loss** on rollback.
- Additive migrations (CREATE TABLE, ADD COLUMN) are safe to roll back.
- Always test rollbacks in staging before running in production.

## Key Tables

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
| `email_jobs` | Outbound send queue, inbound fetch queue |
| `sync_jobs` | Email sync job tracking |
| `drafts` | Saved drafts |
| `attachments` | File attachments (BYTEA storage, max 10 MB per file) |
| `events` | Durable event log for WebSocket catch-up |
| `discovered_addresses` | Contact autocomplete data (per-domain address discovery) |
| `org_labels` | Custom label definitions (per-org) |
| `email_bounces` | Bounce tracking (per-org, per-address) |
| `stripe_events` | Stripe webhook dedup table (event_id primary key) |
| `user_reassignments` | Audit log of user-to-user reassignment operations |
| `system_settings` | Instance-wide settings (encrypted values supported) |

## Environment Variables

### Required

| Variable | Notes |
|----------|-------|
| `SESSION_SECRET` | Must be at least 32 characters |
| `ENCRYPTION_KEY` | Base64-encoded 32 bytes (`openssl rand -base64 32`). Used for AES-256-GCM encryption of stored API keys and system settings |

### Infrastructure

| Variable | Default | Notes |
|----------|---------|-------|
| `DATABASE_URL` | `postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable` | Must start with `postgres://` or `postgresql://` |
| `REDIS_URL` | `redis://localhost:6379` | Must start with `redis://` or `rediss://` |
| `API_PORT` | `8080` | HTTP server port (1-65535) |
| `APP_URL` | `http://localhost:3000` | Frontend URL (used for CORS, redirects) |
| `PUBLIC_URL` | `http://localhost:8080` | Backend URL (used for webhooks -- warns if localhost) |

### Stripe (Commercial Mode)

All three are required when `STRIPE_KEY` is set. When `STRIPE_KEY` is empty, billing is disabled (self-hosted mode).

| Variable | Notes |
|----------|-------|
| `STRIPE_KEY` | Stripe secret key. Its presence/absence gates commercial vs. self-hosted mode |
| `STRIPE_WEBHOOK_SECRET` | Required when `STRIPE_KEY` is set |
| `STRIPE_PRICE_ID` | Required when `STRIPE_KEY` is set |
| `RESEND_SYSTEM_API_KEY` | Required when `STRIPE_KEY` is set (used for system emails) |
| `SYSTEM_FROM_ADDRESS` | Required when `STRIPE_KEY` is set (sender address for system emails) |

### Worker Intervals

All accept Go duration strings (e.g., `5m`, `1h`, `30s`). Values must be positive.

| Variable | Default | Notes |
|----------|---------|-------|
| `DOMAIN_HEARTBEAT_INTERVAL` | `6h` | How often to verify domains against Resend |
| `TRASH_COLLECTOR_INTERVAL` | `1h` | How often to purge expired trash |
| `TRASH_COLLECTOR_ENABLED` | *(empty)* | Set to `true` to enable trash collection |
| `EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old events |
| `EVENT_RETENTION_DAYS` | `90` | How long to keep events. Set to `0` to disable pruning |
| `STATUS_RECOVERY_INTERVAL` | `5m` | How often to poll Resend for stale email statuses |
| `STRIPE_EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old Stripe dedup records |
| `GRACE_PERIOD_INTERVAL` | `1h` | How often to check for expired grace periods |

### WebSocket

| Variable | Default | Notes |
|----------|---------|-------|
| `WS_MAX_CONNECTIONS_PER_USER` | `5` | Max concurrent WebSocket connections per user |
| `WS_TOKEN_CHECK_INTERVAL` | `1m` | How often to re-validate WebSocket auth tokens |
| `EVENT_CATCHUP_MAX_AGE` | `48h` | Max age of events served on WebSocket reconnect |

## Backup & Restore

**What to back up:**
1. **PostgreSQL database** -- contains all emails, users, settings, encrypted API keys
2. **`ENCRYPTION_KEY` value** -- without it, stored Resend API keys are unrecoverable

**What NOT to back up:**
- Redis data (rate limit counters, job queues -- all self-healing)
- `.next` cache (frontend build artifacts)
- Container images (rebuilt from source)

### Bare Metal

```bash
# Backup
pg_dump -Fc inboxes > inboxes_$(date +%Y%m%d).dump

# Restore
pg_restore -d inboxes --clean --if-exists inboxes_20260227.dump
```

### Docker

```bash
# Backup
docker exec $(docker ps -qf name=postgres) \
  pg_dump -U inboxes -Fc inboxes > inboxes_$(date +%Y%m%d).dump

# Restore
cat inboxes_20260227.dump | docker exec -i $(docker ps -qf name=postgres) \
  pg_restore -U inboxes -d inboxes --clean --if-exists
```

### Automated Backups

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
- **Orphan recovery** will re-enqueue pending email jobs stuck for >5 minutes
- **WebSocket pub/sub** reconnects automatically

No emails, users, or settings are lost -- all durable data is in PostgreSQL.

## Monitoring / Logging

### Structured Logging

All backend logs use Go's `slog` package with structured key-value pairs:

```
level=INFO msg="server starting" port=8080
level=INFO msg="sync worker: job completed" job_id=abc-123
level=ERROR msg="status recovery: fetch from Resend failed" email_id=xyz error="..."
```

Worker logs are prefixed with the worker name (e.g., `sync worker:`, `trash collector:`, `domain heartbeat:`, `email worker:`, `event pruner:`, `grace period worker:`, `stripe event pruner:`, `status recovery:`, `inbox poller:`).

### Key Log Patterns to Monitor

| Pattern | Meaning |
|---------|---------|
| `domain heartbeat: API key invalid` | Org's Resend API key was revoked |
| `domain heartbeat: DNS verification degraded` | SPF or DKIM records lost verification |
| `email worker: job permanently failed` | Email send exhausted retries |
| `email worker: domain marked disconnected` | Domain-level send failure (e.g., 403) |
| `sync worker: stale job permanently failed` | Sync job stuck and exhausted retries |
| `inbox poller: failed to fetch` | Polling failed for an org (transient or key issue) |
| `grace period worker: transitioned org to free` | Subscription grace period expired |

## Encryption Key Management

Stored Resend API keys (per-org) and system settings are encrypted with AES-256-GCM using the `ENCRYPTION_KEY` environment variable.

- Key format: base64-encoded 32 bytes (`openssl rand -base64 32`)
- Each encrypted value has its own IV and authentication tag
- Stored in the database as separate columns (`value`/`*_encrypted`, `iv`/`*_iv`, `tag`/`*_tag`)

**If you lose the ENCRYPTION_KEY:**
- All stored Resend API keys become unrecoverable
- Users must re-enter their Resend API keys via onboarding or settings
- No email data is lost (emails are stored in plaintext)
- Generate a new key and have each org reconnect their Resend account

## Troubleshooting

### Emails stuck in "received" status

The status recovery worker automatically polls Resend every 5 minutes for outbound emails stuck in `received` state. If emails remain stuck:

1. Check logs for `status recovery: fetch from Resend failed` errors
2. Verify the org's Resend API key is valid
3. Check the `resend_email_id` on the email -- it must be non-null for recovery to work

### Sync jobs stuck in "running"

The sync stale recovery goroutine detects running jobs with heartbeats older than 90 seconds and re-enqueues them automatically. If jobs remain stuck:

1. Check logs for `sync worker: stale recovery` messages
2. Verify Redis is reachable
3. Manually reset: `UPDATE sync_jobs SET status='pending', heartbeat_at=now() WHERE status='running' AND heartbeat_at < now() - interval '5 minutes';`

### Email jobs stuck in "pending"

The email orphan recovery detects pending jobs with `updated_at` older than 5 minutes and re-pushes them to Redis. If jobs remain stuck:

1. Check Redis connectivity
2. Check logs for `email worker: orphan recovery` messages
3. Manually re-enqueue by inserting the job ID into the `email:jobs` Redis list

### Domains marked disconnected unexpectedly

The domain heartbeat worker marks domains `disconnected` when:
- The domain is missing from Resend's API response
- The org's API key returns 403 (revoked)

It will **not** disconnect on transient errors (5xx). Domains self-heal if they reappear in Resend. Check logs for `domain heartbeat: domain disconnected` with the reason.

## Graceful Shutdown

On SIGINT or SIGTERM:

1. HTTP server stops accepting new connections
2. In-flight requests have 30 seconds to complete
3. Context cancellation propagates to all background workers
4. Workers finish their current operation and exit
5. Database and Redis connections are closed
