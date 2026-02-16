# Scaling Sync — Background Job Queue Plan

## Current State

- Sync runs inline via SSE (one HTTP connection, one goroutine)
- Dedup by `resend_email_id` makes re-runs safe — no duplicates
- If it breaks, user retries manually; scan re-fetches from page 1, import skips already-imported emails
- Works fine for onboarding, not production-grade

## Target Architecture

```
Frontend                    Any Go Instance              Redis              Worker Pool
┌──────────┐    POST       ┌──────────────┐   LPUSH    ┌───────┐          ┌──────────┐
│ Start    │──────────────▶│ Create job   │───────────▶│ Queue │          │ Worker 1 │
│ Import   │               │ in Postgres  │            │       │◀─BRPOP──│          │
└──────────┘               └──────────────┘            └───────┘          └──────────┘
                                                                          │
┌──────────┐    GET        ┌──────────────┐                               │
│ Poll     │──────────────▶│ Read job     │                               │
│ Status   │◀──────────────│ from Postgres│◀──── updates job rows ────────┘
└──────────┘               └──────────────┘
```

## Database: `sync_jobs` table

```sql
CREATE TABLE sync_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  user_id UUID NOT NULL REFERENCES users(id),
  status TEXT NOT NULL DEFAULT 'queued',  -- queued, scanning, importing, completed, failed

  -- Cursor tracking (resume from where we left off)
  sent_cursor TEXT DEFAULT '',
  received_cursor TEXT DEFAULT '',

  -- Counts
  sent_fetched INT DEFAULT 0,
  received_fetched INT DEFAULT 0,
  imported INT DEFAULT 0,
  total INT DEFAULT 0,

  -- Results
  sent_imported INT DEFAULT 0,
  received_imported INT DEFAULT 0,
  thread_count INT DEFAULT 0,
  address_count INT DEFAULT 0,

  -- Error tracking
  error TEXT,
  retry_count INT DEFAULT 0,
  max_retries INT DEFAULT 3,

  -- Timing
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ,  -- worker updates every 10s, stale = dead worker
  created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_sync_jobs_org ON sync_jobs(org_id);
```

## Redis Queue

- Queue key: `sync:jobs`
- Worker does `BRPOP sync:jobs` (blocking pop, waits for work)
- Payload: `{ "job_id": "uuid", "org_id": "uuid" }`
- Multiple workers can run on multiple Go instances — Redis ensures each job is picked up exactly once

## Worker Logic

```
1. BRPOP job from Redis queue
2. Load job row from Postgres
3. If status = "queued" → set to "scanning", begin scan
   If status = "failed" and retry_count < max_retries → resume from cursors
4. Scan phase:
   - Paginate sent emails from sent_cursor
   - Update sent_cursor + sent_fetched in DB every page
   - Paginate received emails from received_cursor
   - Update received_cursor + received_fetched every page
   - Set total = sent_fetched + received_fetched, status = "importing"
5. Import phase:
   - Import each email (dedup check, insert, thread)
   - Update imported count every 10 emails (batch update, not per-email)
   - Update last_heartbeat_at every 10s
6. On completion: status = "completed", set result counts
7. On error: status = "failed", increment retry_count, re-enqueue if retries remain
```

## Heartbeat + Stale Job Recovery

- Worker updates `last_heartbeat_at` every 10 seconds
- A periodic check (cron or goroutine) finds jobs where:
  - `status IN ('scanning', 'importing')` AND `last_heartbeat_at < now() - interval '60 seconds'`
- These are dead workers — re-enqueue the job (it resumes from cursors)

## API Endpoints

```
POST /api/sync              → Create sync job, enqueue to Redis, return job_id
GET  /api/sync/:id          → Return job status + counts (frontend polls this)
GET  /api/sync/:id/stream   → Optional SSE wrapper that polls DB and streams (nice-to-have)
```

## Frontend

- POST to start sync → gets job_id
- Poll `GET /api/sync/:id` every 2 seconds
- Display scanning/importing/completed based on status field
- Progress bar uses imported/total from the response
- Works from any browser, any Go instance — no sticky connections

## Horizontal Scaling

- Any Go instance can create a job (write to Postgres + Redis)
- Any Go instance running a worker can pick it up (BRPOP is atomic)
- Frontend polls any Go instance (reads from Postgres)
- If worker instance dies → heartbeat goes stale → job gets re-enqueued → another instance picks it up
- Zero sticky sessions, zero shared memory

## Migration Path

1. Add `sync_jobs` table (migration)
2. Add worker goroutine to `main.go` (starts on boot, runs BRPOP loop)
3. Replace `SyncEmailsWithProgress` SSE handler with POST (create job) + GET (poll status)
4. Update `syncEmailsInternal` to read/write cursors from job row
5. Update frontend to poll instead of EventSource
6. Keep SSE endpoint temporarily for backwards compat, remove later

## When to Build This

Pre-launch, after core features are stable. The current SSE approach works for dev/testing. This is the production-grade version.
