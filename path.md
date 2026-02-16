# Path: Open Source + Hosted Commercial Version

## The Model

Same codebase. Fully open source (MIT). Two ways to use it:

1. **Self-host**: Clone repo, `docker-compose up`, connect Resend, done
2. **Hosted**: Sign up at our site, connect Resend, we handle the infra — paid

Anyone can fork and compete. We compete on being the original, the brand, and the convenience.

---

## What We Have Today

- Go backend (Chi, pgx, Redis) + Next.js frontend
- Multi-tenant data model (org_id on everything)
- Auth (signup → org creation → JWT cookies)
- Onboarding flow (connect Resend → select domains → sync → go)
- Full email client (threads, compose, reply, drafts, search, folders)
- Dockerfiles for backend and frontend
- Coolify deployment for our own staging/prod

## What's Left

### 1. Self-Host Packaging (the open source release)

**Complete docker-compose.yml**
Current docker-compose only has the backend service. Need the full stack:
- postgres
- redis
- backend (Go API)
- frontend (Next.js)
- caddy (reverse proxy, automatic HTTPS via Let's Encrypt)
- Single `docker-compose up -d` spins everything up

**Environment documentation**
Document all env vars in README and `.env.example`:
- `DOMAIN` — the domain pointing to this server (e.g. `mail.example.com`). Caddy uses this for auto-SSL
- `DATABASE_URL` — postgres connection string (has a default in compose)
- `REDIS_URL` — redis connection (has a default in compose)
- `JWT_SECRET` — any random string, user generates it
- `APP_URL` — public URL of the app (e.g. `https://mail.example.com`). Used for CORS, cookies, and **Resend webhook registration**

That's it. Resend API key comes through the in-app onboarding, not env vars.

**Webhooks & HTTPS (important)**

Resend delivers inbound emails and status updates via webhooks. The backend must be reachable
from the public internet over HTTPS for this to work.

- **Self-hosters on a VPS (the normal case)**: Point a domain at the server, set `DOMAIN` and
  `APP_URL` in `.env`. Caddy in the docker-compose handles HTTPS automatically via Let's Encrypt.
  The onboarding flow registers `{APP_URL}/api/webhooks/resend/{orgId}` with Resend. Done.

- **Self-hosters running locally (dev/testing)**: Resend can't reach localhost. Options:
  - `ngrok http 8080` → set `APP_URL` to the ngrok URL
  - Cloudflare Tunnel → same idea
  - Document this as a "local dev" note, not the primary path

- **Hosted version (us)**: Not an issue. We have a public domain on Coolify.

The Caddy container is what makes self-hosting painless — no manual cert setup, no nginx config.
Just set your domain and go.

**README.md**
- What this is (one paragraph)
- Quick start: point domain at server, set `DOMAIN` in `.env`, `docker-compose up -d`, sign up, connect Resend
- Requirements: a VPS with a public IP, a domain, a Resend account
- Configuration reference (env vars)
- Note on local dev: use ngrok/cloudflare tunnel for webhooks
- Link to GitHub issues for support

**License**
- MIT license file in repo root
- Done

**Estimated work: ~1 day** (mostly writing docs, fixing up docker-compose, adding Caddy)

---

### 2. Hosted Version (our commercial offering)

**Stripe billing integration**

New backend work:
- `POST /api/billing/checkout` — create Stripe checkout session
- `POST /api/billing/portal` — create Stripe customer portal session (manage/cancel)
- `POST /api/webhooks/stripe` — handle subscription events
- `plans` table or just hardcoded tiers (keep it simple to start)
- Middleware that checks subscription status on protected routes
- Store `stripe_customer_id` and `plan` on the `orgs` table (new migration)

New frontend work:
- Billing settings page (current plan, manage subscription button)
- Upgrade prompt when hitting limits (if we have limits)
- Maybe a banner for trial/expired states

Plan structure (keep it simple):
- Free tier: 1 domain, 1 user (or just a trial period)
- Paid tier: unlimited domains, unlimited users, $X/mo
- That's it. Don't overthink tiers at launch.

**Conditional behavior (no feature flags needed)**
- If `STRIPE_KEY` env var is set → billing is active, signup requires payment
- If `STRIPE_KEY` is not set → billing doesn't exist, everything is free
- Self-hosters never set it. We always set it. Same codebase, zero branching.

**Signup hardening for public-facing**
- Rate limiting on auth endpoints (can use Redis, already have it)
- Email verification on signup (send a code via Resend)
- Maybe CAPTCHA on signup if abuse becomes a problem (not day 1)

**Estimated work: ~3-5 days** (Stripe is most of it)

---

### 3. Landing Page (separate project, not in this repo)

A simple static/marketing site:
- Hero: what this is, screenshot
- Features: quick bullet points
- Pricing: the plan(s)
- "Sign up" button → links to hosted app signup
- "Self-host" button → links to GitHub repo
- Footer: ToS, privacy policy

Can be a one-page Next.js site, Framer, Astro, whatever. It's marketing, not engineering. Deploy it anywhere.

**Estimated work: ~1 day** (if using a template/simple framework)

---

### 4. Legal (boring but necessary for hosted)

- Terms of Service
- Privacy Policy
- Can use a generator or template to start, lawyer-review later

---

## Execution Order

1. **Self-host packaging** — docker-compose, README, license, `.env.example`
   Ship it. Put it on GitHub. Get feedback.

2. **Stripe billing** — plans table migration, checkout/portal/webhook endpoints, billing UI
   This unblocks the hosted version.

3. **Landing page** — separate repo, deploy to custom domain
   Points to hosted app + GitHub.

4. **Launch hosted** — deploy the app with `STRIPE_KEY` set, point landing page at it

5. **Legal** — ToS/privacy before or alongside hosted launch

---

## Architecture (no changes needed)

```
Self-hoster                          Hosted (us)
─────────────                        ───────────
Their VPS                            Our Coolify server
├── caddy (auto-HTTPS)               ├── (Coolify handles HTTPS)
├── postgres                         ├── postgres
├── redis                            ├── redis
├── backend (Go)                     ├── backend (Go) + STRIPE_KEY set
└── frontend (Next.js)               └── frontend (Next.js)

No STRIPE_KEY → no billing           STRIPE_KEY set → billing active
Single org (their team)              Multi-org (each customer = org)
APP_URL = their domain               APP_URL = our domain
Resend webhooks → their server       Resend webhooks → our server
```

Same code. Same repo. Same Docker images. Just different env vars.

### Webhook Flow (same for both)

```
User connects Resend API key (onboarding)
  → Backend registers webhook: {APP_URL}/api/webhooks/resend/{orgId}
  → Resend sends inbound emails + delivery events to that URL
  → Backend processes them into threads/emails
```

The only requirement: `APP_URL` must be a publicly reachable HTTPS URL.
Self-hosters get this from Caddy + their domain. We get it from Coolify.

**Webhook resilience**: Resend retries failed deliveries with backoff (~24-72h).
Brief downtime is fine. Extended downtime: re-sync from settings catches missed emails.
Already built — sync exists in settings, webhook processing should be made idempotent
(check `resend_email_id` before insert to skip duplicates).

---

## Future (ideas, not commitments)

### Multiple Email Providers

Currently hardcoded to Resend. Could support others behind a provider interface:
- AWS SES
- Postmark
- Mailgun
- SendGrid
- SMTP (raw)

Architecture: Go interface for `EmailProvider` with methods like `Send()`, `ListEmails()`,
`RegisterWebhook()`. Each provider implements it. Config picks which one.
Resend stays the default. Others are community-contributed or added over time.

This would massively expand who can self-host — not everyone wants/uses Resend.

### Self-Hoster Config Options

Things self-hosters might want to control (via env vars or a settings file):
- Auto-sync interval (or disable)
- Email retention period (auto-delete after X days)
- Max users per org
- Disable public signup (single-admin mode)
- Custom branding (logo, app name)
- SMTP relay as fallback

### Hosted Version Features

Things that could differentiate the paid hosted version:
- Priority support
- Uptime SLA
- Managed backups & exports
- Usage analytics dashboard
- Custom domain for the app itself (white-label)

### General Feature Ideas

- Filters / rules (auto-archive, auto-label, auto-forward)
- Labels / tags (beyond folders)
- Scheduled send
- Undo send (delay sending by X seconds)
- Email templates / signatures
- Shared inboxes (multiple users see same inbox)
- API access (let users build on top)
- Mobile app (or PWA)
- Calendar integration
- Contact management (beyond autocomplete)
