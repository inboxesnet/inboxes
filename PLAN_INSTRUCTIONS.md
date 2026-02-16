# Instructions: Write PLAN.md

## Goal

Read the entire codebase and produce a single `PLAN.md` — the cohesive plan for shipping Inboxes as "Coolify for Gmail for Resend." Open source, self-hostable, with a hosted commercial version. The plan unifies everything from the existing docs (`path.md`, `SCALING_SYNC.md`, `notes.md`) plus the architectural upgrade to a proper client state model.

## Context from this session

The core architectural insight: the frontend currently treats React Query as a fetch layer (go get this endpoint) instead of a state store (here's what a thread looks like, keep it current). This causes edge cases everywhere — mark-as-read not reflecting, sync not updating the inbox, selection lost on navigation. Gmail solves this with:

1. **Normalized client cache** — threads/emails keyed by ID, views derive from cache
2. **Single event stream** — every mutation (local or remote) comes back as an event, client applies it. WebSocket is the delivery mechanism, not a separate system bolted on
3. **Optimistic mutations** — client applies changes immediately, server confirms or corrects
4. **Sync token / cursor model** — on reconnect, fetch only what changed since last known state (like Gmail's `historyId`), not refetch everything

The building blocks already exist: event bus (Redis pub/sub → WS hub → frontend), React Query cache, `WSSync` hook, optimistic updates in some places. They're just not unified into a single coherent model.

## What to read

### Backend — understand every mutation path and event flow

1. `backend/cmd/api/main.go` — entrypoint, services wired up
2. `backend/internal/config/config.go` — env vars
3. `backend/internal/db/db.go` — connection setup
4. `backend/internal/db/migrations.go` — migration runner
5. `backend/internal/db/migrations/*.sql` — full schema (all migration files)
6. `backend/internal/model/models.go` — Go structs
7. `backend/internal/middleware/auth.go` — JWT claims
8. `backend/internal/middleware/cors.go` — CORS config
9. `backend/internal/router/router.go` — all routes
10. `backend/internal/event/event.go` — event bus, types, Redis publish
11. `backend/internal/ws/hub.go` — WS hub, Redis subscribe, routing
12. `backend/internal/ws/client.go` — WS client, read/write pumps
13. `backend/internal/handler/threads.go` — thread CRUD + mutations (read/star/archive/trash/bulk)
14. `backend/internal/handler/emails.go` — send email, search
15. `backend/internal/handler/webhooks.go` — inbound email from Resend, event publishing
16. `backend/internal/handler/drafts.go` — draft CRUD + send
17. `backend/internal/handler/onboarding.go` — connect Resend, sync, setup
18. `backend/internal/handler/orgs.go` — org settings, sync-stream
19. `backend/internal/handler/auth.go` — signup, login, JWT
20. `backend/internal/handler/domains.go` — domain management
21. `backend/internal/handler/users.go` — user management
22. `backend/internal/handler/aliases.go` — alias management
23. `backend/internal/handler/contacts.go` — contact suggestions
24. `backend/internal/handler/attachments.go` — file upload/download
25. `backend/internal/handler/cron.go` — scheduled jobs
26. `backend/internal/handler/helpers.go` — shared handler utilities
27. `backend/internal/service/sync.go` — email sync from Resend API
28. `backend/internal/service/resend.go` — Resend API wrapper
29. `backend/internal/service/encryption.go` — API key encryption
30. `backend/internal/service/spam.go` — spam detection

### Frontend — understand every state path and UI flow

31. `frontend/lib/types.ts` — all TypeScript types
32. `frontend/lib/api.ts` — HTTP client
33. `frontend/lib/query-client.ts` — React Query config
34. `frontend/lib/query-keys.ts` — cache key structure
35. `frontend/lib/utils.ts` — utilities
36. `frontend/contexts/notification-context.tsx` — WebSocket connection management
37. `frontend/contexts/domain-context.tsx` — domain state, sidebar
38. `frontend/contexts/email-window-context.tsx` — compose window state
39. `frontend/contexts/thread-list-context.tsx` — thread list state
40. `frontend/hooks/use-threads.ts` — React Query hooks for threads
41. `frontend/hooks/use-thread-selection.ts` — selection state
42. `frontend/hooks/use-ws-sync.ts` — WS → cache sync
43. `frontend/app/layout.tsx` — root layout
44. `frontend/app/(app)/layout.tsx` — app shell (what's always mounted)
45. `frontend/app/(app)/d/[domainId]/layout.tsx` — domain layout
46. `frontend/app/(app)/d/[domainId]/inbox/page.tsx` — inbox page
47. `frontend/app/(app)/d/[domainId]/sent/page.tsx` — sent page
48. `frontend/app/(app)/d/[domainId]/archive/page.tsx` — archive page
49. `frontend/app/(app)/d/[domainId]/trash/page.tsx` — trash page
50. `frontend/app/(app)/d/[domainId]/spam/page.tsx` — spam page
51. `frontend/app/(app)/d/[domainId]/drafts/page.tsx` — drafts page
52. `frontend/app/(app)/d/[domainId]/search/page.tsx` — search page
53. `frontend/app/(app)/d/[domainId]/[folder]/[threadId]/page.tsx` — thread detail page
54. `frontend/app/(app)/d/page.tsx` — domain redirect
55. `frontend/app/(app)/onboarding/page.tsx` — onboarding flow
56. `frontend/components/thread-list-page.tsx` — thread list (shared across folders)
57. `frontend/components/thread-list.tsx` — thread row rendering
58. `frontend/components/thread-toolbar.tsx` — toolbar (select, bulk actions, pagination)
59. `frontend/components/thread-view.tsx` — thread detail view
60. `frontend/components/floating-compose-window.tsx` — compose/reply
61. `frontend/components/domain-sidebar.tsx` — sidebar navigation
62. `frontend/components/settings-modal.tsx` — settings
63. `frontend/components/keyboard-shortcuts.tsx` — keyboard shortcuts
64. `frontend/components/notification-listener.tsx` — notification display
65. `frontend/components/tiptap-editor.tsx` — rich text editor
66. `frontend/components/domain-icon.tsx` — domain favicon
67. `frontend/components/drag-preview.tsx` — drag and drop preview

### Infra

68. `docker-compose.yml`
69. `Dockerfile.backend`
70. `Dockerfile.frontend`
71. `.env.example`

### Existing planning docs (re-read to synthesize)

72. `path.md` — open source + hosted plan
73. `SCALING_SYNC.md` — background job queue plan
74. `notes.md` — bugs, ideas, done items
75. `ONBOARDING_SLIDESHOW.md` — onboarding flow

## What to write

A single `PLAN.md` that covers:

### 1. Architecture: Client State Model
- Normalized cache design (what gets cached, key structure)
- Event stream unification (WS as the single source of truth for all mutations)
- Optimistic mutation pattern (every action: apply locally → send to server → server confirms via event → reconcile if different)
- Sync token / cursor for reconnection (fetch delta, not the world)
- Which React Query patterns to keep, which to replace
- Migration path from current fetch-based model to state-machine model

### 2. Architecture: Backend Event Completeness
- Audit every handler mutation — does it publish an event? Which ones are missing?
- Define the complete event catalog (every event type the system needs)
- Ensure mutations return updated state in responses (not just 200 OK)

### 3. Feature Completion (from notes.md)
- Inline thread view (reading pane, not navigate away)
- Reply/compose rewrite (Gmail-style floating composer)
- Search fix
- CC handling on inbound
- Multi-recipient fields
- Display names on outbound
- Contact cards
- Webhook cleanup
- All items from notes.md Open section, prioritized

### 4. Production Hardening (from SCALING_SYNC.md)
- Background job queue for sync
- Heartbeat + stale job recovery
- Resume from cursors on failure

### 5. Self-Host Packaging (from path.md)
- Full docker-compose (postgres, redis, backend, frontend, caddy)
- .env.example with all vars documented
- README quick start
- MIT license

### 6. Hosted / Commercial (from path.md)
- Stripe billing (checkout, portal, webhook, middleware)
- Conditional behavior via STRIPE_KEY
- Signup hardening (rate limiting, email verification)

### 7. Execution Order
- What to build first, what depends on what
- Clear phases with exit criteria
- What's day-1 for open source launch vs. what comes after

## Output format

Write `PLAN.md` at the project root. Make it dense and actionable — not a book, not vague. Each section should be concrete enough that an AI agent could pick it up and start implementing without asking clarifying questions. Reference specific files and functions where relevant.
