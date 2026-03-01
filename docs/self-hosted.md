# Self-Hosted vs Commercial Mode

Inboxes runs in one of two modes based on a single environment variable.

## How Mode Is Determined

- **`STRIPE_KEY` is set** → Commercial mode (billing enabled, email verification required)
- **`STRIPE_KEY` is unset or empty** → Self-hosted mode (no billing, no email verification)

There is no config file or feature flag. The `STRIPE_KEY` presence is checked at startup and in route registration.

## Mode Comparison

| Behavior | Self-Hosted | Commercial |
|----------|-------------|------------|
| First-run setup | Setup wizard | Standard signup |
| Email verification | Skipped | Required |
| Billing / Stripe | Disabled | Enabled (plan enforcement) |
| Signup | Disabled (invite-only after setup) | Open registration |
| System email settings | Owner-configurable via UI | Managed via env var |
| `RESEND_SYSTEM_API_KEY` | Optional (can set via UI) | Required at startup |
| Payment wall | None | Active plan required for features |

## Self-Hosted Mode

### Setup Wizard

On first launch with no users in the database, the setup wizard is available:

1. **Check status** — `GET /api/setup/status` returns `needs_setup: true`
2. **Validate Resend key** — `POST /api/setup/validate-key` tests a Resend API key and returns available domains
3. **Create admin** — `POST /api/setup` creates the first org + admin user with `is_owner = true`

The setup endpoint accepts optional system email configuration:
- `system_resend_key` — encrypted and stored in `system_settings` for sending invite/reset emails
- `system_from_address` — the "from" address for system emails
- `system_from_name` — the display name for system emails

After the first user is created, the setup endpoints are locked (return `403`).

### System Email Configuration

The instance owner (`is_owner = true`) can manage system email settings from the UI:

- `GET /api/system/email` — view current from-address
- `PATCH /api/system/email` — update from-address, optionally send a test email

These endpoints are protected by `RequireOwner` middleware and only registered when `STRIPE_KEY` is unset.

### No Email Verification

In self-hosted mode, users created via the setup wizard or invite flow have `email_verified = true` by default. There is no verification code step.

### Invite-Only

After the initial admin is created, new users can only be added via the invite flow (`POST /api/users/invite`). The signup endpoint still exists but is only functional in commercial mode.

## Commercial Mode

### Startup Validation

When `STRIPE_KEY` is set, the server validates at startup that all required Stripe variables are present:

- `STRIPE_WEBHOOK_SECRET` — required
- `STRIPE_PRICE_ID` — required
- `RESEND_SYSTEM_API_KEY` — required (commercial mode needs to send verification/reset emails)

If any are missing, the server exits with an error message.

### Email Verification

New signups must verify their email address before accessing the app. A 6-digit code is sent to their email via the system Resend API key.

### Plan Enforcement

Feature endpoints (threads, emails, domains, etc.) are gated by `RequirePlan` middleware:
- Checks if the org has an active Stripe subscription
- Returns `402` if the plan is inactive (triggers the payment wall on the frontend)
- In self-hosted mode, this middleware is a no-op (always passes)

### Billing Endpoints

- `GET /api/billing` — returns plan status, subscription info
- `POST /api/billing/checkout` — creates a Stripe Checkout session
- `POST /api/billing/portal` — creates a Stripe Customer Portal link
- `POST /api/webhooks/stripe` — receives Stripe webhook events (subscription changes)

The `plan.changed` WebSocket event is broadcast to all org members when a subscription changes.

### Stripe Webhook

The Stripe webhook endpoint is only registered when `STRIPE_KEY` is set.

**Setup (manual step in the Stripe Dashboard):**

1. Go to the [Stripe Dashboard → Developers → Webhooks](https://dashboard.stripe.com/webhooks)
2. Click **Add endpoint**
3. Set the endpoint URL to: `{PUBLIC_URL}/api/webhooks/stripe` (e.g. `https://mail.yourdomain.com/api/webhooks/stripe`)
4. Subscribe to the following events:
   - `checkout.session.completed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
5. Copy the **Signing secret** and set it as `STRIPE_WEBHOOK_SECRET` in your env

> **Important:** If this webhook is not configured, users can complete Stripe Checkout but their plan will never activate (the app won't receive the subscription events).

## Environment Variables by Mode

### Always Required

| Variable | Description |
|----------|-------------|
| `ENCRYPTION_KEY` | AES-256 key for encrypting stored Resend API keys |
| `SESSION_SECRET` | JWT signing secret |

### Self-Hosted Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `RESEND_SYSTEM_API_KEY` | — | Can be set via setup wizard UI instead |
| `PUBLIC_URL` | `http://localhost:8080` | Resend webhook callback URL |
| `APP_URL` | `http://localhost:3000` | Cookie domain, CORS origin |
| `EVENT_RETENTION_DAYS` | `90` | How long to keep events in the database |

### Commercial Required

| Variable | Description |
|----------|-------------|
| `STRIPE_KEY` | Stripe secret key (enables commercial mode) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret |
| `STRIPE_PRICE_ID` | Stripe Price ID for the subscription product |
| `RESEND_SYSTEM_API_KEY` | Must be set as env var (no UI fallback) |
| `SYSTEM_FROM_ADDRESS` | Sender address for verification/reset/invite emails (e.g. `noreply@yourdomain.com`) |
