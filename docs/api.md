# API Reference

Complete endpoint reference for the Inboxes backend API.

## Authentication

All protected endpoints require a valid JWT in an `httpOnly` cookie named `token`. The cookie is set on login/signup/claim and cleared on logout.

- **Cookie name:** `token`
- **Signing:** HMAC-SHA256 (`HS256`)
- **Expiry:** 7 days
- **Flags:** `HttpOnly`, `SameSite=Lax`, `Secure` (when served over HTTPS), `Path=/`

**JWT claims:** `user_id`, `org_id`, `role` (`admin` | `member`), `jti`, `iat`, `exp`

**CSRF protection:** All state-changing requests (non-GET/HEAD/OPTIONS) to protected endpoints must include an `X-Requested-With` header (any non-empty value). Requests without it receive `403 Forbidden`.

## Error Format

All errors return JSON:

```json
{ "error": "human-readable message" }
```

Common HTTP status codes: `400` (validation), `401` (unauthenticated), `403` (forbidden), `404` (not found), `409` (conflict), `422` (unprocessable), `429` (rate limited), `500` (server error).

Rate-limited responses include a `Retry-After` header (seconds until the window resets).

## Rate Limits

| Endpoint | Limit | Window | Key |
|----------|-------|--------|-----|
| `GET /api/health` | 30 | 60 sec | IP |
| `GET /api/config` | 30 | 60 sec | IP |
| `POST /api/webhooks/resend/{orgId}` | 60 | 60 sec | IP |
| `POST /api/webhooks/stripe` | 60 | 60 sec | IP |
| `GET /api/setup/status` | 60 | 60 sec | IP |
| `POST /api/setup` | 3 | 15 min | IP |
| `POST /api/setup/validate-key` | 3 | 15 min | IP |
| `POST /api/auth/signup` | 5 | 1 hour | IP |
| `POST /api/auth/login` | 10/IP + 10/email | 15 min | IP + body `email` |
| `POST /api/auth/forgot-password` | 3/IP + 3/email | 1 hour | IP + body `email` |
| `POST /api/auth/reset-password` | 5 | 15 min | IP |
| `POST /api/auth/claim` | 5 | 15 min | IP |
| `GET /api/auth/claim/validate` | 10 | 15 min | IP |
| `POST /api/auth/verify-email` | 5/IP + 5/email | 15 min | IP + body `email` |
| `POST /api/auth/resend-verification` | 3/IP + 3/email | 1 hour | IP + body `email` |
| `PATCH /api/users/me/password` | 5 | 15 min | IP |
| `POST /api/emails/send` | 20/IP + 30/user | 60 sec | IP + user |
| `POST /api/drafts/{id}/send` | 20/IP + 30/user | 60 sec | IP + user |
| Admin cron/jobs group | 5 | 60 sec | IP |

Rate limiting uses Redis counters with automatic in-memory fallback when Redis is unavailable.

---

## Public Endpoints

No authentication required.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health check |

Rate limited: 30 requests/minute per IP.

**Response (200):**
```json
{ "status": "ok" }
```
Returns `503` with `{"status": "degraded"}` if either Postgres or Redis is unreachable.

### Runtime Config

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/config` | Public runtime configuration |

Rate limited: 30 requests/minute per IP.

**Response (200):**
```json
{ "api_url": "https://...", "ws_url": "wss://...", "commercial": false }
```
`commercial` is `true` when `STRIPE_KEY` is configured. Uses `Cache-Control: no-store`.

### Setup (Self-Hosted Only)

Available only when `STRIPE_KEY` is unset.

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `GET` | `/api/setup/status` | 60/min | Check if initial setup is needed |
| `POST` | `/api/setup` | 3/15min | Create first admin account and org |
| `POST` | `/api/setup/validate-key` | 3/15min | Validate a Resend API key |

**GET /api/setup/status** response:
```json
{ "needs_setup": true, "commercial": false, "system_email_configured": false }
```

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
Only succeeds when no users exist. Sets a JWT cookie on success. Returns `201`.

**POST /api/setup/validate-key** body:
```json
{ "api_key": "re_..." }
```
Response:
```json
{ "valid": true, "full_access": true, "domains": [{"id": "...", "name": "example.com", "status": "verified"}] }
```

### Auth

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `POST` | `/api/auth/signup` | 5/hr | Register a new account |
| `POST` | `/api/auth/login` | 10/15min | Log in, receive JWT cookie |
| `POST` | `/api/auth/forgot-password` | 3/hr | Send password reset email |
| `POST` | `/api/auth/reset-password` | 5/15min | Reset password with token |
| `POST` | `/api/auth/claim` | 5/15min | Claim an invited account |
| `GET` | `/api/auth/claim/validate` | 10/15min | Validate a claim/invite token |
| `POST` | `/api/auth/verify-email` | 5/15min | Verify email with 6-digit code |
| `POST` | `/api/auth/resend-verification` | 3/hr | Resend verification code |

**POST /api/auth/signup** body:
```json
{ "org_name": "My Org", "email": "user@example.com", "name": "User Name", "password": "..." }
```
In commercial mode, returns `{"requires_verification": true, "email": "..."}` (201) and sends a 6-digit verification code. In self-hosted mode (no Stripe), returns a user object and sets a JWT cookie. Blocked when users already exist in self-hosted mode.

**POST /api/auth/login** body:
```json
{ "email": "user@example.com", "password": "..." }
```
Response (200):
```json
{ "user": {"id": "...", "org_id": "...", "email": "...", "name": "...", "role": "admin"}, "onboarding_completed": true }
```
Returns `403` with `"email_not_verified"` if email verification is pending (commercial mode).

**POST /api/auth/forgot-password** body:
```json
{ "email": "user@example.com" }
```
Always returns `200` with `{"message": "if that email exists, a reset link has been sent"}` (does not reveal email existence).

**POST /api/auth/reset-password** body:
```json
{ "token": "hex-token", "password": "new-password" }
```
Revokes all existing sessions on success.

**POST /api/auth/claim** body:
```json
{ "token": "hex-token", "name": "User Name", "password": "..." }
```
Sets JWT cookie and returns user object.

**GET /api/auth/claim/validate** query params: `token`
Response (200):
```json
{ "email": "invited@example.com", "name": "...", "status": "invited" }
```

**POST /api/auth/verify-email** body:
```json
{ "email": "user@example.com", "code": "123456" }
```
Sets JWT cookie and returns user object on success.

**POST /api/auth/resend-verification** body:
```json
{ "email": "user@example.com" }
```
Always returns `200` (does not reveal email existence).

### Webhooks

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `POST` | `/api/webhooks/resend/{orgId}` | 60/min | Resend webhook receiver (Svix signature-verified) |
| `POST` | `/api/webhooks/stripe` | 60/min | Stripe webhook receiver (commercial mode only) |

Webhooks use their own signature verification (Svix for Resend, Stripe SDK for Stripe), not JWT auth. The `orgId` parameter must be a valid UUID. Returns `410 Gone` for deleted orgs to signal Resend to stop delivering.

### WebSocket

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/ws` | WebSocket connection |

Authenticates via JWT cookie passed as a query parameter or cookie during the upgrade handshake.

---

## Protected Endpoints

Require valid JWT cookie. All state-changing requests require `X-Requested-With` header.

### Logout

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/logout` | Clear auth cookie and revoke token |

Accessible even without an active plan. Response: `{"message": "logged out"}`.

### Org Settings

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/orgs/settings` | Any user | Get org settings |
| `PATCH` | `/api/orgs/settings` | **Admin only** | Update org settings |

Accessible even without an active plan.

**GET /api/orgs/settings** response:
```json
{
  "id": "uuid",
  "name": "Org Name",
  "onboarding_completed": true,
  "has_api_key": true,
  "billing_enabled": false,
  "resend_rps": 10,
  "auto_poll_enabled": true,
  "auto_poll_interval": 300
}
```
`auto_poll_enabled` and `auto_poll_interval` are only included in self-hosted mode.

**PATCH /api/orgs/settings** body (all fields optional):
```json
{
  "name": "New Name",
  "api_key": "re_...",
  "resend_rps": 10,
  "auto_poll_enabled": true,
  "auto_poll_interval": 300
}
```
- `resend_rps`: 1-100
- `auto_poll_interval`: 120-3600 seconds (self-hosted only)
- `auto_poll_enabled`: self-hosted only

Returns `204 No Content`.

### User Profile

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `GET` | `/api/users/me` | -- | Get current user profile |
| `PATCH` | `/api/users/me` | -- | Update profile (name) |
| `PATCH` | `/api/users/me/password` | 5/15min | Change password |
| `GET` | `/api/users/me/preferences` | -- | Get user preferences (JSON blob) |
| `PATCH` | `/api/users/me/preferences` | -- | Update preferences (merge) |
| `GET` | `/api/users/me/sessions` | -- | List active sessions |
| `DELETE` | `/api/users/me/sessions/{jti}` | -- | Revoke a session |

Accessible even without an active plan.

**PATCH /api/users/me** body:
```json
{ "name": "New Name" }
```
Returns `204 No Content`.

**PATCH /api/users/me/password** body:
```json
{ "current_password": "...", "new_password": "..." }
```
Revokes all other sessions and issues a new JWT for the current session. Returns `204 No Content`.

**PATCH /api/users/me/preferences** body: arbitrary JSON object (merged into existing preferences).
Returns `204 No Content`.

**GET /api/users/me/sessions** response:
```json
[{"jti": "uuid", "created_at": "...", "current": true}]
```

**DELETE /api/users/me/sessions/{jti}**: Cannot revoke the current session. Returns `204 No Content`.

### Billing

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/billing` | Any user | Get billing status and plan info |
| `POST` | `/api/billing/checkout` | **Admin only** | Create Stripe checkout session |
| `POST` | `/api/billing/portal` | **Admin only** | Create Stripe billing portal URL |

Accessible even without an active plan (needed to subscribe).

**GET /api/billing** response:
```json
{
  "plan": "pro",
  "plan_expires_at": null,
  "billing_enabled": true,
  "subscription": {
    "status": "active",
    "cancel_at_period_end": false,
    "current_period_end": "2025-02-01T00:00:00Z"
  }
}
```
`plan` values: `free`, `pro`, `past_due`, `cancelled`.

**POST /api/billing/checkout** response:
```json
{ "url": "https://checkout.stripe.com/..." }
```

**POST /api/billing/portal** response:
```json
{ "url": "https://billing.stripe.com/..." }
```

### System Settings (Self-Hosted, Owner Only)

Available only when `STRIPE_KEY` is unset. Requires `is_owner = true` on the user record.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/system/email` | Get system email configuration |
| `PATCH` | `/api/system/email` | Update system from-address |

**GET /api/system/email** response:
```json
{ "from_address": "noreply@example.com", "from_name": "Inboxes" }
```

**PATCH /api/system/email** body:
```json
{
  "from_address": "noreply@example.com",
  "from_name": "Inboxes",
  "send_test": true
}
```
When `send_test` is true, sends a test email to the current user. Response:
```json
{ "saved": true, "test_sent": true }
```

### Sync Jobs

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/sync` | **Admin only** | Start an email sync job |
| `GET` | `/api/sync/{id}` | Any user | Get sync job status |

**POST /api/sync** response:
```json
{
  "id": "uuid",
  "status": "pending",
  "phase": "pending",
  "imported": 0,
  "total": 0,
  "already_active": false
}
```
If a sync job is already running, returns the existing job with `"already_active": true`.

**GET /api/sync/{id}** returns the full sync job object including `status`, `phase`, `imported`, `total`, `sent_count`, `received_count`, `thread_count`, `address_count`, `created_at`, and `error` (if failed).

### Events (Catchup)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/events` | Get events after a given ID for WebSocket reconnection catchup |

**Query params:**
- `since` (int64) -- last event ID the client received
- `limit` (int, default 100, max 200)

**Response (200):**
```json
{ "events": [{"id": 123, "event": "thread.new", "domain_id": "...", "thread_id": "...", "payload": {}, "created_at": "..."}] }
```

Returns `410 Gone` if the `since` ID points to an event older than the catchup window (default 48 hours), signaling the client to do a full data refetch.

---

## Admin-Only Endpoints

Require `role = admin`. Protected by JWT auth.

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `POST` | `/api/cron/purge-trash` | 5/min | Manually trigger trash purge |
| `POST` | `/api/cron/cleanup-webhooks` | 5/min | Clean up stale Resend webhooks |
| `GET` | `/api/admin/jobs` | 5/min | List email job queue status |

**POST /api/cron/purge-trash** response:
```json
{ "purged": 5 }
```

**POST /api/cron/cleanup-webhooks** response:
```json
{ "cleaned": 2, "failed": 0 }
```

**GET /api/admin/jobs** response:
```json
{ "jobs": [...] }
```

---

## Plan-Required Endpoints

Require an active subscription (commercial mode) or any authenticated user (self-hosted). Enforced by `RequirePlan` middleware. Returns `402 Payment Required` with `{"error": "subscription_required"}` when no active plan.

### Onboarding

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/onboarding/status` | Get current onboarding step |
| `POST` | `/api/onboarding/connect` | Submit Resend API key |
| `POST` | `/api/onboarding/domains` | Select domains to import |
| `POST` | `/api/onboarding/webhook` | Register Resend webhook |
| `GET` | `/api/onboarding/addresses` | List discovered email addresses |
| `POST` | `/api/onboarding/addresses` | Configure aliases from addresses |
| `POST` | `/api/onboarding/complete` | Mark onboarding complete |

**GET /api/onboarding/status** response:
```json
{ "step": "connect" }
```
Possible steps: `connect`, `domains`, `sync`, `addresses`. May include `sync_in_progress` (bool) and `sync_job_id` when a sync is active.

**POST /api/onboarding/connect** body:
```json
{ "api_key": "re_..." }
```
Validates the key against Resend, encrypts and stores it, upserts discovered domains. Response:
```json
{ "domains": [{"id": "uuid", "domain": "example.com", "resend_domain_id": "...", "status": "active"}] }
```

**POST /api/onboarding/domains** body:
```json
{ "domain_ids": ["uuid1", "uuid2"] }
```
Marks selected domains as visible, hides the rest.

**POST /api/onboarding/webhook** response:
```json
{ "webhook_id": "...", "webhook_url": "https://..." }
```
Returns `{"webhook_skipped": true, "reason": "..."}` if `PUBLIC_URL` is localhost.

**POST /api/onboarding/addresses** body:
```json
{
  "addresses": [
    { "address": "hello@example.com", "type": "individual", "name": "Hello" },
    { "address": "team@example.com", "type": "group", "name": "Team" },
    { "address": "noreply@example.com", "type": "skip", "name": "" }
  ]
}
```

**POST /api/onboarding/complete** response:
```json
{ "message": "onboarding completed", "first_domain_id": "uuid" }
```

### Threads

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/threads` | List threads |
| `PATCH` | `/api/threads/bulk` | Bulk action on multiple threads |
| `GET` | `/api/threads/{id}` | Get thread with all emails |
| `PATCH` | `/api/threads/{id}/read` | Mark thread as read |
| `PATCH` | `/api/threads/{id}/unread` | Mark thread as unread |
| `PATCH` | `/api/threads/{id}/star` | Toggle or set star |
| `PATCH` | `/api/threads/{id}/archive` | Move to archive |
| `PATCH` | `/api/threads/{id}/trash` | Move to trash (30-day auto-purge) |
| `PATCH` | `/api/threads/{id}/spam` | Mark as spam or not-spam |
| `PATCH` | `/api/threads/{id}/mute` | Toggle mute |
| `PATCH` | `/api/threads/{id}/move` | Move to a label/folder |
| `DELETE` | `/api/threads/{id}` | Permanently delete (must be in trash) |

Non-admin users can only see and act on threads associated with their assigned aliases.

**GET /api/threads** query params:
- `domain_id` -- filter by domain (optional)
- `label` -- `inbox` (default), `sent`, `archive`, `trash`, `spam`, `starred`, or custom label name
- `page` (default 1)
- `limit` (default 50, max 200)

Response:
```json
{ "threads": [...], "page": 1, "total": 42 }
```

**PATCH /api/threads/bulk** body:
```json
{
  "thread_ids": ["uuid1", "uuid2"],
  "action": "archive",
  "label": "custom-label",
  "select_all": false,
  "filter_label": "inbox",
  "filter_domain_id": "uuid"
}
```
Actions: `archive`, `trash`, `spam`, `read`, `unread`, `move`, `label`, `unlabel`, `mute`, `unmute`, `delete`.
- `label` is required for `move`, `label`, and `unlabel` actions.
- `select_all: true` resolves thread IDs from filters instead of using `thread_ids`.
- `delete` only affects threads that are already in trash.

Response: `{"message": "updated", "affected": 5}`

**GET /api/threads/{id}** response:
```json
{ "thread": {"id": "...", "subject": "...", "emails": [...], ...} }
```

**PATCH /api/threads/{id}/star** body (optional):
```json
{ "starred": true }
```
If no body is provided, toggles the current star state.

**PATCH /api/threads/{id}/spam** body (optional):
```json
{ "action": "not_spam" }
```
When `action` is `"not_spam"`, removes the spam label and moves the thread back to inbox.

**PATCH /api/threads/{id}/move** body:
```json
{ "label": "inbox" }
```
Valid system labels: `inbox`, `trash`, `spam`, `archive`. Custom label names are also accepted.

**DELETE /api/threads/{id}**: Only threads with the `trash` label can be permanently deleted. Returns `404` otherwise.

### Emails

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `POST` | `/api/emails/send` | 20/IP + 30/user per min | Send an email (queued via job system) |
| `GET` | `/api/emails/search` | -- | Search emails |

**POST /api/emails/send** body:
```json
{
  "domain_id": "uuid",
  "from": "hello@example.com",
  "to": ["recipient@example.com"],
  "cc": [],
  "bcc": [],
  "subject": "Hello",
  "html": "<p>Body</p>",
  "text": "Body",
  "reply_to_thread_id": "uuid",
  "in_reply_to": "<message-id@example.com>",
  "references": ["<ref1@example.com>"],
  "attachment_ids": ["uuid"]
}
```
Validates sender authorization, checks bounce block list, enforces 50-recipient and 512KB body limits. Returns `202 Accepted`:
```json
{ "email_id": "uuid", "thread_id": "uuid", "job_id": "uuid", "status": "queued" }
```

**GET /api/emails/search** query params:
- `q` (required) -- search query
- `domain_id` (optional) -- filter by domain

Response:
```json
{ "threads": [...] }
```

### Domains

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/domains` | Any user | List visible domains |
| `GET` | `/api/domains/all` | Any user | List all domains (including hidden) |
| `POST` | `/api/domains` | Any user | Add a new domain (creates in Resend) |
| `POST` | `/api/domains/{id}/verify` | Any user | Verify domain DNS records |
| `POST` | `/api/domains/{id}/webhook` | **Admin only** | Re-register Resend webhook |
| `DELETE` | `/api/domains/{id}` | **Admin only** | Soft-delete a domain (cascades) |
| `PATCH` | `/api/domains/reorder` | Any user | Reorder sidebar domains |
| `PATCH` | `/api/domains/visibility` | **Admin only** | Show/hide domains in sidebar |
| `GET` | `/api/domains/unread-counts` | Any user | Unread counts per domain |
| `POST` | `/api/domains/sync` | Any user | Sync domain list from Resend |

**POST /api/domains** body:
```json
{ "domain": "example.com" }
```
Response (201):
```json
{ "id": "uuid", "domain": "example.com", "resend_domain_id": "...", "status": "pending", "dns_records": [...] }
```

**PATCH /api/domains/reorder** body:
```json
{ "order": [{"id": "uuid1", "order": 0}, {"id": "uuid2", "order": 1}] }
```
Returns `204 No Content`.

**PATCH /api/domains/visibility** body:
```json
{ "visible": ["uuid1", "uuid2"] }
```
All domains not in the `visible` list are hidden. At least one domain must remain visible. Returns `204 No Content`.

**POST /api/domains/{id}/webhook** response:
```json
{ "webhook_id": "...", "webhook_url": "https://..." }
```

**POST /api/domains/sync**: Syncs domain statuses from Resend and returns the full domain list (including hidden).

### Users (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/users` | **Admin only** | List org users |
| `POST` | `/api/users/invite` | **Admin only** | Invite a new user |
| `POST` | `/api/users/{id}/reinvite` | **Admin only** | Resend invite email |
| `PATCH` | `/api/users/{id}/disable` | **Admin only** | Disable user |
| `PATCH` | `/api/users/{id}/role` | **Admin only** | Change user role |
| `PATCH` | `/api/users/{id}/enable` | **Admin only** | Re-enable disabled user |
| `GET` | `/api/users/me/aliases` | Any user | List current user's aliases |

**POST /api/users/invite** body:
```json
{ "email": "newuser@example.com", "name": "New User", "role": "member" }
```
`role` defaults to `"member"` if omitted. Sends an invite email. Response (201):
```json
{ "id": "uuid", "status": "invited" }
```

**PATCH /api/users/{id}/disable** body (optional):
```json
{ "target_user_id": "uuid" }
```
When `target_user_id` is provided, reassigns the disabled user's threads, aliases, and drafts to the target user before disabling. Cannot disable yourself. At least 2 active admins must remain when disabling an admin.

**PATCH /api/users/{id}/role** body:
```json
{ "role": "admin" }
```
Valid roles: `admin`, `member`. Cannot change your own role or the owner's role. At least 2 active admins must remain when demoting an admin. Response:
```json
{ "role": "admin" }
```

### Org Deletion

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `DELETE` | `/api/orgs` | **Owner only** | Soft-delete organization |
| `DELETE` | `/api/orgs/hard` | **Owner only** | Permanently delete organization |

Only the org owner (`is_owner = true`) can delete the organization. Both endpoints cancel Stripe subscriptions and unregister Resend webhooks (best-effort).

**DELETE /api/orgs**: Soft-deletes the org and cascades to all child entities. Revokes all user sessions. Returns `204 No Content`.

**DELETE /api/orgs/hard** body:
```json
{ "confirm": "DELETE Org Name" }
```
The `confirm` value must exactly match `"DELETE "` followed by the org name. Permanently deletes all org data. Returns `204 No Content`.

### Aliases

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/aliases` | Any user | List all aliases |
| `POST` | `/api/aliases` | **Admin only** | Create alias |
| `PATCH` | `/api/aliases/{id}` | **Admin only** | Update alias name |
| `DELETE` | `/api/aliases/{id}` | **Admin only** | Delete alias |
| `POST` | `/api/aliases/{id}/users` | **Admin only** | Add user to alias |
| `DELETE` | `/api/aliases/{id}/users/{userId}` | **Admin only** | Remove user from alias |
| `PATCH` | `/api/aliases/{id}/default` | Any user | Set as default send-from alias |
| `GET` | `/api/aliases/discovered` | Any user | List discovered addresses |

**GET /api/aliases** query params:
- `domain_id` (optional) -- filter by domain

**POST /api/aliases** body:
```json
{ "address": "hello@example.com", "name": "Hello", "domain_id": "uuid" }
```
Response (201):
```json
{ "id": "uuid", "address": "hello@example.com", "name": "Hello" }
```

**PATCH /api/aliases/{id}** body:
```json
{ "name": "Updated Name" }
```

**POST /api/aliases/{id}/users** body:
```json
{ "user_id": "uuid", "can_send_as": true }
```
Returns `204 No Content`.

**PATCH /api/aliases/{id}/default**: Sets this alias as the current user's default for sending. Returns `204 No Content`. Returns `403` if the user does not have access to the alias.

### Drafts

| Method | Path | Rate Limit | Description |
|--------|------|------------|-------------|
| `GET` | `/api/drafts` | -- | List drafts |
| `POST` | `/api/drafts` | -- | Create draft |
| `PATCH` | `/api/drafts/{id}` | -- | Update draft |
| `DELETE` | `/api/drafts/{id}` | -- | Delete draft |
| `POST` | `/api/drafts/{id}/send` | 20/IP + 30/user per min | Send a draft |

**GET /api/drafts** query params:
- `domain_id` (optional) -- filter by domain

Response:
```json
{ "drafts": [...] }
```

**POST /api/drafts** body:
```json
{
  "domain_id": "uuid",
  "thread_id": "uuid",
  "kind": "compose",
  "subject": "...",
  "from_address": "hello@example.com",
  "to_addresses": ["recipient@example.com"],
  "cc_addresses": [],
  "bcc_addresses": [],
  "body_html": "<p>...</p>",
  "body_plain": "...",
  "attachment_ids": []
}
```
`domain_id` is required. `kind` defaults to `"compose"`. Response (201):
```json
{ "id": "uuid" }
```

**PATCH /api/drafts/{id}** body (all fields optional):
```json
{
  "subject": "...",
  "from_address": "...",
  "to_addresses": [...],
  "cc_addresses": [...],
  "bcc_addresses": [...],
  "body_html": "...",
  "body_plain": "...",
  "attachment_ids": [...]
}
```
Returns `204 No Content`.

**POST /api/drafts/{id}/send**: Validates sender authorization, checks bounce block list, creates an email job. Returns `409 Conflict` if the draft is already being sent. Response (202):
```json
{ "email_id": "uuid", "thread_id": "uuid", "job_id": "uuid", "status": "queued" }
```
The draft is deleted by the worker after successful send.

### Labels

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/labels` | List custom labels |
| `POST` | `/api/labels` | Create label |
| `PATCH` | `/api/labels/{id}` | Rename label |
| `DELETE` | `/api/labels/{id}` | Delete label |

System labels (`inbox`, `sent`, `trash`, `spam`, `starred`, `archive`, `drafts`) and `alias:*` labels are reserved and cannot be used for custom labels.

**POST /api/labels** body:
```json
{ "name": "Projects" }
```
Max 100 characters. Response (201):
```json
{ "id": "uuid", "name": "Projects" }
```

**PATCH /api/labels/{id}** body:
```json
{ "name": "New Name" }
```
Renames the label in the org_labels table (thread_labels use the label name directly). Response:
```json
{ "id": "uuid", "name": "New Name" }
```

### Contacts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/contacts/suggest` | Autocomplete contacts |

**GET /api/contacts/suggest** query params:
- `q` (required) -- search query

Returns up to 10 suggestions based on previous email participants in the org.

### Attachments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/attachments/upload` | Upload attachment (multipart/form-data) |
| `GET` | `/api/attachments/{id}/meta` | Get attachment metadata |
| `GET` | `/api/attachments/{id}` | Download attachment |

**POST /api/attachments/upload**: Accepts `multipart/form-data` with a `file` field. Max 10MB. Executable file types are blocked. Response (201):
```json
{ "id": "uuid", "filename": "document.pdf", "content_type": "application/pdf", "size": 12345 }
```

**GET /api/attachments/{id}/meta** response:
```json
{ "id": "uuid", "filename": "document.pdf", "content_type": "application/pdf", "size": 12345 }
```

**GET /api/attachments/{id}**: Returns the binary file content with appropriate `Content-Type`, `Content-Disposition`, and `X-Content-Type-Options: nosniff` headers. Unsafe content types are served as `application/octet-stream`.

Attachments are stored as BYTEA in PostgreSQL.
