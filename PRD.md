# PRD: Inboxes.net — Company Email Platform

## Introduction

Inboxes.net is a simple, affordable company email solution built on Resend's infrastructure. It provides organizations with full email functionality (send, receive, threading, aliases, catch-all) without per-seat pricing or the feature bloat of Google Workspace/Microsoft 365.

**Target users:** Small to mid-size orgs who only need email, teams already using Calendly/Slack/Zoom for other functions, cost-conscious orgs, and portfolio entrepreneurs managing multiple domains.

## Goals

- Full send/receive email via custom domains using Resend
- Gmail-style conversation threading
- Team management with invite-based onboarding
- Aliases/distribution lists for shared inboxes
- Catch-all routing so no email is lost
- Real-time updates via WebSockets
- Basic full-text search across user's emails
- Mobile-first, minimal web UI
- RBAC with Admin and Member roles

## Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | Next.js 14+ (App Router), TypeScript, Tailwind CSS, shadcn/ui |
| Backend | Next.js API Routes |
| Database | PostgreSQL via Prisma ORM |
| Auth | Custom email/password (bcrypt + session cookies) |
| Email | Resend (send + receive via webhook) |
| Real-time | WebSockets (ws or socket.io) |
| File Storage | S3-compatible (attachments) |
| Hosting | Vercel |

---

## User Stories

---

### US-001: Project scaffolding

**Description:** As a developer, I need a properly configured Next.js project with all dependencies so I can begin building features.

**Acceptance Criteria:**
- [x] Next.js 14+ with App Router, TypeScript, Tailwind CSS initialized
- [x] shadcn/ui installed and configured with default theme
- [x] Prisma installed with PostgreSQL provider configured
- [x] Project structure: `app/`, `components/`, `lib/`, `prisma/`, `types/`
- [x] Environment variables template (`.env.example`) with placeholders for DATABASE_URL, RESEND_API_KEY, SESSION_SECRET
- [x] ESLint + Prettier configured
- [x] Typecheck passes

---

### US-002: Core database schema — Org, User, Domain

**Description:** As a developer, I need the foundational database models so that orgs, users, and domains can be stored.

**Acceptance Criteria:**
- [x] Org model: id (uuid), name, catch_all_enabled (bool, default true), created_at, updated_at
- [x] User model: id (uuid), org_id (fk), email (unique), name, password_hash, role (enum: admin/member), status (enum: invited/active/disabled), invite_token, invite_expires_at, claimed_at, created_at, updated_at
- [x] Domain model: id (uuid), org_id (fk), domain (unique), status (enum: pending/verified/active), mx_verified (bool), spf_verified (bool), dkim_verified (bool), verified_at, created_at, updated_at
- [x] Migration runs successfully
- [x] Typecheck passes

---

### US-003: Email and Thread database schema

**Description:** As a developer, I need the Email and Thread models to store all email data.

**Acceptance Criteria:**
- [x] Thread model: id (uuid), org_id (fk), user_id (fk), subject, participant_emails (json), last_message_at, message_count (int), unread_count (int), starred (bool, default false), folder (enum: inbox/sent/archive/trash), deleted_at (nullable), created_at, updated_at
- [x] Email model: id (uuid), org_id (fk), thread_id (fk), user_id (fk), message_id (string), in_reply_to (nullable), references_header (json, nullable), from_address, to_addresses (json), cc_addresses (json), bcc_addresses (json), subject, body_html (text), body_plain (text), attachments (json), direction (enum: inbound/outbound), status (enum: received/sent/delivered/bounced/failed), read (bool), starred (bool), folder (enum: inbox/sent/archive/trash), deleted_at (nullable), trash_expires_at (nullable), delivered_via_alias (nullable fk), original_to (nullable), received_at, created_at
- [x] Proper indexes on: user_id, thread_id, org_id, message_id, folder, received_at
- [x] Migration runs successfully
- [x] Typecheck passes

---

### US-004: Alias database schema

**Description:** As a developer, I need the Alias and AliasUser models to support distribution lists.

**Acceptance Criteria:**
- [x] Alias model: id (uuid), org_id (fk), address (string, unique), name (string), created_at, updated_at
- [x] AliasUser model: id (uuid), alias_id (fk), user_id (fk), can_send_as (bool, default true), created_at
- [x] Unique constraint on (alias_id, user_id)
- [x] Migration runs successfully
- [x] Typecheck passes

---

### US-005: Auth — signup endpoint (create org + admin)

**Description:** As a new user, I can create an organization and become its admin so I can start setting up email.

**Acceptance Criteria:**
- [x] `POST /api/auth/signup` accepts: org_name, user_name, email, password
- [x] Password hashed with bcrypt (cost factor 12)
- [x] Creates Org record, then User record with role=admin, status=active
- [x] Returns session cookie (httpOnly, secure, sameSite=lax)
- [x] Validates: email format, password 8+ chars, org_name not empty
- [x] Returns 409 if email already exists
- [x] Typecheck passes

---

### US-006: Auth — login endpoint

**Description:** As a registered user, I can log in with email and password to access my inbox.

**Acceptance Criteria:**
- [x] `POST /api/auth/login` accepts: email, password
- [x] Verifies password against stored hash with bcrypt
- [x] Returns session cookie on success
- [x] Returns 401 with generic message on invalid credentials
- [x] Only allows login for users with status=active
- [x] Typecheck passes

---

### US-007: Auth — session middleware and logout

**Description:** As a developer, I need middleware to protect routes and a logout endpoint to end sessions.

**Acceptance Criteria:**
- [x] Session stored as signed JWT in httpOnly cookie (contains user_id, org_id, role)
- [x] Middleware reads/validates session on all `/api/*` and app routes (except /auth/*)
- [x] `getCurrentUser()` helper returns typed user object or null
- [x] `POST /api/auth/logout` clears session cookie
- [x] Unauthenticated requests to protected routes return 401
- [x] Typecheck passes

---

### US-008: Auth — password reset flow

**Description:** As a user who forgot their password, I can request a reset link and set a new password.

**Acceptance Criteria:**
- [x] `POST /api/auth/forgot-password` accepts email, generates reset token (stored on User), sends email via Resend
- [x] Reset token expires after 1 hour
- [x] `POST /api/auth/reset-password` accepts token + new_password, updates password_hash, clears token
- [x] Returns 400 for expired/invalid tokens
- [x] Typecheck passes

---

### US-009: Auth UI — signup page

**Description:** As a new user, I see a signup form to create my organization.

**Acceptance Criteria:**
- [x] Page at `/signup` with form: org name, your name, email, password, confirm password
- [x] Client-side validation (matching passwords, email format, 8+ char password)
- [x] Submits to signup API, redirects to `/dashboard` on success
- [x] Shows API error messages inline
- [x] Link to login page
- [x] Typecheck passes
- [x] Verify changes work in browser

---

### US-010: Auth UI — login page

**Description:** As a returning user, I see a login form to access my inbox.

**Acceptance Criteria:**
- [x] Page at `/login` with form: email, password
- [x] Submits to login API, redirects to `/inbox` on success
- [x] Shows error message on invalid credentials
- [x] Link to signup and forgot-password pages
- [x] Typecheck passes
- [x] Verify changes work in browser

---

### US-011: Auth UI — password reset pages

**Description:** As a user, I can request a password reset and set a new password via the UI.

**Acceptance Criteria:**
- [x] Page at `/forgot-password` with email input, submits to forgot-password API
- [x] Success message: "Check your email for reset instructions"
- [x] Page at `/reset-password?token=xxx` with new password + confirm fields
- [x] Submits to reset-password API, redirects to login on success
- [x] Shows error for invalid/expired token
- [x] Typecheck passes
- [x] Verify changes work in browser

---

### US-012: App layout shell

**Description:** As a user, I see a consistent app layout with sidebar navigation.

**Acceptance Criteria:**
- [x] Layout at `/app` (or root authenticated layout) with sidebar + main content area
- [x] Sidebar shows: Inbox, Sent, Search, Settings nav items
- [x] Admin users also see: Team, Domain, Aliases in sidebar
- [x] Mobile: sidebar collapses to hamburger menu (Sheet component)
- [x] Shows current user name/email in sidebar footer with logout button
- [x] Active nav item highlighted
- [x] Typecheck passes
- [x] Verify changes work in browser

---

### US-013: Domain — add domain API

**Description:** As an admin, I can add my domain and receive the required DNS records to configure.

**Acceptance Criteria:**
- [x] `POST /api/domains` accepts: domain (string). Admin-only.
- [x] Validates domain format (no protocol, no path, valid TLD)
- [x] Creates Domain record with status=pending
- [x] Calls Resend API to register domain and get required DNS records (MX, SPF, DKIM)
- [x] Returns DNS records that need to be configured (type, name, value, priority)
- [x] Returns 409 if domain already registered
- [x] Typecheck passes

---

### US-014: Domain — verification API

**Description:** As an admin, I can trigger domain verification to check if DNS records are properly configured.

**Acceptance Criteria:**
- [x] `POST /api/domains/:id/verify` triggers Resend domain verification
- [x] Checks MX, SPF, DKIM status via Resend API
- [x] Updates Domain record: mx_verified, spf_verified, dkim_verified flags
- [x] When all verified: status → verified → active, set verified_at
- [x] Returns current verification status for each record type
- [x] Typecheck passes

---

### US-015: Domain — setup UI (admin)

**Description:** As an admin, I see a domain setup page with DNS records to configure and a verify button.

**Acceptance Criteria:**
- [x] Page at `/settings/domain` (admin-only)
- [x] If no domain: form to enter domain name
- [x] After adding: displays DNS records table with Type, Name, Value, Priority columns
- [x] Each record has a "Copy" button for the value
- [x] "Verify Domain" button that triggers verification API
- [x] Status badges: pending (yellow), verified (green), failed (red) per record
- [x] Typecheck passes
- [x] Verify changes work in browser

---

### US-016: Send email — API endpoint

**Description:** As a user, I can send an email via the API which calls Resend to deliver it.

**Acceptance Criteria:**
- [x] `POST /api/emails/send` accepts: to, cc (optional), bcc (optional), subject, body_html, body_plain, in_reply_to (optional), references (optional)
- [x] Sends via Resend `POST /emails` with user's email as `from`
- [x] Creates Email record with direction=outbound, status=sent
- [x] Creates or updates Thread record (new thread if not a reply, existing thread if in_reply_to matches)
- [x] Sets proper Message-ID header
- [x] Returns 400 if org domain not active
- [x] Typecheck passes

---

### US-017: Send email — delivery status webhook

**Description:** As a developer, I need to track email delivery status via Resend webhooks.

**Acceptance Criteria:**
- [x] Webhook handler at `POST /api/webhooks/resend` handles: email.sent, email.delivered, email.bounced, email.delivery_delayed
- [x] Verifies Resend webhook signature (HMAC)
- [x] Updates Email record status based on event type
- [x] Ignores duplicate webhook events gracefully
- [x] Returns 200 immediately (processing is fast)
- [x] Typecheck passes

---

### US-018: Receive email — inbound webhook handler

**Description:** As a developer, I need to receive inbound emails from Resend and store them in the correct user's inbox.

**Acceptance Criteria:**
- [x] Webhook handler at `POST /api/webhooks/resend` handles: email.received event
- [x] Verifies webhook signature
- [x] Parses: from, to, cc, subject, body_html, body_plain, message_id, in_reply_to, references, attachments metadata
- [x] Routes to correct user by matching `to` address against User.email
- [x] Creates Email record with direction=inbound, status=received, read=false
- [x] Stores attachment metadata (id, filename, content_type, size) as JSON
- [x] Typecheck passes

---

### US-019: Receive email — threading logic

**Description:** As a developer, inbound emails are grouped into threads based on email headers.

**Acceptance Criteria:**
- [x] On inbound email: check `In-Reply-To` and `References` headers to find existing thread
- [x] If match found: add email to existing thread, update thread's last_message_at, message_count, unread_count
- [x] If no match: create new Thread with subject (stripped of Re:/Fwd:), set participant_emails
- [x] Update thread's participant_emails with any new addresses
- [x] Thread matching works across inbound and outbound emails
- [x] Typecheck passes

---

### US-020: Receive email — alias routing

**Description:** As a developer, emails sent to an alias address are delivered to all assigned users.

**Acceptance Criteria:**
- [ ] On inbound: if `to` matches an Alias address, find all AliasUser records
- [ ] Create separate Email record for each alias user (each gets their own copy)
- [ ] Set `delivered_via_alias` to the alias ID on each Email
- [ ] Each user gets their own Thread (or existing thread updated)
- [ ] No duplicate delivery if user is directly addressed AND on the alias
- [ ] Typecheck passes

---

### US-021: Receive email — catch-all routing

**Description:** As a developer, emails to non-existent addresses are delivered to org admins when catch-all is enabled.

**Acceptance Criteria:**
- [ ] On inbound: if `to` matches no User and no Alias, check org's catch_all_enabled
- [ ] If enabled: deliver to all org admins, set `original_to` field on Email
- [ ] If disabled: return appropriate response (Resend will bounce)
- [ ] Catch-all emails clearly marked with the original intended address
- [ ] Typecheck passes

---

### US-022: WebSocket server setup

**Description:** As a developer, I need a WebSocket server so clients can receive real-time email notifications.

**Acceptance Criteria:**
- [ ] WebSocket server integrated with Next.js (via custom server or separate process)
- [ ] Clients authenticate via session token on connection
- [ ] Server maintains map of connected user_ids to socket connections
- [ ] `notifyUser(userId, event, payload)` helper function available for use by other code
- [ ] Handles connection/disconnection gracefully
- [ ] Typecheck passes

---

### US-023: Real-time email push to client

**Description:** As a user, when a new email arrives, my inbox updates in real-time without refreshing.

**Acceptance Criteria:**
- [ ] After inbound email is stored, call `notifyUser()` with event `new_email` and thread/email summary
- [ ] Client-side hook `useWebSocket()` that connects on mount, reconnects on disconnect
- [ ] Hook exposes `onMessage` callback for components to subscribe to events
- [ ] Inbox list re-fetches or optimistically updates when `new_email` event received
- [ ] Typecheck passes

---

### US-024: Inbox list view UI

**Description:** As a user, I see my inbox as a list of threads sorted by most recent activity.

**Acceptance Criteria:**
- [ ] Page at `/inbox` showing threads where folder=inbox
- [ ] Each thread row shows: sender avatar/initial, sender name, subject, body preview (truncated), time, unread indicator (bold + dot)
- [ ] Threads sorted by last_message_at descending
- [ ] Unread threads visually distinct (bold text, blue dot)
- [ ] Clicking a thread navigates to thread view
- [ ] Pagination or infinite scroll (20 threads per page)
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-025: Thread view UI

**Description:** As a user, I can view a full email conversation with all messages in chronological order.

**Acceptance Criteria:**
- [ ] Page at `/inbox/[threadId]` showing all emails in the thread
- [ ] Each message shows: sender, recipients, timestamp, body (rendered HTML, sanitized with DOMPurify)
- [ ] Messages in chronological order (oldest first)
- [ ] Most recent message expanded by default, older messages collapsed (click to expand)
- [ ] Thread marked as read when opened (update unread_count)
- [ ] Back button returns to inbox list
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-026: Compose email — modal UI

**Description:** As a user, I can compose and send a new email via a compose modal.

**Acceptance Criteria:**
- [ ] "Compose" button in sidebar/header opens modal (Dialog component)
- [ ] Fields: To (email input), Subject, Body (rich text: bold, italic, links, lists)
- [ ] CC/BCC fields hidden by default, toggled by button
- [ ] Send button submits to send email API
- [ ] Modal closes on successful send with toast confirmation
- [ ] Shows error inline on failure
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-027: Reply and forward in thread

**Description:** As a user, I can reply to or forward an email within a thread view.

**Acceptance Criteria:**
- [ ] "Reply" button on each message opens inline reply form at bottom of thread
- [ ] Reply pre-fills: To (original sender), Subject (Re: subject), In-Reply-To and References headers
- [ ] "Reply All" includes all participants in To/CC
- [ ] "Forward" opens compose modal with body quoted, empty To field
- [ ] Sent reply appears in thread immediately
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-028: Sent folder view

**Description:** As a user, I can view my sent emails organized by thread.

**Acceptance Criteria:**
- [ ] Page at `/sent` showing threads where folder=sent (or threads containing outbound emails)
- [ ] Same layout as inbox list: recipient names, subject, preview, time
- [ ] Clicking navigates to thread view
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-029: Email actions — archive, trash, delete API

**Description:** As a user, I can archive, trash, or permanently delete emails/threads.

**Acceptance Criteria:**
- [ ] `PATCH /api/threads/:id/archive` — sets folder=archive on thread + all emails
- [ ] `PATCH /api/threads/:id/trash` — sets folder=trash, sets trash_expires_at to 30 days from now
- [ ] `DELETE /api/threads/:id` — permanent delete (only if already in trash)
- [ ] All endpoints scoped to current user's threads only
- [ ] Returns updated thread object
- [ ] Typecheck passes

---

### US-030: Email actions — star and read/unread API

**Description:** As a user, I can star threads and mark them as read/unread.

**Acceptance Criteria:**
- [ ] `PATCH /api/threads/:id/star` — toggles starred boolean on thread
- [ ] `PATCH /api/threads/:id/read` — marks all emails in thread as read, sets unread_count=0
- [ ] `PATCH /api/threads/:id/unread` — marks thread as unread (unread_count=1)
- [ ] All endpoints scoped to current user
- [ ] Typecheck passes

---

### US-031: Email actions — UI controls

**Description:** As a user, I can archive, trash, star, and mark threads read/unread from the UI.

**Acceptance Criteria:**
- [ ] Thread view header shows action buttons: Archive, Trash, Star, Mark Unread
- [ ] Inbox list: swipe or hover actions for Archive, Trash
- [ ] Star icon toggleable in both list and thread view
- [ ] Archive/Trash actions show toast with "Undo" option (reverts folder change)
- [ ] Optimistic UI updates (don't wait for API response)
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-032: Trash auto-purge background job

**Description:** As a developer, I need a scheduled job to permanently delete emails that have been in trash for 30+ days.

**Acceptance Criteria:**
- [ ] API route or cron endpoint: `POST /api/cron/purge-trash` (protected by secret key)
- [ ] Finds all emails where folder=trash AND trash_expires_at < now
- [ ] Permanently deletes expired emails and their threads (if thread is empty)
- [ ] Logs count of purged items
- [ ] Designed to be called by Vercel Cron (once daily)
- [ ] Typecheck passes

---

### US-033: File attachments — upload on send

**Description:** As a user, I can attach files when composing an email.

**Acceptance Criteria:**
- [ ] Compose modal has "Attach" button + drag-drop zone on the body area
- [ ] Files uploaded to S3-compatible storage, returns URL
- [ ] Max 50MB total per email (validate client + server side)
- [ ] Attached files shown as chips with filename, size, remove button
- [ ] On send: pass attachment URLs/data to Resend API
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-034: File attachments — display on received emails

**Description:** As a user, I can see and download attachments on received emails.

**Acceptance Criteria:**
- [ ] Thread view shows attachments list below email body (filename, size, file type icon)
- [ ] Click attachment to download (fetch from Resend Attachments API or S3)
- [ ] Image attachments show inline preview (thumbnail)
- [ ] Non-image attachments show file type icon
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-035: Search — full-text search API

**Description:** As a user, I can search my emails by keyword and get relevant results.

**Acceptance Criteria:**
- [ ] `GET /api/emails/search?q=keyword` searches subject + body_plain using PostgreSQL full-text search (tsvector/tsquery)
- [ ] Results scoped to current user's emails only
- [ ] Add GIN index on tsvector column for performance
- [ ] Results return: thread_id, subject, snippet (matching context), from, date
- [ ] Supports filtering by: folder, from address, date range (query params)
- [ ] Returns max 50 results, ordered by relevance + recency
- [ ] Typecheck passes

---

### US-036: Search — UI

**Description:** As a user, I have a search interface to find emails by keyword.

**Acceptance Criteria:**
- [ ] Page at `/search` with search input (Command component from shadcn)
- [ ] Search triggers on Enter or after 300ms debounce
- [ ] Results displayed as email list: sender, subject with highlighted match, date
- [ ] Clicking result navigates to thread view
- [ ] Empty state when no results found
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-037: User invite — API

**Description:** As an admin, I can invite team members to claim their email addresses.

**Acceptance Criteria:**
- [ ] `POST /api/users/invite` accepts: email (must match org domain), name, role. Admin-only.
- [ ] Creates User with status=invited, generates invite_token (crypto random), sets invite_expires_at (7 days)
- [ ] Sends invite email via Resend to a fallback/personal email (or the org email if they can receive externally)
- [ ] Invite email contains link: `/claim?token=xxx`
- [ ] Returns 409 if email already exists in org
- [ ] `POST /api/users/:id/reinvite` regenerates token and resends (for expired invites). Admin-only.
- [ ] Typecheck passes

---

### US-038: User invite — claim account page

**Description:** As an invited user, I can set my password and activate my account via the invite link.

**Acceptance Criteria:**
- [ ] Page at `/claim?token=xxx`
- [ ] Shows: "Welcome [name], set up your account for [email]"
- [ ] Form: password, confirm password (8+ chars)
- [ ] `POST /api/auth/claim` validates token, hashes password, sets status=active, claimed_at=now, clears token
- [ ] Returns 400 for expired/invalid/already-claimed tokens
- [ ] Redirects to `/inbox` on success with session set
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-039: Admin — user management UI

**Description:** As an admin, I can view, invite, and manage team members.

**Acceptance Criteria:**
- [ ] Page at `/settings/team` (admin-only)
- [ ] Table showing: name, email, role, status (active/invited/disabled), joined date
- [ ] "Invite User" button opens dialog: email, name, role select
- [ ] "Reinvite" button on expired/invited users
- [ ] "Disable" button on active users (sets status=disabled, cannot login)
- [ ] Cannot disable yourself
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-040: RBAC middleware enforcement

**Description:** As a developer, admin-only routes are protected and member users cannot access admin features.

**Acceptance Criteria:**
- [ ] `requireAdmin()` middleware helper that returns 403 for non-admin users
- [ ] Applied to: domain endpoints, user invite/manage endpoints, alias CRUD endpoints, catch-all toggle
- [ ] Member users see only their own emails (queries filtered by user_id)
- [ ] Admin users can view all org inboxes (future: for now, just enforce the boundary)
- [ ] Sidebar hides admin nav items for members
- [ ] Typecheck passes

---

### US-041: Alias — CRUD API

**Description:** As an admin, I can create, update, and delete aliases and assign users to them.

**Acceptance Criteria:**
- [ ] `POST /api/aliases` — create alias (address, name). Admin-only. Validates address matches org domain.
- [ ] `GET /api/aliases` — list all org aliases with assigned users
- [ ] `PATCH /api/aliases/:id` — update name, assigned users
- [ ] `DELETE /api/aliases/:id` — delete alias and all AliasUser records
- [ ] `POST /api/aliases/:id/users` — add user to alias (user_id, can_send_as)
- [ ] `DELETE /api/aliases/:id/users/:userId` — remove user from alias
- [ ] Typecheck passes

---

### US-042: Alias — send as alias

**Description:** As a user assigned to an alias with can_send_as=true, I can send emails from the alias address.

**Acceptance Criteria:**
- [ ] Compose modal shows "From" dropdown if user has send-able aliases
- [ ] Options: personal email + any alias with can_send_as=true
- [ ] When sending as alias: Resend `from` field set to alias address
- [ ] Email record stores which alias was used
- [ ] Reply in thread auto-selects the alias if original was delivered via that alias
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-043: Alias — admin management UI

**Description:** As an admin, I can manage aliases and their assigned users via the UI.

**Acceptance Criteria:**
- [ ] Page at `/settings/aliases` (admin-only)
- [ ] List of aliases showing: address, name, number of assigned users
- [ ] "Create Alias" button opens dialog: address prefix (auto-appends @domain), display name
- [ ] Click alias to expand: shows assigned users with can_send_as toggle
- [ ] "Add User" dropdown to assign existing org users
- [ ] "Remove" button to unassign user from alias
- [ ] Delete alias button with confirmation
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-044: Catch-all — admin toggle UI

**Description:** As an admin, I can enable/disable catch-all routing and see catch-all emails clearly labeled.

**Acceptance Criteria:**
- [ ] Toggle switch on domain settings page: "Catch-all routing" with description
- [ ] `PATCH /api/orgs/settings` endpoint to update catch_all_enabled. Admin-only.
- [ ] When enabled: description shows "Emails to unknown addresses delivered to admins"
- [ ] When disabled: description shows "Emails to unknown addresses will bounce"
- [ ] Inbox list shows badge "catch-all" on emails delivered via catch-all, with original address
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-045: Notifications — browser push setup

**Description:** As a user, I can opt-in to browser push notifications for new emails.

**Acceptance Criteria:**
- [ ] On first login, prompt for notification permission (non-blocking)
- [ ] If granted: register service worker for push notifications
- [ ] When `new_email` WebSocket event fires and tab is not focused: show browser notification (sender, subject preview)
- [ ] Clicking notification focuses the app tab and navigates to thread
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-046: Notifications — in-app indicators

**Description:** As a user, I see unread counts and visual indicators without needing browser notifications.

**Acceptance Criteria:**
- [ ] Tab title shows unread count: "(3) Inbox — Inboxes.net"
- [ ] Sidebar "Inbox" nav item shows unread count badge
- [ ] Favicon updates with unread indicator (small dot overlay)
- [ ] Counts update in real-time via WebSocket events
- [ ] Optional notification sound (off by default)
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-047: Notification preferences UI

**Description:** As a user, I can configure my notification preferences.

**Acceptance Criteria:**
- [ ] Section on `/settings` page: Notifications
- [ ] Toggle: Browser notifications (on/off)
- [ ] Toggle: Notification sound (on/off)
- [ ] Preferences stored on User model (json field or separate columns)
- [ ] `PATCH /api/users/me/preferences` endpoint
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-048: User settings page

**Description:** As a user, I can view and update my profile settings.

**Acceptance Criteria:**
- [ ] Page at `/settings` showing: name, email (read-only), role (read-only)
- [ ] Editable fields: name, password change (current + new + confirm)
- [ ] `PATCH /api/users/me` endpoint for name update
- [ ] `PATCH /api/users/me/password` endpoint (validates current password first)
- [ ] Success/error toasts on save
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-049: Recipient autocomplete

**Description:** As a user composing an email, I get autocomplete suggestions for recipients based on past contacts.

**Acceptance Criteria:**
- [ ] `GET /api/contacts/suggest?q=partial` returns matching email addresses from user's sent/received history
- [ ] Compose To/CC/BCC fields show dropdown suggestions as user types
- [ ] Matches on both email address and sender name
- [ ] Results deduplicated and sorted by frequency of contact
- [ ] Max 10 suggestions shown
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-050: Mobile responsiveness pass

**Description:** As a mobile user, the entire app is usable on small screens.

**Acceptance Criteria:**
- [ ] Inbox list: full-width, touch-friendly row height (min 48px tap targets)
- [ ] Thread view: full-width messages, reply form stacks vertically
- [ ] Compose: full-screen modal on mobile (Sheet component)
- [ ] Sidebar: hidden by default on mobile, shown via hamburger menu
- [ ] Settings pages: single-column layout on mobile
- [ ] All interactive elements have min 44px touch target
- [ ] No horizontal scroll on any page at 320px width
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

### US-051: Email HTML sanitization

**Description:** As a developer, I need to sanitize incoming email HTML to prevent XSS attacks.

**Acceptance Criteria:**
- [ ] Install DOMPurify (or isomorphic equivalent)
- [ ] All email body_html is sanitized before rendering in thread view
- [ ] Strips: script tags, event handlers (onclick etc), javascript: URLs
- [ ] Preserves: basic formatting (bold, italic, links, lists, images, tables)
- [ ] External images loaded via proxy or with user consent (to prevent tracking pixels)
- [ ] Typecheck passes

---

### US-052: Error handling and loading states

**Description:** As a user, I see appropriate loading states and error messages throughout the app.

**Acceptance Criteria:**
- [ ] All API calls show loading spinner/skeleton while pending
- [ ] Failed API calls show toast with error message
- [ ] Inbox list shows skeleton loader on initial fetch
- [ ] Thread view shows skeleton while loading messages
- [ ] Compose shows disabled Send button while sending
- [ ] Network error shows reconnection message
- [ ] 404 page for invalid routes
- [ ] Typecheck passes
- [ ] Verify changes work in browser

---

## Non-Goals

- Multiple domains per org (post-MVP)
- Native mobile apps (iOS/Android)
- Registrar API integrations for auto MX config
- SSO / OAuth login options
- Admin audit logs
- AI features (smart compose, summarization)
- Calendar or contacts integration
- Spam filtering beyond Resend's built-in handling
- Priority-based notifications or rules engine
- Email templates or scheduling
- End-to-end encryption
- Free tier

## Technical Considerations

- **Resend dependency:** All email send/receive goes through Resend. MX records point to Resend; inbound arrives via webhook.
- **PostgreSQL full-text search:** Use `tsvector` + GIN index for MVP search. Migrate to Typesense/Meilisearch if scale requires.
- **WebSocket scaling:** Single-server WebSocket works for MVP. Consider Redis pub/sub adapter for multi-server.
- **Attachment storage:** Cache frequently accessed attachments in S3; others fetched on-demand from Resend API.
- **Session management:** Signed JWT in httpOnly cookie. No refresh token complexity for MVP.
- **Resend webhook idempotency:** Use message_id as idempotency key to prevent duplicate email storage.
- **HTML email rendering:** Sanitize with DOMPurify. Consider iframe sandbox for extra isolation if needed.
