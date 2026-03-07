# Self-Hosted vs Commercial Mode

Inboxes runs in one of two modes, determined by a single environment variable at startup.

## How Mode Is Determined

- **`STRIPE_KEY` is set and non-empty** -- Commercial mode (billing, email verification, open signup)
- **`STRIPE_KEY` is unset or empty** -- Self-hosted mode (no billing, no email verification, invite-only after initial setup)

There is no config file or feature flag. The presence of `STRIPE_KEY` is checked once at startup and passed through to route registration, handler constructors, and middleware.

---

## Self-Hosted Mode

### Setup Wizard

On first launch with zero users in the database, the setup wizard is available at three endpoints:

1. `GET /api/setup/status` -- Returns `needs_setup: true` when zero users exist, along with `commercial: false` and whether a system email key is configured.
2. `POST /api/setup/validate-key` -- Accepts `{"api_key": "..."}` and tests the key against the Resend API. Returns available domains with their IDs, names, and statuses. This endpoint is **not** locked after setup completes; it remains accessible at any time (rate-limited to 3 requests per 15 minutes per IP).
3. `POST /api/setup` -- Creates the first org and admin user. Only available when `STRIPE_KEY` is unset **and** zero users exist. Returns `403` if either condition fails.

The setup endpoint accepts:

| Field | Required | Description |
|-------|----------|-------------|
| `email` | Yes | Admin email address |
| `password` | Yes | Admin password |
| `name` | No | Display name (defaults to email prefix) |
| `system_resend_key` | No | Encrypted and stored in `system_settings` for system emails |
| `system_from_address` | No | "From" address for system emails (invites, password resets) |
| `system_from_name` | No | Display name for system emails |

After setup, a JWT cookie is set automatically so the admin proceeds directly to onboarding without a separate login step.

### First-User Signup Fallback

The `POST /api/auth/signup` endpoint serves as an alternative to the setup wizard. In self-hosted mode, signup is allowed only when zero users exist in the database. The first user created through signup gets `email_verified = true` (skipping verification) and is assigned the `admin` role. Once any user exists, the signup endpoint returns `403 "registration is closed"`.

This means there are two paths to bootstrap a self-hosted instance:

1. **Setup wizard** (`POST /api/setup`) -- provides system email configuration in the same step
2. **Signup** (`POST /api/auth/signup`) -- simpler, but does not configure system email settings

### Invite-Only After Setup

After the first user exists, new users can only be added via `POST /api/users/invite` (requires admin role). Both the setup wizard and the signup endpoint are locked.

### No Email Verification

In self-hosted mode, all users are created with `email_verified = true`. There is no verification code step for signup, setup, or invite claim.

### System Email Configuration

The instance owner (`is_owner = true` in the database) can manage system email settings through owner-only endpoints. These endpoints are registered only when `STRIPE_KEY` is unset:

- `GET /api/system/email` -- Returns current `from_address` and `from_name`
- `PATCH /api/system/email` -- Updates from-address and from-name, with optional `send_test: true` to dispatch a test email

Both endpoints require the `RequireOwner` middleware, which queries `is_owner` from the users table.

### Auto-Poll (Inbox Poller)

Self-hosted instances that cannot receive webhooks (e.g., behind NAT, no public URL) can enable automatic polling. Two per-org settings control this behavior:

| Setting | Type | Default | Range |
|---------|------|---------|-------|
| `auto_poll_enabled` | boolean | `false` | -- |
| `auto_poll_interval` | integer (seconds) | `300` | 120 -- 3600 |

These settings are returned in `GET /api/orgs/settings` and can be updated via `PATCH /api/orgs/settings` -- but only in self-hosted mode. In commercial mode, the auto-poll fields are omitted from the response and rejected if submitted.

The inbox poller worker checks every 30 seconds for orgs that are due for polling. It fetches inbound and sent emails from the Resend API, paginating up to 10 pages (100 emails per page), and stops when it encounters an already-imported email.

---

## Commercial Mode

### Startup Validation

When `STRIPE_KEY` is set, the config loader validates that all required commercial variables are present. The server exits with an error if any are missing:

- `STRIPE_WEBHOOK_SECRET`
- `STRIPE_PRICE_ID`
- `RESEND_SYSTEM_API_KEY`
- `SYSTEM_FROM_ADDRESS`

### Open Signup with Email Verification

In commercial mode, `POST /api/auth/signup` is open to anyone. New users are created with `email_verified = false` and receive a 6-digit verification code by email. They must verify via `POST /api/auth/verify-email` before logging in. Login returns `403 "email_not_verified"` for unverified accounts.

### Plan Enforcement

Feature endpoints (threads, emails, domains, onboarding, aliases, drafts, labels, contacts, attachments) are gated by `RequirePlan` middleware:

- Checks the org's `plan` column in the database
- Allows `pro` and `past_due` plans through
- Allows `cancelled` plans if `plan_expires_at` is still in the future (grace period)
- Returns `402 "subscription_required"` otherwise
- In self-hosted mode, this middleware is a no-op (always passes)

### Billing Endpoints

The following billing endpoints are registered in **both** modes (they sit inside the authenticated route group, not behind a Stripe conditional):

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /api/billing` | User | Returns plan status, expiry, and Stripe subscription details |
| `POST /api/billing/checkout` | Admin | Creates a Stripe Checkout session |
| `POST /api/billing/portal` | Admin | Creates a Stripe Customer Portal session |

In self-hosted mode, these endpoints are still reachable but will fail to interact with Stripe (no API key configured). The `GET /api/billing` response includes `"billing_enabled": true` regardless of mode.

The Stripe webhook endpoint is conditionally registered:

| Endpoint | Condition | Description |
|----------|-----------|-------------|
| `POST /api/webhooks/stripe` | `STRIPE_KEY != ""` | Receives Stripe webhook events |

### Stripe Webhook Setup

1. Go to [Stripe Dashboard -- Developers -- Webhooks](https://dashboard.stripe.com/webhooks)
2. Click **Add endpoint**
3. Set the URL to `{PUBLIC_URL}/api/webhooks/stripe`
4. Subscribe to **all 9** event types:
   - `checkout.session.completed`
   - `customer.subscription.created`
   - `customer.subscription.deleted`
   - `customer.subscription.updated`
   - `customer.subscription.paused`
   - `customer.subscription.resumed`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
   - `invoice.upcoming`
5. Copy the **Signing secret** and set it as `STRIPE_WEBHOOK_SECRET`

**What each event does:**

| Event | Effect |
|-------|--------|
| `checkout.session.completed` | Sets org plan to `pro`, stores subscription ID |
| `customer.subscription.created` | Sets org plan to `pro`, stores subscription ID |
| `customer.subscription.updated` | Maps Stripe status to plan: `active`/`trialing` = `pro`, `past_due` = `past_due`, `unpaid`/`canceled` = `cancelled` |
| `customer.subscription.deleted` | Sets plan to `cancelled` with 7-day grace period |
| `customer.subscription.paused` | Sets plan to `past_due` (keeps access) |
| `customer.subscription.resumed` | Restores plan to `pro`, clears expiry |
| `invoice.payment_succeeded` | Restores plan to `pro`, clears expiry |
| `invoice.payment_failed` | Sets plan to `past_due` with 14-day expiry |
| `invoice.upcoming` | Logged only; no plan state change |

All webhook events are deduplicated via the `stripe_events` table. Duplicate event IDs are skipped with `200 OK`.

A `plan.changed` WebSocket event is broadcast to all org members whenever the plan changes.

> If the webhook is not configured, users can complete Stripe Checkout but their plan will never activate.

---

## Environment Variables

### Always Required

| Variable | Format | Description |
|----------|--------|-------------|
| `ENCRYPTION_KEY` | Base64 string that decodes to exactly 32 bytes | AES-256 key for encrypting stored Resend API keys and webhook secrets |
| `SESSION_SECRET` | String, minimum 32 characters | JWT signing secret (HMAC-SHA256) |

Generate an `ENCRYPTION_KEY`:

```bash
openssl rand -base64 32
```

### Infrastructure

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable` | PostgreSQL connection string. Must start with `postgres://` or `postgresql://` |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string. Must start with `redis://` or `rediss://` |
| `API_PORT` | `8080` | Port for the HTTP server (1--65535) |

### Application

| Variable | Default | Description |
|----------|---------|-------------|
| `PUBLIC_URL` | `http://localhost:8080` | Publicly reachable URL for Resend webhook callbacks. Must be a valid HTTP/HTTPS URL |
| `APP_URL` | `http://localhost:3000` | Frontend URL. Used for CORS origin, cookie domain, and redirect URLs. Must be a valid HTTP/HTTPS URL |
| `EVENT_RETENTION_DAYS` | `90` | How long to keep WebSocket events in the database |

### System Email

| Variable | Required | Description |
|----------|----------|-------------|
| `RESEND_SYSTEM_API_KEY` | Commercial: yes. Self-hosted: no | API key for sending system emails (invites, password resets, verification). In self-hosted mode, this can alternatively be set via the setup wizard UI, which stores it encrypted in the `system_settings` database table. When both the env var and DB value exist, the env var takes precedence |
| `SYSTEM_FROM_ADDRESS` | Commercial: yes. Self-hosted: no | Sender address for system emails (e.g., `noreply@yourdomain.com`). Read in both modes -- in self-hosted mode it serves as a fallback when no value is configured in `system_settings` via the UI |

### Stripe (Commercial Only)

| Variable | Description |
|----------|-------------|
| `STRIPE_KEY` | Stripe secret key. Its presence enables commercial mode |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret |
| `STRIPE_PRICE_ID` | Stripe Price ID for the subscription product |

### Worker Intervals (Optional)

All accept Go duration strings (e.g., `1h`, `5m`, `30s`).

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN_HEARTBEAT_INTERVAL` | `6h` | How often to check Resend domain verification status |
| `TRASH_COLLECTOR_INTERVAL` | `1h` | How often to purge expired trash |
| `TRASH_COLLECTOR_ENABLED` | `false` | Set to `true` to enable the trash collector |
| `EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old WebSocket events |
| `STATUS_RECOVERY_INTERVAL` | `5m` | How often to poll Resend for stale outbound email statuses |
| `STRIPE_EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old Stripe dedup events |
| `GRACE_PERIOD_INTERVAL` | `1h` | How often to check for expired plan grace periods |

### WebSocket (Optional)

| Variable | Default | Description |
|----------|---------|-------------|
| `WS_MAX_CONNECTIONS_PER_USER` | `5` | Maximum concurrent WebSocket connections per user |
| `WS_TOKEN_CHECK_INTERVAL` | `1m` | How often to re-validate WebSocket session tokens |
| `EVENT_CATCHUP_MAX_AGE` | `48h` | Maximum age of events returned by the catchup endpoint |

---

## Feature Comparison

| Behavior | Self-Hosted | Commercial |
|----------|-------------|------------|
| Mode trigger | `STRIPE_KEY` absent | `STRIPE_KEY` present |
| First-run setup | Setup wizard or first-user signup | Standard signup |
| Signup after first user | Disabled (`403`) | Open registration |
| Email verification | Skipped (always `email_verified = true`) | Required (6-digit code) |
| Billing / Stripe | No plan enforcement | Active plan required (`RequirePlan`) |
| Billing endpoints | Registered but non-functional | Fully functional |
| Stripe webhook route | Not registered | Registered |
| System email settings | Owner-configurable via UI (`/api/system/email`) | Managed via env vars only |
| `RESEND_SYSTEM_API_KEY` source | Env var or `system_settings` DB table | Env var (required) |
| `SYSTEM_FROM_ADDRESS` source | Env var (fallback) or `system_settings` DB table | Env var (required) |
| Auto-poll settings | Available in org settings | Hidden from org settings |
| `RequirePlan` middleware | No-op (always passes) | Enforces active subscription |

---

## /api/config Endpoint

`GET /api/config` is a public, unauthenticated endpoint that returns runtime configuration for the frontend. It uses `Cache-Control: no-store` to prevent CDN/proxy caching.

Response:

```json
{
  "api_url": "http://localhost:3000",
  "ws_url": "ws://localhost:3000",
  "commercial": false
}
```

| Field | Type | Description |
|-------|------|-------------|
| `api_url` | string | Same as `APP_URL` |
| `ws_url` | string | `APP_URL` with the scheme converted to `ws://` or `wss://` |
| `commercial` | boolean | `true` when `STRIPE_KEY` is set, `false` otherwise |

The frontend uses `commercial` to conditionally show billing UI, signup forms, and verification flows.

---

## Startup Warnings

The server logs warnings (but does not exit) for these insecure configurations:

| Condition | Warning |
|-----------|---------|
| `PUBLIC_URL` contains `localhost` or `127.0.0.1` | "PUBLIC_URL points to localhost -- webhooks will not work in production" |
| `DATABASE_URL` contains `sslmode=disable` with a non-local host (not `localhost`, `127.0.0.1`, or `postgres`) | "database connection has sslmode=disable with non-local host -- credentials may be transmitted in plain text" |
| `REDIS_URL` has no password with a non-local host (not `localhost`, `127.0.0.1`, or `redis`) | "Redis connection has no password with non-local host -- data may be accessible to anyone on the network" |

These checks are performed during config loading. Local hosts are exempt because these warnings target production deployments where insecure defaults were not changed.
