# Inboxes — Gap Finder

Third-eye analysis: everything NOT explicitly covered by `PRE-LAUNCH-AUDIT.md` or `UX-STORY-MAP.md`. Generated from 18 parallel codebase exploration agents.

---

## 1. Security Gaps (Not in Audit)

### S1. Email Case Sensitivity — Account Enumeration & Duplicates
**Files:** `backend/internal/handler/auth.go:241,335,477,551`
Login/signup use case-sensitive `WHERE email = $1`. User can register `User@Example.com` and `user@example.com` as separate accounts. Login fails if case doesn't match. `canSendAs()` in helpers.go uses `strings.EqualFold()` — inconsistent.

### S2. Verification Code Brute-Force (6-digit, 15-min window)
**File:** `backend/internal/handler/auth.go:472-480`
Verification code is 6 digits (1M possibilities). No rate limiting on the verify endpoint. Attacker can enumerate all codes within the 15-minute window.

### S3. No Account Lockout After Failed Logins
**File:** `backend/internal/handler/auth.go:217-292`
Unlimited login attempts with no lockout. Rate limiting exists at the router level (10/15min per IP) but no per-account lockout.

### S4. Change Password Endpoint Not Rate-Limited
**File:** `backend/internal/handler/users.go:388-440`
Attacker can brute-force current password by calling this endpoint repeatedly. No rate limit.

### S5. ResendVerification Endpoint Not Rate-Limited
**File:** `backend/internal/handler/auth.go:501-542`
Attacker can flood a user's inbox with 100+ verification emails. No throttle.

### S6. Disabled Users Pass Auth for Up to 7 Days
**File:** `backend/internal/middleware/auth.go:27-62`
Auth middleware only checks JWT validity + token blacklist. Does NOT check `user.status = 'active'` or `org.deleted_at IS NULL`. If Redis is down (fail-open), disabled user's old tokens work until 7-day JWT expiry.

### S7. LIKE Wildcard Injection in Contact Search
**File:** `backend/internal/handler/contacts.go:26-37`
Search query parameterized correctly but LIKE metacharacters (`%`, `_`) not escaped. User input like `%` returns all contacts — data enumeration vector.

### S8. Webhook Secret Lookup Failure = Silent Bypass
**File:** `backend/internal/handler/webhooks.go:78-80`
`QueryRow().Scan()` error for webhook secret is silently ignored. If lookup fails, validation uses empty secret — potential bypass.

### S9. No Last Admin Protection
**File:** `backend/internal/handler/users.go:184-237`
Admin A can disable Admin B. If B was the only other admin, org has one admin. Self-disable is blocked, but no check ensures "at least 1 active admin remains."

### S10. BCC Privacy Leak via participant_emails
**File:** `backend/internal/queue/process_fetch.go:315-325`
BCC addresses are added to `participant_emails` JSON on threads. Frontend can access `participant_emails`, allowing BCC recipients to be inferred by comparing participant lists across emails.

### S11. Images Load Immediately — Tracking Pixels
**File:** `frontend/components/thread-view.tsx:369-380`
DOMPurify allows `src` on `<img>` with no restrictions. Email tracking pixels (`<img src="https://tracker.com/pixel">`) load automatically. No opt-in setting, no "Load Images" button.

### S12. CSS Injection via style Attribute
**File:** `frontend/components/thread-view.tsx:369-380`
DOMPurify config allows `style` attribute with no CSS validation. Malicious CSS can break layout (`position:absolute; width:9999px`), exfiltrate data, or hide content.

### S13. No Concurrent Session Limit
**Files:** `backend/internal/handler/auth.go`, `backend/internal/middleware/auth.go`
No tracking of active session count per user. Compromised password = unlimited simultaneous logins with no detection.

### S14. Password Change + Redis Failure = Silent Failure
**File:** `backend/internal/handler/users.go:388-440`
On password change, token revocation via Redis can fail silently. User is told "other sessions logged out" but they aren't. Extends audit item #11 to a specific dangerous scenario.

---

## 2. Race Conditions & Concurrency (Beyond Audit #18)

### R1. Draft Double-Send
**File:** `backend/internal/handler/drafts.go:275-445`
No idempotency key on draft send. Two simultaneous clicks create two separate email_jobs and send duplicate emails.

### R2. Label Operations Without Transactions
**File:** `backend/internal/handler/threads.go:368-540`
BulkAction, Move, Star, Mute, MarkRead/Unread all perform multiple separate SQL statements without transactional wrapping. Concurrent operations on same threads cause inconsistent label state.

### R3. Thread Creation Check-Then-Insert
**File:** `backend/internal/handler/drafts.go:368-384`
Two concurrent draft sends without threadID create duplicate threads with identical subject/participants.

### R4. Optimistic Update Ordering (Frontend)
**File:** `frontend/hooks/use-threads.ts:44-121`
Rapid star/unstar clicks can receive out-of-order API responses. Final cache state depends on timing, not logic.

### R5. Label Rename Not Atomic
**File:** `backend/internal/handler/labels.go:84-126`
`org_labels` and `thread_labels` updated in separate SQL calls. Concurrent label creation during rename can orphan labels.

### R6. Bulk Selection Cleared Before API Completes
**File:** `frontend/components/thread-list-page.tsx:170`
`selection.clearSelection()` called immediately before bulk API call. If API fails, selection is already gone — user can't retry.

---

## 3. Error Handling Gaps

### E1. 20+ Unchecked QueryRow.Scan() Calls (Backend)
**Files:** `threads.go`, `auth.go`, `setup.go`, `onboarding.go`, `webhooks.go`, `users.go`, `billing.go`
Silent failures when DB queries return errors. Variables used with undefined values. Most critical: webhook secret lookup (webhooks.go:78-80).

### E2. 15+ Unchecked Exec() Calls
**File:** `backend/internal/handler/threads.go:431-899`
Trash expiry updates, label operations, read/unread marking — all fire-and-forget. Failed operations leave data inconsistent.

### E3. 30+ Ignored json.Marshal() Errors
**Files:** `emails.go`, `drafts.go`, `contacts.go`, `webhooks.go`, `process_fetch.go`
Corrupted JSON silently stored in database or sent to APIs.

### E4. Redis Failures Return 200 OK
**Files:** `webhooks.go:175-177`, `emails.go:278-280`, `drafts.go:433-435`
When Redis LPUSH fails after DB insert, job is orphaned. Webhook returns 200 (Resend thinks delivery succeeded). Recovery depends on stale job cron (5+ minute delay).

### E5. No Panic Recovery in Workers
**Files:** All `backend/internal/queue/*.go`, `backend/internal/worker/*.go`
Worker main loops have no panic recovery. Single panic kills the worker permanently. No jobs processed, no alert.

### E6. Draft Auto-Save Failures Silently Swallowed
**File:** `frontend/components/floating-compose-window.tsx:231-234`
If auto-save API fails, `saveStatus` is cleared (no error shown). User closes window thinking draft is saved — data lost.

### E7. Settings Modal — Infinite Spinners on API Failure
**File:** `frontend/components/settings-modal.tsx`
Team, Aliases, Labels, Org, Billing tabs all have `.catch(() => {})` — loading states never resolve on failure. Spinner shows forever.

### E8. Search Failure = Infinite Loading
**File:** `frontend/components/thread-list-page.tsx:63-70`
No `isError` check for search queries. Failed search shows spinner indefinitely with no retry option.

### E9. File Upload Errors Not Displayed
**File:** `frontend/components/floating-compose-window.tsx:156-189`
`uploadFile()` uses raw `fetch()` instead of the `request()` wrapper. Network errors go completely unhandled.

---

## 4. Form Validation Gaps

### V1. Subject Length Mismatch: Frontend 998 vs Backend 500
**Files:** `floating-compose-window.tsx:507` (maxLength=998), `emails.go:84` (validateLength 500)
User enters 600-char subject, frontend allows it, backend rejects with cryptic error.

### V2. Password Complexity Not Validated Client-Side
**Files:** All auth forms (signup, reset, claim, settings)
Frontend only checks `minLength={8}`. Backend requires upper + lower + digit. User enters "simple12" → passes frontend, fails backend.

### V3. Setup Form Doesn't Validate Email
**File:** `backend/internal/handler/setup.go`
Unlike signup/login which use `validateEmail()`, setup uses basic required check only.

### V4. Claim Endpoint Doesn't Validate Name
**File:** `backend/internal/handler/auth.go:427-437`
Signup validates name length (255), but claim does not.

### V5. RPS Range Not Shown in Frontend
**File:** `frontend/components/settings-modal.tsx`
Backend enforces 1-100 for Resend RPS, but frontend has no range indicator. User enters "abc" → silent failure.

### V6. Recipient Count Unlimited
**Files:** `frontend/components/recipient-input.tsx`, `backend/internal/handler/emails.go`
No max recipient count on either side. User can add hundreds of recipients.

---

## 5. Database Performance Issues (Beyond Audit #25-26)

### D1. Correlated Subquery: labelsSubquery Runs 50x Per Page
**File:** `backend/internal/handler/threads.go:22`
`(SELECT array_agg(label) FROM thread_labels WHERE thread_id = t.id)` runs for every row. Missing index on `thread_labels(thread_id)`.

### D2. Bulk Delete = 300+ Queries for 100 Threads
**File:** `backend/internal/handler/threads.go:510-517`
Each thread: `hasLabel()` + `removeAllLabels()` + `UPDATE threads` = 3+ queries per thread in a loop.

### D3. N+1: getUserAliasAddresses() Called Per Request
**File:** `backend/internal/handler/threads.go:25-42`
Called from List, Get, BulkAction, UnreadCount — same query repeated per endpoint call.

### D4. N+1: Alias Discovery Per Recipient
**File:** `backend/internal/queue/process_fetch.go:348-360`
One `SELECT FROM aliases` per recipient address. Email with 10 recipients = 10 queries. Should batch: `WHERE address = ANY($2::text[])`.

### D5. Missing GIN Indexes on JSONB Columns
**File:** `backend/internal/handler/onboarding.go:356-393`
`to_addresses @> to_jsonb(...)` and `cc_addresses @> to_jsonb(...)` without GIN indexes. Full table scan.

### D6. Missing Index: threads(org_id, deleted_at, last_message_at)
Every thread list query filters by org_id + deleted_at IS NULL + ORDER BY last_message_at DESC. No composite index.

### D7. Missing Foreign Key Indexes
Tables `email_jobs`, `alias_users`, `discovered_addresses` — FK columns `user_id`, `domain_id` have no indexes, causing sequential scans on joins.

### D8. Unbounded Query: GetAddresses() in Onboarding
**File:** `backend/internal/handler/onboarding.go:356-393`
No LIMIT clause. Org with millions of discovered addresses fetches everything.

---

## 6. Email Threading & Rendering Gaps

### T1. References Header Parsed but Never Used for Threading
**File:** `backend/internal/queue/process_fetch.go:218-226`
`References` header stored in DB but not used for thread matching. This is the most reliable RFC 5322 threading approach.

### T2. Empty Subject Threads All Merge Together
**File:** `backend/internal/queue/process_fetch.go:270`
`WHERE subject = ''` matches ALL no-subject emails. Unrelated emails get merged into same thread.

### T3. Cross-Domain Emails Split Into Separate Threads
**File:** `backend/internal/queue/process_fetch.go:270`
`WHERE domain_id = $2` partitions threads by domain. Reply from domain B to thread on domain A creates a new thread.

### T4. No RTL Text Detection
**File:** `frontend/components/thread-view.tsx`
No `dir="auto"` on email content. Hebrew, Arabic, Persian emails display incorrectly as LTR.

### T5. Bounce Notifications Create User-Visible Threads
**File:** `backend/internal/handler/webhooks.go:128-179`
Bounce emails from `mailer-daemon@` processed as normal inbound. Creates threads that shouldn't be user-visible.

### T6. Reply-To Header Not Used for Threading or Reply
**File:** `backend/internal/queue/process_fetch.go:158-161`
`reply_to` stored but ignored. Frontend `getReplyFromAddress` doesn't check Reply-To header — replies go to From instead.

---

## 7. Domain & User Lifecycle Gaps

### L1. Domain Deletion Doesn't Clean Up Aliases/Threads
**File:** `backend/internal/handler/domains.go:434-453`
Soft-delete sets `status='deleted'` but aliases, threads, emails, discovered_addresses remain orphaned.

### L2. Deleted Domains Still Receive Emails
**File:** `backend/internal/handler/webhooks.go:128-179`
Webhook handler doesn't check domain status before creating email_jobs. Emails delivered to deleted domains still processed.

### L3. Domain UNIQUE Constraint is Global
**File:** `backend/internal/db/migrations/001_initial.sql:51`
`domain TEXT NOT NULL UNIQUE` — not per-org. Soft-deleted domain blocks other orgs from adding it.

### L4. DNS Record Changes Never Detected
**File:** `backend/internal/worker/domain_heartbeat.go`
Heartbeat only compares domain names. Never re-fetches DNS verification status (mx_verified, spf_verified, dkim_verified).

### L5. Disabled Users' Aliases Still Receive Emails
**File:** `backend/internal/handler/webhooks.go:147-155`
No check that email recipient's alias belongs to an active user. If all admins disabled, email processing fails silently.

### L6. No Hard Delete for Users or Orgs (GDPR)
**Files:** `backend/internal/handler/users.go`, `backend/internal/handler/orgs.go`
Only soft-delete/disable. Data persists forever. Email addresses locked by UNIQUE constraints — can't re-invite.

### L7. Webhook Processing for Deleted Orgs
**File:** `backend/internal/handler/webhooks.go:59-102`
No check for `org.deleted_at`. Deleted orgs' webhooks silently fail. Should return 410 Gone and unregister webhook.

### L8. No Domain Disconnection Notifications
**File:** `backend/internal/worker/domain_heartbeat.go:75-101`
Disconnected domains only logged. No WebSocket event, no email alert to admins.

---

## 8. Billing & Stripe Edge Cases (Beyond Audit #32)

### B1. No Stripe Event Deduplication
**File:** `backend/internal/handler/billing.go:213-362`
No tracking of processed event IDs. Webhook retries process same event multiple times.

### B2. Checkout-to-Webhook Race Window
**File:** `backend/internal/handler/billing.go:32-119`
User returns from Stripe before webhook arrives. `/api/billing` returns `plan: "free"` for 3-5 seconds. Paywall briefly blocks.

### B3. Multiple Simultaneous Checkout Sessions
**File:** `frontend/components/payment-wall.tsx:26-37`
No request deduplication. Two rapid clicks create two Stripe checkout sessions. Second overrides first.

### B4. Grace Period Never Enforced by Background Job
**File:** `backend/internal/middleware/auth.go:176`
After grace period expires, plan stays "cancelled" with expired `plan_expires_at`. No periodic job to set plan to "free."

### B5. Past_Due Treated as Immediate Cancellation
**File:** `backend/internal/handler/billing.go:289-290`
`SubscriptionStatusPastDue` sets plan to "cancelled." But Stripe still retries payment — if it succeeds later, subscription.updated webhook restores plan. Gap: users see "cancelled" during Stripe's retry window.

### B6. Emails Still Sent After Org Deletion
**File:** `backend/internal/handler/orgs.go:119-185`
Org deletion doesn't drain Redis queue. Already-queued emails process and send after org is deleted.

### B7. No Plan Check at Send Time
**File:** `backend/internal/handler/emails.go:68-290`
`RequirePlan()` runs at queue time. Email worker doesn't re-check plan when actually sending. If subscription lapses between queue and send, email still goes out.

### B8. No Cancellation Confirmation UI
**File:** `frontend/components/settings-modal.tsx`
Org deletion immediately cancels Stripe subscription. No "cancel at period end" option, no confirmation dialog showing what will happen.

---

## 9. WebSocket & Real-Time Gaps (Beyond Audit #23)

### W1. No Per-User/Org Connection Limits
**File:** `backend/internal/ws/hub.go:16`
`clients` map is unbounded. Compromised user can open 1000+ connections, exhausting memory and DB pool.

### W2. Token Expiry Grace Period: 5 Minutes
**File:** `backend/internal/ws/client.go:187-202`
Token validity checked every 5 minutes. Revoked user keeps receiving events for up to 5 minutes after logout.

### W3. Alias Filtering Fails Open on DB Error
**File:** `backend/internal/ws/hub.go:107-113`
If `getThreadAliasLabels()` fails (DB error), returns nil → event broadcasts to ALL org members instead of filtered set.

### W4. Frontend `connected` State Never Consumed
**File:** `frontend/contexts/notification-context.tsx`
`connected` state is exported but no component ever reads it. The "Reconnecting..." banner in sidebar uses a separate mechanism.

### W5. Event Catchup Has No Age Limit
**File:** `backend/internal/handler/events.go:34-40`
`GET /api/events?since=X` has no time-based cutoff. Client offline for 30 days replays events from entire period. No HTTP 410 for "too old."

### W6. Reconnect Thundering Herd
**File:** `frontend/contexts/notification-context.tsx:103`
Jitter is only `Math.random() * 1000` (1 second). 100 users reconnecting simultaneously create server spike.

---

## 10. Accessibility Gaps (Beyond Audit #24)

### A1. No Focus Trap in Compose Window
**File:** `frontend/components/floating-compose-window.tsx`
Modal-style window with no `role="dialog"`, no `aria-modal`, no focus trap. Tab key escapes to elements behind.

### A2. No ARIA Combobox Pattern on Recipient Input
**File:** `frontend/components/recipient-input.tsx`
No `role="combobox"`, `aria-autocomplete`, `aria-expanded`, `role="listbox"` on suggestions.

### A3. Thread List Has No Semantic List Roles
**File:** `frontend/components/thread-list.tsx`
Container is plain `<div>`. No `role="list"`, no `role="listitem"`. Screen readers can't navigate by item count.

### A4. No Skip Navigation Link
No "Skip to main content" link. Keyboard users must tab through entire sidebar.

### A5. No Document Title Updates on Navigation
Navigating between inbox/sent/drafts doesn't update `<title>`. Screen readers can't announce current page.

### A6. Color Contrast Failures
144 instances of `text-muted-foreground` across components. Variants with `/60` and `/70` opacity likely fail WCAG AA 4.5:1 ratio.

### A7. Settings Modal Missing Tab ARIA Pattern
**File:** `frontend/components/settings-modal.tsx`
No `role="tablist"`, `role="tab"`, `aria-selected`, `role="tabpanel"`, `aria-controls`.

### A8. No aria-expanded on Collapsible Email Messages
**File:** `frontend/components/thread-view.tsx`
Collapsed/expanded emails have no `aria-expanded` or `aria-controls`.

### A9. Toast Notifications Not in Live Region
Sonner toaster has no `role="region"` or `aria-live="polite"`. Screen readers don't announce toasts.

### A10. Drag-and-Drop Has No Keyboard Alternative
**File:** `frontend/components/drag-preview.tsx`
No accessible keyboard fallback for moving threads between folders.

### A11. Form Errors Not Announced
All auth forms: error messages lack `role="alert"`, `aria-live`, `aria-invalid`, `aria-describedby`.

---

## 11. Configuration & Environment Gaps

### C1. Missing from .env.example
`TRASH_COLLECTOR_ENABLED`, `BACKEND_URL`, `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_WS_URL` — all used in code but not documented.

### C2. No Config Validation
`EncryptionKey` (should be base64, 32 bytes), `SessionSecret` (no length check), `AppURL`/`PublicURL` (no URL format check), `APIPort` (no port range check).

### C3. Stripe Vars Missing from docker-compose.yml
`STRIPE_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_ID`, `SYSTEM_FROM_ADDRESS` not passed to backend service. Commercial mode can't work in Docker.

### C4. Worker Intervals All Hardcoded
Domain Heartbeat (6h), Trash Collector (1h), Event Pruner (6h), Status Recovery (5m), HTTP timeouts (15s read/write) — none configurable via env vars.

### C5. Frontend WebSocket Fallback
**File:** `frontend/contexts/notification-context.tsx:23`
Falls back to `ws://localhost:8080` if derivation fails. Would leak connection attempts in production.

---

## 12. Dead Code & Orphaned Endpoints

### O1. `/api/contacts/lookup` — Never Called
**File:** `backend/internal/handler/contacts.go:62`
Route registered, handler implemented, but no frontend caller. Only `/api/contacts/suggest` is used.

### O2. `/api/threads/unread-count` — Superseded
**File:** `backend/internal/handler/threads.go:928`
Route registered but frontend uses `/api/domains/unread-counts` instead.

### O3. `/api/attachments/{id}` Download — Likely Dead
**File:** `backend/internal/handler/attachments.go:83`
No frontend code calls the download endpoint. Only upload is used.

### O4. `truncate()` Utility — Never Used
**File:** `frontend/lib/utils.ts:85-88`
Defined but never imported anywhere.

---

## 13. Missing Features (Not Promised, But Expected)

| Feature | Status | Notes |
|---|---|---|
| Email Signature | Not implemented | No per-alias or global signature (planned — see REACT-EMAIL-FEATURE.md) |
| Select All Across Pages | Not implemented | Current page only (100 threads) |
| Undo for Non-Archive/Trash | Not implemented | Only archive/trash have undo toasts |
| Browser Notification Permission | Partial | Backend supports it; frontend never requests permission |
| Mailto: Link Interception | Not implemented | In-app mailto links not intercepted |
| Draft Attachment Restoration | Partial | IDs saved but not displayed on reopen |
| Reply-to Bounced Warning | Not implemented | Reply button enabled on bounced emails |

---

## 14. Test Coverage Gaps

### Backend Coverage: ~35-40%
**Untested handlers (13 of 19):**
- `auth.go` — Login, signup, password reset (CRITICAL)
- `billing.go` — Stripe integration (CRITICAL)
- `domains.go` — Domain management (CRITICAL)
- `onboarding.go` — User setup flow (CRITICAL)
- `orgs.go` — Org management (CRITICAL)
- `aliases.go`, `attachments.go`, `contacts.go`, `cron.go`, `labels.go`, `setup.go`, `sync.go`, `users.go`

**Untested workers (5 of 6):** domain_heartbeat, event_pruner, sync_worker, trash_collector

**Untested queue processors:** process_fetch.go (CRITICAL), process_send.go (CRITICAL)

**Untested middleware (4 of 5):** CORS, logging, rate limiting, security headers

**Untested services:** token_blacklist.go

### Frontend Coverage: ~15-20%
**Untested components (16 of 18):** including floating-compose-window, thread-view, thread-list, settings-modal, domain-sidebar, payment-wall, tiptap-editor

**Untested contexts (5 of 5):** domain-context, notification-context, email-window-context, thread-list-context, app-config-context

**Untested hooks (2 of 5):** use-sync-job, use-ws-sync

---

## 15. Loading & Empty State Gaps

### LS1. Zero Domains = Blank Sidebar
**File:** `frontend/components/domain-sidebar.tsx`
No empty state message. First-time users see blank domain area with no guidance.

### LS2. Settings Modal Tabs Hang on API Failure
**File:** `frontend/components/settings-modal.tsx`
Team, Aliases, Labels, Org, Billing — all show infinite spinners on error. `catch(() => {})` never resets loading state.

### LS3. No Skeleton Screens Anywhere
All lists (threads, aliases, labels, team) show full spinner or nothing. No skeleton placeholder rows.

### LS4. Domain Context Loading = Empty Arrays
**File:** `frontend/contexts/domain-context.tsx`
While initializing, children receive `domains: []`. Components render blank before data arrives.

### LS5. Unread Count Query Failure = Badges Disappear
**File:** `frontend/contexts/domain-context.tsx:36-39`
If unread counts API fails, returns `{}`. Badges silently vanish. No error indicator.

### LS6. Compose Window — Aliases Empty State
**File:** `frontend/components/floating-compose-window.tsx:69`
If alias list is empty or load fails, "From" dropdown is blank. No guidance to create an alias.

---

## Priority Summary

| Priority | Count | Category |
|---|---|---|
| P0 (Before Launch) | 14 | S1-S2, S6-S8, R1-R2, V1-V2, T2, L2, B1, E4 |
| P1 (Soon After) | 18 | S3-S5, S9-S12, R3-R4, E1-E3, D1-D2, T1, T3, L1 |
| P2 (Near Term) | 20 | V3-V6, E5-E9, D3-D8, T4-T6, L3-L8, B2-B8 |
| P3 (Ongoing) | 15 | W1-W6, A1-A11, C1-C5, LS1-LS6 |
| P4 (Backlog) | 15 | O1-O4, test coverage, missing features, dead code |

---

*Generated by 18 parallel codebase exploration agents. Cross-referenced against PRE-LAUNCH-AUDIT.md and UX-STORY-MAP.md to exclude already-documented findings.*
