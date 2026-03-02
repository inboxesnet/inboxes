# PRD Implementation Order — Architecture-First

The build order that minimizes rework. Each phase establishes patterns or schemas that later phases depend on. Skip ahead and you'll rewrite handlers to add error checking, transactions, or email normalization after the fact.

---

## Phase 1: Core Patterns

Establish these FIRST. Every handler you write after uses them. If you build features without these patterns, you'll refactor every handler later.

| Order | PRD | Title | Why First |
|-------|-----|-------|-----------|
| 1 | **049** | Unchecked Queries | Every `QueryRow` must check errors. Establishes the rule before you touch any handler. |
| 2 | **050** | Fire-and-Forget Writes | Same — every `Exec` must check errors. |
| 3 | **051** | JSON Errors | Same — every `json.Marshal` must check errors. |
| 4 | **053** | Worker Panics | Panic recovery wrapper for all workers. Add once, never think about it. |
| 5 | **044** | Label Transactions | Transaction pattern for all multi-step handlers. Every thread action handler follows this. |
| 6 | **007** | Case-Sensitive Emails | Email normalization + migration. Everything downstream (auth, contacts, aliases) assumes lowercase. |

**Rationale**: If you fix PRD-046 (Star/Unstar) or PRD-048 (Label Rename) before PRD-044, you'll rewrite them to add transactions. If you fix any handler before 049/050/051, you'll revisit for error checking. PRD-007 changes the email column data, so do it before any auth work.

---

## Phase 2: Database Schema & Indexes

Shape the data before building on it. Indexes first (cheap, immediate payoff), query pattern fixes next (they assume indexes exist), schema constraints last (they affect how you write insertion code going forward).

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 7 | **063** | Composite Index | Highest-impact index — covers the core thread list query. |
| 8 | **058** | deleted_at Index | Partial index for soft-delete filtering. Consolidate with 063 into one migration. |
| 9 | **064** | FK Indexes | Missing foreign key indexes. One migration, affects all JOINs. |
| 10 | **062** | JSON Indexes | GIN indexes for JSONB columns. |
| 11 | **057** | N+1 Trash | Fixes core thread list query pattern — do AFTER indexes exist. |
| 12 | **059** | Labels Subquery | Changes label aggregation — batch fetch instead of correlated subquery. |
| 13 | **121** | Cascade Deletes | Establishes soft-delete cascade pattern. All entity deletion handlers follow this. |
| 14 | **074** | Domain Uniqueness | Partial index replacing global unique constraint. |
| 15 | **122** | BYTEA Check Constraint | Simple `CHECK (size <= 10MB)` migration. |
| 16 | **120** | Email Uniqueness | Per-org constraint (documented, may defer actual migration). |

---

## Phase 3: Auth & Session Infrastructure

Middleware that every request hits. Do before touching any auth handlers.

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 17 | **009** | Disabled Users Auth | Adds status check to auth middleware. Every authenticated request. |
| 18 | **019** | Token Blacklist Fail-Closed | Redis failure mode decision. Rate limiting, token revocation, sessions all depend on this. |
| 19 | **026** | Password+Redis Failure | Changes `RevokeAllForUser` return signature. Multiple handlers must update. |
| 20 | **025** | Concurrent Sessions | Session tracking in Redis. Depends on 019 + 026. |
| 21 | **035** | Session Expiry UX | Global 401 → modal instead of hard redirect. Compose window depends on this. |

---

## Phase 4: Email Processing Pipeline

Core email threading logic. 066, 067, 068 all modify the same threading algorithm in `process_fetch.go` — do them together to avoid merge conflicts and rework. 070 and 071 add new fields/logic to the same pipeline. 061 optimizes the query pattern after the logic is settled.

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 22 | **066** | References Header Threading | Adds Step 1.5 to thread matching. |
| 23 | **067** | Empty Subject Merging | Skips Step 2 for empty subjects. |
| 24 | **068** | Cross-Domain Threading | Org-scoped instead of domain-scoped matching. |
| 25 | **070** | Bounce Filtering | Adds bounce detection to inbound pipeline. |
| 26 | **071** | Reply-To Header | Adds `reply_to_addresses` field to Email type. |
| 27 | **061** | Alias Lookup Batching | Batch DB queries in email fetch pipeline. |

---

## Phase 5: Entity Lifecycle

Domain/user/org deletion cascade. 072 creates the pattern. Everything else builds on it.

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 28 | **072** | Domain Deletion Cleanup | Establishes cascading soft-delete for domains. |
| 29 | **073** | Deleted Domains Still Receive | Webhook entry point guard. Depends on 072. |
| 30 | **076** | Disabled Users' Aliases | Alias cleanup on user disable. |
| 31 | **078** | Webhook Cleanup for Deleted Orgs | Unregister Resend webhooks on org deletion. |

---

## Phase 6: Billing / Stripe

State machine first, then features on top. 080 → 081 is the foundation. 085 adds a new state. 084/087/086 are features that assume the state machine is correct.

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 33 | **080** | Stripe Dedup | Idempotency for webhook events. Foundation for all billing. |
| 34 | **081** | Stripe Event Handlers | Complete the state machine (subscription.updated, invoice.paid, etc.). |
| 35 | **085** | Past-Due Status | New `past_due` plan status. Affects middleware + frontend. |
| 36 | **084** | Grace Period | Background worker for grace → cancelled transition. |
| 37 | **087** | Plan Check at Send | Email worker re-checks plan. Depends on 085 for status enum. |
| 38 | **086** | Emails After Deletion | Cancel queued jobs on org deletion. |

---

## Phase 7: Config & Infrastructure

Deploy correctly. 106 → 107 is prerequisite chain. Everything else is independent.

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 39 | **106** | Env Vars Documentation | Prerequisite for 107. |
| 40 | **107** | Config Validation at Startup | Fail-fast on bad config. Depends on 106. |
| 41 | **109** | Configurable Worker Intervals | All 4 workers. Do after 053 (panic recovery). |
| 42 | **108** | Stripe Compose Vars | Docker compose for commercial mode. |
| 43 | **112** | PUBLIC_URL Required | Required config for webhooks. |
| 44 | **114** | DB Password Required | Security-critical config. |
| 45 | **113** | Resource Limits | Docker memory/CPU limits. |

---

## Phase 8: WebSocket Stack

| Order | PRD | Title | Why Here |
|-------|-----|-------|----------|
| 46 | **089** | Connection Limit | Resource management. Foundation for WS stability. |
| 47 | **091** | Alias Filter Fail-Closed | Security: drop events on query error. |
| 48 | **092** | Event Catchup Age Limit | Prevents massive catchup queries. |
| 49 | **090** | Token Expiry + Push Revocation | Reduce security window. Depends on 089. |
| 50 | **093** | Reconnect Jitter | Frontend-only. Depends on 092 (catchup behavior). |

---

## Phase 9: Security Leaf Fixes ✅

Independent, isolated fixes. Can be parallelized — batch into a sprint, any order.

| PRD | Title | Effort |
|-----|-------|--------|
| 001 | Sync Job IDOR | 1-line SQL fix |
| 002 | Content-Disposition Injection | Helper function |
| 003 | Open Redirect | 1-line fix |
| 004 | Docker Root | Dockerfile change |
| 005 | CSP Header | Middleware + config |
| 006 | Billing URL Injection | UUID validation |
| 008 | Verification Brute-Force | Redis rate limit |
| 010 | Webhook Secret Bypass | Error check |
| 011 | LIKE Wildcard Injection | Helper function |
| 012 | BCC Privacy Leak | Column selection |
| 013 | Tracking Pixels | DOMPurify hook |
| 014 | Malicious CSS | CSS allowlist |
| 015 | Admin Rate Limits | Middleware wiring |
| 016 | DB SSL Warning | Config check |
| 017 | Redis Auth Warning | Config check |
| 018 | CSRF Header | Middleware + frontend |
| 020 | Attachment MIME | Content-Type whitelist |
| 021 | Login Lockout | Rate limit middleware |
| 022 | Change-Password Rate Limit | Rate limit middleware |
| 023 | Resend-Verification Rate Limit | Rate limit middleware |
| 024 | Last Admin Protection | Count check |

---

## Phase 10: Frontend UX ✅

Independent, parallelize freely. Compose/Drafts should come after Phase 3 (session expiry).

**Compose / Drafts** (after session expiry is in place):
- 027 Compose Draft Loss
- 028 Duplicate Drafts
- 029 Double-Send
- 030 Auto-Save Invisible

**Search / Navigation**:
- 031 Empty Search Results
- 032 Search Placeholder
- 033 Scroll Position Lost
- 034 Pagination Controls

**Validation Alignment**:
- 038 Subject Length
- 039 Password Rules
- 040 Setup Email Validation
- 041 Claim Name Validation
- 042 Rate Limit Range
- 043 Recipient Limit

**UI Polish**:
- 036 Trash Countdown
- 037 Keyboard Shortcuts Discoverability
- 047 Bulk Selection Error Recovery
- 054 Settings Spinners
- 055 Search Loading State
- 056 Upload Errors
- 082 Paywall Flash
- 083 Double Checkout Prevention
- 088 Cancellation UI

**Accessibility**:
- 094 ARIA Labels on Icon Buttons
- 095 Focus Trap in Compose
- 096 ARIA Combobox on Recipient Input
- 097 Semantic List Markup for Threads
- 098 Skip Navigation Link
- 099 Page Title Updates
- 100 Color Contrast WCAG
- 101 Settings Modal Tabs Accessibility
- 102 aria-expanded on Collapsible Emails
- 103 Toast Live Region
- 104 Keyboard Alternative for Drag-Drop
- 105 Form Error Announcements

---

## Phase 11: Cleanup & Remaining ✅

**Dead Code Removal**:
- ✅ 116 `/api/contacts/lookup` endpoint — removed handler + route
- ✅ 117 `/api/threads/unread-count` endpoint — removed handler + route
- ✅ 118 Attachment Download endpoint — already secured (MIME whitelist, sanitizeFilename, nosniff)
- ✅ 119 `truncate()` utility function — removed from utils.ts + tests

**Pattern Applications** (these apply Phase 1 patterns to specific handlers — do after PRD-044):
- ✅ 045 Duplicate Threads — idempotency check via email_jobs before draft send
- ✅ 046 Star/Unstar Sync — set-value API + deterministic optimistic updates
- ✅ 048 Label Rename — SELECT moved into TX with FOR UPDATE locking
- ✅ 060 Bulk Delete Queries — batch SQL replacing per-thread loop

**Remaining**:
- ✅ 065 Onboarding LIMIT — added LIMIT 500, fixed unchecked Scan
- ✅ 069 RTL Text Support — dir="auto" on email containers + DOMPurify attr
- ✅ 075 DNS Record Change Detection — heartbeat parses SPF/DKIM, publishes degraded events
- ✅ 079 Domain Disconnect Notification — event types + WS toast notifications
- ✅ 110 WebSocket Localhost Fallback — removed hardcoded fallback, empty string guard
- ✅ 111 CI Build Step — go build before go test in workflow
- ✅ 115 Migration Rollback Docs — added to docs/operations.md
- ✅ 124 Select All Across Pages — server-side filter resolution + Gmail-style banner UI
- ✅ 125 Browser Notification Permission — prompt component with localStorage dismiss
- ✅ 126 Draft Attachments Restoration — metadata endpoint + compose window restoration

---

## Dependency Graph (Summary)

```
Phase 1: Error patterns (049-051, 053)
  └─→ Transaction pattern (044)
      └─→ Email normalization (007)

Phase 2: DB indexes + schema (057-064, 121, 122)

Phase 3: Auth middleware (009, 019, 025, 026, 035)

Phase 4: Email pipeline (066-068, 070-071, 061)

Phase 5: Entity lifecycle (072-078)
  └─→ depends on Phase 2 (121 cascade deletes)

Phase 6: Billing state machine (080-087)

Phase 7: Config/infra (106-114)

Phase 8: WebSocket (089-093)

Phase 9+: Security leaves → Frontend UX → Cleanup
  └─→ can run in parallel with Phases 6-8
```

Each layer assumes the previous layer's patterns exist. The critical path is **Phases 1 → 2 → 3 → 4 → 5**. Phases 6, 7, 8 can run in parallel once Phase 3 is done. Phases 9, 10, 11 can start any time after Phase 1.
