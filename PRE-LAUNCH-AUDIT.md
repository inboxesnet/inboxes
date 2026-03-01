# Pre-Launch Audit: Inboxes

## Context

Full end-to-end audit before launch. Covers backend security, frontend UX/security, infrastructure, integrations, and data model. North stars: **UX** and **Security**. Findings deduplicated across 4 parallel audits (backend, frontend, infra, data model).

---

## P0 — Security (fix before launch)

### 1. Sync Job IDOR
**File:** `backend/internal/handler/sync.go` — `GetSync` method
**Problem:** `GET /api/sync/{id}` fetches sync job by ID without verifying `org_id` matches the caller's org. Any authenticated user can query any org's sync status.
**Fix:** Add `AND org_id = $2` with `claims.OrgID` to the query.

### 2. Content-Disposition header injection
**File:** `backend/internal/handler/attachments.go:100`
**Problem:** Filename is interpolated directly into `Content-Disposition` header without sanitization. A filename containing `"` could break header parsing or enable response splitting.
**Fix:** Sanitize filename — strip/escape quotes and non-ASCII, or use RFC 5987 `filename*=UTF-8''...` encoding.

### 3. Open redirect in PaymentWall
**File:** `frontend/components/payment-wall.tsx:30-32`
**Problem:** `parsed.hostname.endsWith("stripe.com")` allows `stripe.com.evil.com`.
**Fix:** Use strict equality: `parsed.hostname === "checkout.stripe.com"`.

### 4. Docker containers run as root
**Files:** `backend/Dockerfile`, `frontend/Dockerfile`
**Problem:** Neither Dockerfile sets a non-root user. Container escape → host-level root.
**Fix:** Add `RUN addgroup -S app && adduser -S app -G app` then `USER app` before `CMD`.

### 5. Add Content-Security-Policy header
**File:** `backend/internal/middleware/security.go`
**Problem:** No CSP header. Email HTML content could execute scripts if DOMPurify is bypassed in a future regression.
**Fix:** Add `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src * data: blob:; connect-src 'self' wss:; frame-ancestors 'none'`.

### 6. Billing checkout URL injection
**File:** `backend/internal/handler/billing.go:149,154`
**Problem:** `firstDomainID` (from DB, but ultimately from Resend API response) is concatenated into success/cancel URLs without validation. Malicious domain ID could inject path segments.
**Fix:** Validate `firstDomainID` is a valid UUID before interpolation.

---

## P1 — Security (fix soon after launch)

### 7. Rate-limit admin/cron endpoints
**File:** `backend/internal/router/router.go`
**Problem:** `/api/cron/purge-trash`, `/api/cron/cleanup-webhooks`, `/api/admin/jobs` have no rate limiting. A compromised admin token could trigger unlimited purge cycles.
**Fix:** Add rate limiter (e.g., 5 req/min) to admin group.

### 8. sslmode=disable in production compose
**File:** `docker-compose.yml`, `.env.example`
**Problem:** Default `DATABASE_URL` uses `sslmode=disable`. If compose is used with a remote DB, passwords are sent in cleartext.
**Fix:** Document production setting `sslmode=require` prominently. Add comment in `.env.example`.

### 9. Redis AUTH not configured
**File:** `.env.example`, docs
**Problem:** Default `REDIS_URL=redis://localhost:6379` has no password. Network-accessible Redis is an open door.
**Fix:** Document `redis://:password@host:6379` for production. Add note in deployment docs.

### 10. Missing CSRF token layer
**Problem:** Auth relies solely on SameSite=Lax cookies. Lax allows top-level GET navigations from cross-origin — safe for state-changing POSTs, but a GET endpoint that has side effects would be vulnerable.
**Current mitigation:** All state-changing endpoints are POST/PATCH/DELETE (not GET). SameSite=Lax is sufficient *today*.
**Fix:** Audit all GET endpoints to confirm none have side effects. Add comment documenting this decision. Consider adding `X-Requested-With` header check as defense-in-depth.

### 11. Token blacklist fails open
**File:** `backend/internal/service/token_blacklist.go`
**Problem:** If Redis is down, revoked tokens are accepted. A logout + Redis failure = session still valid.
**Fix:** Document the trade-off. Optionally: if Redis is down, reject tokens (fail closed) or require re-auth. At minimum log a warning.

### 12. Attachment MIME type bypass
**File:** `backend/internal/handler/attachments.go:40-62`
**Problem:** Blocks executables but trusts client `Content-Type` for `text/*`. User could upload malicious HTML/SVG as `text/html`, served to other users.
**Fix:** Force `Content-Type: application/octet-stream` for downloads (already using `Content-Disposition: attachment`, but belt-and-suspenders). Or whitelist only known-safe MIME types for inline serving.

---

## P2 — UX (fix before launch)

### 13. Compose window silently loses draft on save failure
**File:** `frontend/components/floating-compose-window.tsx:264-275`
**Problem:** If auto-save fails when user closes compose, `catch` block swallows the error and closes anyway. User's draft is lost.
**Fix:** Show error toast and keep the window open on save failure. Only close on success or explicit discard.

### 14. No "No results" message in search
**File:** `frontend/components/thread-list-page.tsx:186-220`
**Problem:** Search returning 0 results shows an empty thread list with no message. User can't tell if search is loading, found nothing, or failed.
**Fix:** Add "No results found for '{query}'" empty state when search returns empty.

### 15. Session expiry hard-redirects to /login without warning
**File:** `frontend/lib/api.ts:30-31`
**Problem:** On 401, `window.location.href = "/login"` immediately. If user was composing an email, all unsaved work is lost.
**Fix:** Fire a custom event first (like `session-expired`). Show a modal: "Your session expired. Please log in again." Give user a moment to copy their work. Then redirect on modal dismiss.

### 16. Trash countdown is static
**File:** `frontend/components/thread-view.tsx:298-302`
**Problem:** "This conversation will be permanently deleted in 3 days" is computed once on render. Never updates.
**Fix:** Use `useEffect` + `setInterval` (every hour) to recompute. Or just accept it as "good enough" since users rarely stare at the banner.

### 17. No keyboard shortcut discoverability
**File:** `frontend/components/keyboard-shortcuts.tsx`
**Problem:** Shortcuts exist but no visible hint in the UI that they exist. Users must accidentally press `?`.
**Fix:** Add small "?" icon in sidebar footer that opens the shortcuts dialog. Or show a one-time tooltip on first visit.

### 18. Draft race condition — duplicate creation
**File:** `frontend/components/floating-compose-window.tsx:196-248`
**Problem:** Multiple rapid saves can fire before `setDraftId(res.id)` is set, creating duplicate drafts.
**Fix:** Add a `savingRef` mutex. If a save is in-flight, skip or queue the next one.

---

## P3 — UX (fix soon after launch)

### 19. Thread list scroll position lost on back navigation
**Problem:** Viewing a thread then going back resets the thread list scroll position.
**Fix:** Store scroll position in a ref or context. Restore on back navigation.

### 20. No pagination controls visible
**File:** `frontend/components/thread-list-page.tsx`
**Problem:** No page numbers, no "load more" button, no "page X of Y" indicator.
**Fix:** Add either infinite scroll with "load more" or explicit page controls.

### 21. Attachment upload has no progress indicator
**File:** `frontend/components/floating-compose-window.tsx:156-189`
**Problem:** Large file uploads show a spinner but no progress percentage.
**Fix:** Use `XMLHttpRequest` or `fetch` with `ReadableStream` for progress tracking.

### 22. Search placeholder doesn't hint at capability
**File:** `frontend/components/thread-list-page.tsx:199`
**Problem:** Input just says "Search..." — user doesn't know if it searches subject, body, sender, etc.
**Fix:** Change to "Search emails by subject, sender, or content..."

### 23. No offline/disconnected indicator
**File:** `frontend/contexts/notification-context.tsx`
**Problem:** WebSocket disconnection has 3-second delay before showing status. User might take actions that silently fail.
**Fix:** Show a persistent banner: "Reconnecting..." after 3s. Queue or block actions while disconnected.

### 24. Missing ARIA labels on action buttons
**Problem:** Star, archive, trash icon buttons lack `aria-label`. Screen readers announce them as just "button".
**Fix:** Add `aria-label="Star thread"`, `aria-label="Archive"`, etc. to all icon-only buttons.

---

## P4 — Data Model (fix before launch if easy, else soon after)

### 25. N+1 query: trash_expires_at in thread list
**File:** `backend/internal/handler/threads.go:205-207`
**Problem:** Inside `for rows.Next()` loop, runs a separate query per thread to get `trash_expires_at`.
**Fix:** Include `t.trash_expires_at` in the initial list query's SELECT clause.

### 26. Missing index on threads(deleted_at)
**Problem:** Every thread list query filters `t.deleted_at IS NULL`. Without an index, this scans the full table.
**Fix:** `CREATE INDEX idx_threads_deleted_at ON threads(deleted_at) WHERE deleted_at IS NOT NULL;`

### 27. users(email) globally unique — blocks multi-org
**File:** `backend/internal/db/migrations/001_initial.sql`
**Problem:** `email TEXT NOT NULL UNIQUE` is globally unique, not per-org. Same email can't exist in two orgs.
**Impact:** Pre-launch this is fine (single-org). But it's a schema debt that requires a migration to fix later.
**Fix (later):** Change to `UNIQUE(org_id, email)`. Defer to post-launch unless multi-org is a launch feature.

### 28. No FK CASCADE on org/user/domain deletes
**Problem:** Deleting an org (soft or hard) doesn't cascade to threads, emails, aliases, etc. Orphaned rows if someone runs manual SQL.
**Current mitigation:** App uses soft-delete and handler-level cleanup.
**Fix:** Add `ON DELETE CASCADE` or `ON DELETE SET NULL` to all FK relationships. Or document that direct SQL deletes are unsupported.

### 29. BYTEA attachments won't scale
**File:** `backend/internal/db/migrations/028_attachments.sql`
**Problem:** Attachment binary data stored inline as BYTEA. Every SELECT on attachments table loads full binaries into memory. DB backup size balloons.
**Fix (later):** Move to S3/object storage. For now: document the limitation. Add a CHECK constraint for max size: `CHECK (size <= 10485760)`.

---

## P5 — Infrastructure (fix before launch)

### 30. CI doesn't build Go binary
**File:** `.github/workflows/test.yml`
**Problem:** CI runs `go test` but never `go build`. Linker errors won't be caught until Docker build.
**Fix:** Add `go build -o /dev/null ./cmd/api` step to CI.

### 31. Missing PUBLIC_URL in docker-compose
**File:** `docker-compose.yml`
**Problem:** `PUBLIC_URL` is required for webhook registration but not in compose file. Easy to forget.
**Fix:** Add `PUBLIC_URL=${PUBLIC_URL}` with a comment explaining it's required.

### 32. Stripe event handlers incomplete
**File:** `backend/internal/handler/billing.go`
**Problem:** Docs list 5 Stripe events to subscribe to. Code only handles 2: `checkout.session.completed` and `customer.subscription.deleted`. Missing: `customer.subscription.updated`, `invoice.payment_succeeded`, `invoice.payment_failed`.
**Fix:** Add handlers for the 3 missing events. At minimum: `invoice.payment_failed` → set `plan_status = 'past_due'` and notify user.

### 33. No container resource limits in compose
**File:** `docker-compose.yml`
**Problem:** No memory/CPU limits. Runaway container can consume all host resources.
**Fix:** Add `deploy.resources.limits` (memory: 512M, cpus: '0.5' for each service).

### 34. Weak default DB password
**File:** `.env.example`, `docker-compose.yml`
**Problem:** Default password is `inboxes`. Copy-pasted to production.
**Fix:** Remove default or add bold "CHANGE IN PRODUCTION" comment. Use `${POSTGRES_PASSWORD:?Set POSTGRES_PASSWORD}` in compose to require explicit setting.

### 35. No migration rollback docs
**Problem:** If a migration breaks production, there's no documented rollback procedure.
**Fix:** Add "Rollback" section to `docs/operations.md`: `goose -dir migrations down`, with warnings about data loss.

---

## P6 — Nice-to-have / Post-launch

### 36. Add dependency scanning to CI (Trivy/Snyk)
### 37. Move to S3 for attachments
### 38. Add audit columns (created_by, updated_by) to critical tables
### 39. Unread badge pulse animation on WebSocket update
### 40. Breadcrumb navigation for mobile
### 41. Skeleton loading states for thread view
### 42. Focus trap in compose modal
### 43. Event payload schema validation (prevent accidental sensitive data in events)
### 44. Document encryption key backup procedure in ops guide
### 45. Add `appendonly yes` to Redis production config docs
### 46. Improve password policy (optional special character, HIBP check)

---

## Verification approach

After implementing P0-P2:

1. **Backend build + tests:** `cd backend && go build ./cmd/api && go test ./... -race`
2. **Frontend tests:** `cd frontend && npm test`
3. **Security spot-checks:**
   - Try `GET /api/sync/{other-org-job-id}` → should 404 now (IDOR fix)
   - Upload attachment with `filename="test\".html"` → should be sanitized
   - Check response headers include CSP
   - Verify Docker images run as non-root: `docker exec <container> whoami`
4. **UX spot-checks:**
   - Open compose, type content, disconnect network, close → should warn about unsaved
   - Search for gibberish → should show "no results" message
   - Session expire mid-compose → should show modal, not hard redirect
5. **Infra:**
   - `docker compose up` without `PUBLIC_URL` → should fail with clear error
   - CI passes with go build step added
