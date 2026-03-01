# API Reference

Complete endpoint reference for the Inboxes backend API.

## Authentication

All protected endpoints require a valid JWT in an `httpOnly` cookie named `token`. The cookie is set on login/signup and cleared on logout.

**JWT claims:** `user_id`, `org_id`, `role` (`admin` | `member`), `jti`, `iat`, `exp`

## Error Format

All errors return JSON:

```json
{ "error": "human-readable message" }
```

Common HTTP status codes: `400` (validation), `401` (unauthenticated), `403` (forbidden), `404` (not found), `409` (conflict), `429` (rate limited), `500` (server error).

Rate-limited responses include a `Retry-After` header (seconds).

## Rate Limits

| Endpoint | Limit | Window |
|----------|-------|--------|
| `POST /api/setup` | 3 | 15 min |
| `GET /api/setup/status` | 5 | 15 min |
| `POST /api/setup/validate-key` | 3 | 15 min |
| `POST /api/auth/signup` | 5 | 1 hour |
| `POST /api/auth/login` | 10 | 15 min |
| `POST /api/auth/forgot-password` | 3/IP + 3/email | 1 hour |
| `POST /api/auth/reset-password` | 5 | 15 min |
| `POST /api/auth/claim` | 5 | 15 min |
| `GET /api/auth/claim/validate` | 10 | 15 min |
| `POST /api/auth/verify-email` | 5 | 15 min |
| `POST /api/auth/resend-verification` | 3 | 1 hour |

All rate limits are per-IP using Redis counters. `forgot-password` also rate-limits by the `email` field in the request body.

---

## Public Endpoints

No authentication required.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Returns service health status |

**Response:**
```json
{ "status": "ok", "db": true, "redis": true }
```
Returns `503` with `"status": "degraded"` if either dependency is down.

### Runtime Config

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/config` | Returns public runtime configuration |

**Response:**
```json
{ "api_url": "https://...", "ws_url": "wss://...", "commercial": false }
```
Cached for 1 hour (`Cache-Control: public, max-age=3600`).

### Setup (Self-Hosted Only)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/setup/status` | Check if initial setup is needed |
| `POST` | `/api/setup` | Create first admin account + org |
| `POST` | `/api/setup/validate-key` | Validate a Resend API key |

**POST /api/setup** body:
```json
{
  "name": "Admin Name",
  "email": "admin@example.com",
  "password": "...",
  "system_resend_key": "re_...",
  "system_from_address": "noreply@example.com",
  "system_from_name": "Inboxes"
}
```
Only works when `STRIPE_KEY` is unset and no users exist.

### Auth

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/signup` | Register a new account (commercial mode) |
| `POST` | `/api/auth/login` | Log in, receive JWT cookie |
| `POST` | `/api/auth/forgot-password` | Send password reset email |
| `POST` | `/api/auth/reset-password` | Reset password with token |
| `POST` | `/api/auth/claim` | Claim an invited account |
| `GET` | `/api/auth/claim/validate` | Validate a claim token |
| `POST` | `/api/auth/verify-email` | Verify email with code |
| `POST` | `/api/auth/resend-verification` | Resend verification code |

### Webhooks

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/webhooks/resend/{orgId}` | Resend webhook receiver (signature-verified) |
| `POST` | `/api/webhooks/stripe` | Stripe webhook receiver (commercial mode only) |

Webhooks use their own signature verification, not JWT auth.

---

## Protected Endpoints

Require valid JWT cookie. All responses are JSON.

### Auth (Protected)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/logout` | Clear auth cookie, revoke token |

Accessible even without an active plan.

### Org Settings

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/orgs/settings` | Get org settings |
| `PATCH` | `/api/orgs/settings` | Update org settings |

### User Profile

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users/me` | Get current user profile |
| `PATCH` | `/api/users/me` | Update profile (name) |
| `PATCH` | `/api/users/me/password` | Change password |
| `GET` | `/api/users/me/preferences` | Get user preferences |
| `PATCH` | `/api/users/me/preferences` | Update preferences |

### Billing

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/billing` | Get billing status and plan info |
| `POST` | `/api/billing/checkout` | Create Stripe checkout session |
| `POST` | `/api/billing/portal` | Create Stripe customer portal URL |

### System Settings (Self-Hosted, Owner Only)

Only available when `STRIPE_KEY` is unset. Requires `is_owner = true`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/system/email` | Get system email configuration |
| `PATCH` | `/api/system/email` | Update system from-address |

**PATCH /api/system/email** body:
```json
{
  "from_address": "noreply@example.com",
  "from_name": "Inboxes",
  "send_test": true
}
```

### Sync

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/sync` | Start an email sync job |
| `GET` | `/api/sync/{id}` | Get sync job status |

### Events

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/events?since={id}` | Get events after a given ID (for WS reconnection catchup) |

---

## Admin-Only Endpoints

Require `role = admin`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/cron/purge-trash` | Manually trigger trash purge |
| `POST` | `/api/cron/cleanup-webhooks` | Clean up stale webhooks |
| `GET` | `/api/admin/jobs` | List email job queue status |

---

## Plan-Required Endpoints

Require an active subscription (commercial mode) or any authenticated user (self-hosted). Enforced by `RequirePlan` middleware.

### Onboarding

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/onboarding/status` | Get onboarding progress |
| `POST` | `/api/onboarding/connect` | Submit Resend API key |
| `POST` | `/api/onboarding/domains` | Select domains to import |
| `POST` | `/api/onboarding/webhook` | Register Resend webhook |
| `GET` | `/api/onboarding/addresses` | List discovered email addresses |
| `POST` | `/api/onboarding/addresses` | Configure aliases from addresses |
| `POST` | `/api/onboarding/complete` | Mark onboarding complete |

### Threads

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/threads?domain_id={id}&label={label}&page={n}` | List threads |
| `GET` | `/api/threads/unread-count` | Get total unread count |
| `PATCH` | `/api/threads/bulk` | Bulk action on multiple threads |
| `GET` | `/api/threads/{id}` | Get thread with all emails |
| `PATCH` | `/api/threads/{id}/read` | Mark thread as read |
| `PATCH` | `/api/threads/{id}/unread` | Mark thread as unread |
| `PATCH` | `/api/threads/{id}/star` | Toggle star |
| `PATCH` | `/api/threads/{id}/archive` | Move to archive |
| `PATCH` | `/api/threads/{id}/trash` | Move to trash (30-day expiry) |
| `PATCH` | `/api/threads/{id}/spam` | Move to spam |
| `PATCH` | `/api/threads/{id}/mute` | Toggle mute |
| `PATCH` | `/api/threads/{id}/move` | Move to a label/folder |
| `DELETE` | `/api/threads/{id}` | Permanently delete |

**GET /api/threads** query params: `domain_id` (required), `label` (inbox/sent/archive/trash/spam/starred/custom), `page`, `per_page`, `unread_only`.

### Emails

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/emails/send` | Send an email (queued via job system) |
| `GET` | `/api/emails/search?q={query}&domain_id={id}` | Search emails |

### Domains

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/domains` | List visible domains |
| `GET` | `/api/domains/all` | List all domains (including hidden) |
| `POST` | `/api/domains` | Add a new domain |
| `POST` | `/api/domains/{id}/verify` | Verify domain DNS |
| `POST` | `/api/domains/{id}/webhook` | Re-register Resend webhook |
| `DELETE` | `/api/domains/{id}` | Soft-delete a domain |
| `PATCH` | `/api/domains/reorder` | Reorder sidebar domains |
| `PATCH` | `/api/domains/visibility` | Show/hide domains in sidebar |
| `GET` | `/api/domains/unread-counts` | Unread counts per domain |
| `POST` | `/api/domains/sync` | Trigger domain sync |

### Users (Admin)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users` | List org users |
| `POST` | `/api/users/invite` | Invite a new user |
| `POST` | `/api/users/{id}/reinvite` | Resend invite email |
| `PATCH` | `/api/users/{id}/disable` | Disable user (with reassignment) |
| `PATCH` | `/api/users/{id}/role` | Change user role |
| `PATCH` | `/api/users/{id}/enable` | Re-enable disabled user |
| `GET` | `/api/users/me/aliases` | List current user's aliases |

### Orgs

| Method | Path | Description |
|--------|------|-------------|
| `DELETE` | `/api/orgs` | Soft-delete organization (admin only) |

### Aliases

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/aliases` | List all aliases |
| `POST` | `/api/aliases` | Create alias |
| `PATCH` | `/api/aliases/{id}` | Update alias |
| `DELETE` | `/api/aliases/{id}` | Delete alias |
| `POST` | `/api/aliases/{id}/users` | Add user to alias |
| `DELETE` | `/api/aliases/{id}/users/{userId}` | Remove user from alias |
| `PATCH` | `/api/aliases/{id}/default` | Set default alias |
| `GET` | `/api/aliases/discovered` | List discovered addresses |

### Drafts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/drafts` | List drafts |
| `POST` | `/api/drafts` | Create draft |
| `PATCH` | `/api/drafts/{id}` | Update draft |
| `DELETE` | `/api/drafts/{id}` | Delete draft |
| `POST` | `/api/drafts/{id}/send` | Send a draft |

### Labels

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/labels` | List custom labels |
| `POST` | `/api/labels` | Create label |
| `PATCH` | `/api/labels/{id}` | Rename label |
| `DELETE` | `/api/labels/{id}` | Delete label |

### Contacts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/contacts/suggest?q={query}` | Autocomplete contacts |
| `GET` | `/api/contacts/lookup?email={addr}` | Look up contact info |

### Attachments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/attachments/upload` | Upload attachment (multipart/form-data) |
| `GET` | `/api/attachments/{id}` | Download attachment |

Attachments are stored as BYTEA in PostgreSQL.

---

## WebSocket

| Path | Auth | Description |
|------|------|-------------|
| `GET` | `/api/ws` | WebSocket connection (JWT cookie) |

See [WebSocket Events](websocket.md) for event types and payloads.
