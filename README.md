# Inboxes

A self-hostable email client powered by [Resend](https://resend.com). Connect your domain, import existing emails, and manage your inbox with a clean UI.

## Features

- **Multi-domain inbox** - manage multiple domains from a single interface, reorder and hide domains in the sidebar
- **Threads** - grouped conversations with inbox, sent, archive, trash, spam, and starred views
- **Custom labels** - create labels, drag threads between folders, bulk actions
- **Compose & drafts** - floating compose window, draft auto-save, reply/forward
- **Attachments** - upload and download file attachments
- **Aliases** - create aliases, assign users, control who can send from which address
- **Team management** - invite users, assign roles (admin/member), deactivate with data reassignment
- **Email sync** - import historical emails from Resend with background job processing
- **Real-time updates** - WebSocket push with alias-based event filtering and cross-tab sync
- **Contact autocomplete** - suggestions from address history
- **Keyboard shortcuts** - Gmail-style shortcuts for power users
- **Dark mode** - system-aware with manual toggle
- **Self-hosted or commercial** - run for free on your own server, or enable Stripe billing for SaaS mode
- **Domain monitoring** - automatic heartbeat checks against Resend, self-healing reconnection

## Security

- **Encryption at rest** - Resend API keys encrypted with AES-256-GCM
- **JWT authentication** - tokens in httpOnly cookies, token blacklist for revocation
- **Rate limiting** - per-IP and per-identity rate limits on all public endpoints
- **Request validation** - body size limits, content-type validation, input sanitization
- **Security headers** - X-Content-Type-Options, X-Frame-Options, Referrer-Policy, CSP
- **Webhook verification** - Resend and Stripe webhook signatures verified before processing

## Install (macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/headswim/inboxes/main/scripts/setup.sh | bash
```

This installs everything you need (Homebrew, Go, Node, PostgreSQL, Redis), creates the database, and generates your config. You'll be prompted to choose between a quick install (default credentials) or a custom install (recommended - set your own database credentials and webhook URL). Then:

```bash
cd ~/inboxes
./scripts/dev.sh
```

Open **http://localhost:3000** to get started. You'll sign up, enter your Resend API key, select your domains and aliases, and import your existing emails - all in one guided flow.

> **Note:** Inbound email (receiving via webhook) won't work on localhost because Resend can't reach your machine. Outbound sending and email sync work fine. For inbound, use [ngrok](https://ngrok.com) or a similar tunnel and set `PUBLIC_URL` in `.env` to the tunnel URL.

### Already have the code?

```bash
./scripts/setup.sh   # one-time - installs deps, creates DB, generates .env
./scripts/dev.sh     # starts everything - Ctrl+C to stop
```

## Deploy (VPS with Docker)

For production on a VPS with a domain name:

```bash
git clone https://github.com/headswim/inboxes.git
cd inboxes

cp .env.example .env
# Edit .env - set DOMAIN, generate ENCRYPTION_KEY and SESSION_SECRET:
#   openssl rand -base64 32   (ENCRYPTION_KEY)
#   openssl rand -hex 32      (SESSION_SECRET)

docker compose up -d
```

This starts the backend, frontend, PostgreSQL, and Redis. You'll need a reverse proxy (nginx, Traefik, Caddy, or a platform like Coolify) in front for HTTPS.

See [Deployment Guide](docs/deployment.md) for Coolify setup, reverse proxy options, env var details, and troubleshooting.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOMAIN` | Production | - | Your domain (e.g. `mail.example.com`) |
| `ENCRYPTION_KEY` | Yes | - | `openssl rand -base64 32` |
| `SESSION_SECRET` | Yes | - | `openssl rand -hex 32` |
| `DATABASE_URL` | No | `postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable` | PostgreSQL connection |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection |
| `PUBLIC_URL` | No | `http://localhost:8080` | Backend URL (for Resend webhook registration) |
| `APP_URL` | No | `http://localhost:3000` | Frontend URL (cookie domain, CORS) |
| `RESEND_SYSTEM_API_KEY` | No | - | For sending invite/reset emails |
| `API_PORT` | No | `8080` | Backend port |
| `EVENT_RETENTION_DAYS` | No | `90` | How long to keep events for WS catch-up |
| `STRIPE_KEY` | No | - | Enables commercial mode (billing + email verification) |
| `STRIPE_WEBHOOK_SECRET` | If Stripe | - | Required when `STRIPE_KEY` is set |
| `STRIPE_PRICE_ID` | If Stripe | - | Required when `STRIPE_KEY` is set |

## Architecture

- **Backend:** Go + Chi router + pgx + Redis
- **Frontend:** Next.js 15, React 19, Tailwind CSS, shadcn/ui
- **Database:** PostgreSQL 16 (auto-managed migrations via goose)
- **Cache/PubSub:** Redis 7
- **Background Workers:** 9 workers - email sync, send queue, trash collector, domain heartbeat, event pruner, grace period, stripe event pruner, status recovery, inbox poller

## Self-Hosted vs Commercial

Inboxes runs in self-hosted mode by default. Setting `STRIPE_KEY` enables commercial mode with billing, email verification, and plan enforcement. See [Self-Hosted Guide](docs/self-hosted.md) for details.

## Documentation

- [API Reference](docs/api.md) - all endpoints, auth, rate limits
- [Self-Hosted Guide](docs/self-hosted.md) - mode differences, setup wizard
- [Operations Guide](docs/operations.md) - workers, backup, monitoring
- [Deployment Guide](docs/deployment.md) - Docker Compose, Coolify, env vars

## License

[MIT](LICENSE)
