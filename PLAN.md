# PLAN.md — Inboxes: Coolify for Gmail for Resend

## Context

Inboxes is an open-source, self-hostable email client powered by Resend. Self-hosters run `docker-compose up` with their own domain and Resend API key. A hosted commercial version adds Stripe billing via a single `STRIPE_KEY` env var — same codebase, same Docker images.

This plan synthesizes `path.md` (open source + commercial strategy), `SCALING_SYNC.md` (background job queue), `notes.md` (13 open bugs/features), and an architectural upgrade to a unified client state model. The goal: Gmail-quality UX for teams who send through Resend.

**Core architectural problem:** The frontend uses React Query as a fetch layer (refetch endpoints) instead of a state store (normalized cache with event-driven updates). Two systems (query cache + websocket) aren't unified, causing stale UI, lost selection, and inconsistent read states. The fix is incremental, not a rewrite.

---

## Phase 1: Stabilize Core ✅ COMPLETED

Fix what's broken before shipping anything.

### ✅ 1.1 CC'd Emails Dropped on Inbound
- **File:** `backend/internal/handler/webhooks.go` (~line 95)
- **Fix:** After iterating `emailData.To` for domain matching, if `domainID` is still empty, iterate `emailData.CC` with the same logic

### ✅ 1.2 Outbound Display Name in From Field
- **Files:** `backend/internal/handler/emails.go` (Send), `backend/internal/handler/drafts.go` (Send)
- **Fix:** Add helper in `backend/internal/handler/helpers.go`: `resolveFromDisplay(ctx, db, orgID, address) string` — checks aliases table name first, then users table name, falls back to bare address. Use in both Send handlers before building Resend payload

### ✅ 1.3 Archive/Spam/Trash Folder 404s
- **Files:** `frontend/app/(app)/d/[domainId]/[folder]/[threadId]/page.tsx`
- **Fix:** Test if dynamic `[folder]` catches "archive", "spam", "trash". If Next.js route conflict, add explicit `archive/[threadId]/page.tsx`, `spam/[threadId]/page.tsx`, `trash/[threadId]/page.tsx` as thin wrappers

### ✅ 1.4 Search Fix
- **File:** `backend/internal/handler/emails.go` (Search handler)
- **Fix:** Change query to JOIN with threads table and return thread-shaped results (`id, subject, participant_emails, unread_count, starred, folder, snippet, last_message_at`). Return as `ThreadListResponse` shape so frontend `ThreadList` can render directly

### ✅ 1.5 Multi-Recipient To/CC/BCC
- **New file:** `frontend/components/recipient-input.tsx`
- **Fix:** Tag/pill input component with autocomplete from `GET /api/contacts/suggest`. Replace plain `<Input>` in `FloatingComposeWindow` and `InlineReplyEditor`

### ✅ 1.6 Reply to Specific Email in Thread
- **File:** `frontend/components/thread-view.tsx`
- **Fix:** Add Reply button per `EmailMessage` (not just thread-level). Track `replyToEmailId` in state. Pass specific email's `message_id` as `In-Reply-To` and `References` headers
- **File:** `backend/internal/handler/emails.go` — accept `in_reply_to` and `references` in send request, pass to Resend as headers, store on email row

### ✅ 1.7 Webhook Signature Verification
- **File:** `backend/internal/handler/webhooks.go` (line 62 TODO)
- **Fix:** Add `svix` Go package. Verify Svix signature using org's `resend_webhook_secret` before processing. Return 401 on failure

### ✅ 1.8 Webhook Idempotency
- **File:** `backend/internal/handler/webhooks.go` (~line 133)
- **Fix:** Before thread matching, check `SELECT id FROM emails WHERE resend_email_id = $1`. Skip if exists

### ✅ 1.9 UTF-8 Snippet Truncation
- **File:** `backend/internal/service/sync.go` (snippet creation)
- **Fix:** Replace `snippet[:200]` with rune-safe truncation to avoid cutting multi-byte characters mid-sequence

---

## Phase 2: Self-Host Release ✅ COMPLETED

Package for open-source launch. Most code exists — this is packaging and docs.

### ✅ 2.1 Complete docker-compose.yml
- **File:** `docker-compose.yml` (rewrite)
- 5 services: `postgres` (16-alpine), `redis` (7-alpine), `backend`, `frontend`, `caddy` (2-alpine)
- Health checks on postgres/redis, depends_on with conditions
- Named volumes for postgres data, caddy data/config
- All config via `.env` file

### ✅ 2.2 Caddyfile
- **New file:** `Caddyfile`
- Route `/api/*` and `/api/ws` to backend:8080, everything else to frontend:3000
- Domain from `{$DOMAIN}` env var, auto-HTTPS via Let's Encrypt

### ✅ 2.3 Runtime Config for Frontend
- **Problem:** Next.js inlines `NEXT_PUBLIC_*` at build time — bad for self-hosters
- **Fix:** Add public `GET /api/config` endpoint returning `{ api_url, ws_url }`. Frontend fetches on boot and caches. No rebuild needed per deployment

### ✅ 2.4 .env.example
- **File:** `.env.example` (rewrite)
- Required: `DOMAIN`, `ENCRYPTION_KEY`, `SESSION_SECRET`
- Defaults: `DATABASE_URL`, `REDIS_URL`, `API_PORT`
- Instructions for generating secrets: `openssl rand -hex 32`

### ✅ 2.5 README.md
- What Inboxes is (1 paragraph)
- Prerequisites: VPS, domain, Resend account
- Quick start: copy .env, set domain + secrets, `docker-compose up -d`
- Local dev note: ngrok for webhooks
- Env var reference table

### ✅ 2.6 MIT LICENSE file

---

## Phase 3: Production Hardening

### 3.1 Background Job Queue
- **New migration:** `008_sync_jobs.sql` — `sync_jobs` table with status, cursors, counts, retry logic, heartbeat
- **New file:** `backend/internal/worker/sync_worker.go` — `BRPOP sync:jobs` loop, loads job, calls sync with cursor resume, updates heartbeat every 10s
- **Modify:** `backend/internal/service/sync.go` — accept initial cursors, update job row per page
- **New endpoints:** `POST /api/sync` (create + enqueue), `GET /api/sync/:id` (poll status)
- **Modify:** `backend/cmd/api/main.go` — start worker goroutine on boot
- **Frontend:** Replace EventSource in onboarding + settings with POST → poll pattern
- **Stale recovery:** Periodic check (60s) re-enqueues jobs with stale heartbeat

### 3.2 Webhook Cleanup Job
- **File:** `backend/internal/handler/cron.go`
- New handler: list webhooks from Resend, delete any matching our URL pattern that aren't the current `resend_webhook_id`
- New route: `POST /api/cron/cleanup-webhooks`

### 3.3 Resume Sync on Revisit
- **File:** `backend/internal/handler/onboarding.go` (Status handler)
- Check `sync_jobs` for in-progress job, return `sync_in_progress: true` so frontend reconnects to progress

### 3.4 Auto-Trigger Sync During Onboarding
- **File:** `frontend/app/(app)/onboarding/page.tsx`
- After domain selection, immediately POST to `/api/sync` instead of showing "Start Import" button

---

## Phase 4: Hosted Commercial

### 4.1 Stripe Billing — Backend
- **New migration:** `009_billing.sql` — add `stripe_customer_id`, `stripe_subscription_id`, `plan`, `plan_expires_at` to orgs
- **New file:** `backend/internal/handler/billing.go`
  - `POST /api/billing/checkout` — create Stripe checkout session
  - `POST /api/billing/portal` — create Stripe customer portal session
  - `POST /api/webhooks/stripe` — handle `checkout.session.completed`, `subscription.deleted`, `invoice.payment_failed`
- **Config:** `STRIPE_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_ID` in config.go (all optional)
- **Middleware:** `RequirePlan` — if `STRIPE_KEY` set, check org plan. No `STRIPE_KEY` = everything free

### 4.2 Stripe Billing — Frontend
- Add billing tab to settings modal (only if backend reports `billing_enabled: true` from `GET /api/orgs/settings`)
- "Upgrade" button → POST checkout → redirect to Stripe
- "Manage" button → POST portal → redirect to Stripe portal

### 4.3 Signup Hardening
- **Rate limiting:** Redis-backed `INCR`+`EXPIRE` middleware for signup (5/hr/IP), login (10/15min/IP), forgot-password (3/hr/email)
- **Email verification (hosted only):** When `STRIPE_KEY` set, signup creates `pending_verification` user, sends 6-digit code via Resend, new `POST /api/auth/verify-email` endpoint

---

## Phase 5: Feature Polish

### 5.1 Inline Thread View (Reading Pane)
- **Files:** All folder pages (`inbox/page.tsx`, `sent/page.tsx`, etc.)
- Split-pane layout: thread list (left, ~400px) + thread detail (right)
- Clicking a thread calls `onSelectThread(id)` instead of `router.push` — preserves selection state
- Keep full-page thread view at `[folder]/[threadId]` for direct links and mobile

### 5.2 Contact Widget Cards
- **New file:** `frontend/components/contact-card.tsx`
- Popover on participant name click: initials avatar, email, name, "copy email" button
- Backend: extend `GET /api/contacts/suggest` or add `GET /api/contacts/:email`

### 5.3 Client State Model Upgrade (Incremental)

**Step 1: Enrich event payloads.**
In `backend/internal/handler/threads.go`, after each mutation, query the updated thread and include it in the event payload. This gives the frontend the new state without refetching.

**Step 2: WS events update cache directly (not invalidate).**
In `frontend/hooks/use-ws-sync.ts`, change from `qc.invalidateQueries(...)` to `qc.setQueriesData(...)` using the thread data from the event payload. Eliminates refetch round-trips.

**Step 3: Sync cursor for reconnection.**
The events table has a bigserial `id`. Store last-seen event ID on the client (in `NotificationContext`). On WebSocket reconnect, fetch missed events from new endpoint `GET /api/events?since={lastEventId}&limit=100`. This is Gmail's `historyId` equivalent — catch up without refetching the world.

### 5.4 Onboarding Slideshow
- Auto-rotating tips during import (per `ONBOARDING_SLIDESHOW.md`)

---

## Execution Dependencies

```
Phase 1 (Stabilize) ✅        Phase 2 (Self-Host) ✅     Phase 3 (Hardening)
├─ ✅ 1.1 CC fix               ├─ ✅ 2.1 docker-compose   ├─ 3.1 Job queue
├─ ✅ 1.2 Display name         ├─ ✅ 2.2 Caddyfile        ├─ 3.2 Webhook cleanup
├─ ✅ 1.3 Folder 404           ├─ ✅ 2.3 Runtime config   ├─ 3.3 Resume sync
├─ ✅ 1.4 Search fix           ├─ ✅ 2.4 .env.example     └─ 3.4 Auto-trigger sync
├─ ✅ 1.5 Multi-recipient      ├─ ✅ 2.5 README
├─ ✅ 1.6 Reply to specific    └─ ✅ 2.6 LICENSE          Phase 4 (Hosted)
├─ ✅ 1.7 Webhook verification                            ├─ 4.1 Stripe BE
├─ ✅ 1.8 Webhook idempotency                             ├─ 4.2 Stripe FE
└─ ✅ 1.9 UTF-8 snippet                                   └─ 4.3 Signup hardening

                                                          Phase 5 (Polish)
                                                          ├─ 5.1 Inline thread view
                                                          ├─ 5.2 Contact cards
                                                          ├─ 5.3 State model upgrade
                                                          └─ 5.4 Onboarding slideshow
```

**Blocks:**
- Phase 2 depends on Phase 1 (don't ship broken software open source)
- Phase 3.1 (job queue) depends on Phase 2 (needs docker-compose for worker)
- Phase 4 depends on Phase 2 (self-host package is baseline)
- Phase 5 items are independent, can start anytime after Phase 1

**Parallelizable:**
- All Phase 1 items are independent of each other
- Phase 2 (packaging) can overlap with late Phase 1 work
- Phase 3.2-3.4 are independent of 3.1
- Phase 4 is entirely independent of Phase 3
- All Phase 5 items are independent of each other

**Exit criteria:**
- Phase 1: All notes.md Open items fixed or consciously deferred. Zero data-loss bugs
- Phase 2: `docker-compose up -d` on fresh VPS with a domain = working email client
- Phase 3: Sync survives worker crash and resumes. Duplicate webhooks ignored
- Phase 4: Hosted version accepts payment. Self-hosters unaffected
- Phase 5: No full-page navigation for reading email. Cache updates via events, not refetch

## Verification

After each phase, verify:
1. Start backend (`cd backend && go run ./cmd/api`) and frontend (`cd frontend && npm run dev`)
2. Sign up, connect Resend, sync emails
3. Send/receive emails, verify threading
4. Test all folder views (inbox, sent, archive, trash, spam)
5. Test bulk actions, star, read/unread
6. Open settings, run sync, verify inbox updates via WebSocket
7. For Phase 2: test `docker-compose up -d` on clean machine
8. For Phase 4: test with and without `STRIPE_KEY`
