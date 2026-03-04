# Deployment Guide

## Quick Start (Docker Compose)

```bash
git clone https://github.com/headswim/inboxes.git && cd inboxes
cp .env.example .env
# Edit .env — at minimum set: DOMAIN, ENCRYPTION_KEY, SESSION_SECRET, PUBLIC_URL, POSTGRES_PASSWORD
docker compose up -d
```

The stack includes four services: `postgres`, `redis`, `backend`, and `frontend`. A fifth service (`caddy`) is present but commented out — see [Reverse Proxy Options](#reverse-proxy-options) below.

---

## Environment Variables

### Required (no defaults)

| Variable | Example | Notes |
|----------|---------|-------|
| `DOMAIN` | `mail.yourdomain.com` | Interpolated in docker-compose.yml to build `APP_URL`, `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_WS_URL` |
| `PUBLIC_URL` | `https://mail.yourdomain.com` | Passed to the backend in docker-compose.yml. Used as the Resend webhook callback base URL. Compose will fail to start if unset (`${PUBLIC_URL:?...}` syntax) |
| `ENCRYPTION_KEY` | `openssl rand -base64 32` | Must be valid base64 decoding to exactly 32 bytes. Encrypts stored Resend API keys. **If lost, stored org API keys are unrecoverable** |
| `SESSION_SECRET` | `openssl rand -hex 32` | Must be at least 16 characters. Backend refuses to start without it |
| `POSTGRES_PASSWORD` | `openssl rand -hex 16` | **No default.** Compose will fail to start if unset (`${POSTGRES_PASSWORD:?...}` syntax) |

### Required (have defaults — override in production)

| Variable | Default | Notes |
|----------|---------|-------|
| `POSTGRES_USER` | `inboxes` | |
| `POSTGRES_DB` | `inboxes` | |

### Required in Commercial Mode

When `STRIPE_KEY` is set (non-empty), the backend requires all of the following or it will refuse to start:

| Variable | Notes |
|----------|-------|
| `STRIPE_KEY` | Enables billing, email verification, rate limiting |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret |
| `STRIPE_PRICE_ID` | Stripe price ID for the subscription plan |
| `RESEND_SYSTEM_API_KEY` | Resend API key used for system emails (invites, verification, password reset) |
| `SYSTEM_FROM_ADDRESS` | Sender address for system emails, e.g. `noreply@yourdomain.com` |

### Optional

| Variable | Default | Notes |
|----------|---------|-------|
| `RESEND_SYSTEM_API_KEY` | *(empty)* | In self-hosted mode (no `STRIPE_KEY`), only needed if you want system emails. In commercial mode, required (see above) |
| `SYSTEM_FROM_ADDRESS` | *(empty)* | Sender address for system emails |
| `API_PORT` | `8080` | Backend listen port inside the container |
| `TRASH_COLLECTOR_ENABLED` | `false` | Enable automatic purging of expired trash items |
| `EVENT_RETENTION_DAYS` | `90` | Days to keep WebSocket events before pruning |

### Worker Intervals (all optional, Go duration strings)

| Variable | Default | Notes |
|----------|---------|-------|
| `DOMAIN_HEARTBEAT_INTERVAL` | `6h` | How often to check Resend domain status |
| `TRASH_COLLECTOR_INTERVAL` | `1h` | How often to purge expired trash |
| `EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old events |
| `STATUS_RECOVERY_INTERVAL` | `5m` | How often to poll Resend for stale outbound email statuses |
| `GRACE_PERIOD_INTERVAL` | `1h` | How often to check for expired grace periods |
| `STRIPE_EVENT_PRUNER_INTERVAL` | `6h` | How often to prune old Stripe dedup events |

### WebSocket Settings (all optional)

| Variable | Default | Notes |
|----------|---------|-------|
| `WS_MAX_CONNECTIONS_PER_USER` | `5` | Max concurrent WebSocket connections per user |
| `WS_TOKEN_CHECK_INTERVAL` | `1m` | How often to revalidate WebSocket auth tokens |
| `EVENT_CATCHUP_MAX_AGE` | `48h` | Max age for event catchup queries on reconnect |

---

## How Env Vars Flow

```
.env file (or Coolify env panel)
  |
  v
docker-compose.yml interpolation
  |
  |-- postgres container
  |     POSTGRES_USER    = ${POSTGRES_USER:-inboxes}
  |     POSTGRES_PASSWORD = ${POSTGRES_PASSWORD:?required}
  |     POSTGRES_DB      = ${POSTGRES_DB:-inboxes}
  |
  |-- backend container
  |     DATABASE_URL     = postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable
  |     REDIS_URL        = redis://redis:6379
  |     ENCRYPTION_KEY   = ${ENCRYPTION_KEY}
  |     SESSION_SECRET   = ${SESSION_SECRET}
  |     APP_URL          = https://${DOMAIN}
  |     PUBLIC_URL       = ${PUBLIC_URL:?required}
  |     API_PORT         = 8080
  |     RESEND_SYSTEM_API_KEY, STRIPE_*, SYSTEM_FROM_ADDRESS, worker intervals...
  |
  |-- frontend container
        NEXT_PUBLIC_API_URL = https://${DOMAIN}
        NEXT_PUBLIC_WS_URL  = wss://${DOMAIN}
```

`PUBLIC_URL` IS in docker-compose.yml and is a hard requirement. Compose will refuse to start if it is unset.

---

## APP_URL vs PUBLIC_URL

| Variable | Used for |
|----------|----------|
| `APP_URL` | Cookie domain, password reset links, Stripe redirect URLs, WebSocket URL derivation, `/api/config` response |
| `PUBLIC_URL` | Resend webhook callback URL registered during onboarding (`PUBLIC_URL + /api/webhooks/resend/{orgId}`) |

In most setups both resolve to `https://mail.yourdomain.com`. They exist as separate variables because in some architectures the public-facing URL (for webhooks from Resend) differs from the app URL (for the browser).

---

## Docker Resource Limits

All services in docker-compose.yml have `deploy.resources.limits` set:

| Service | Memory | CPU |
|---------|--------|-----|
| postgres | 512M | 0.5 |
| redis | 128M | 0.25 |
| backend | 256M | 0.5 |
| frontend | 256M | 0.5 |

Adjust these in docker-compose.yml if your workload requires more headroom.

---

## Background Workers

The backend runs several background workers as goroutines (no separate process needed):

| Worker | What it does |
|--------|-------------|
| **sync-worker** | Processes sync jobs from the Redis queue |
| **sync-stale-recovery** | Recovers stale/stuck sync jobs |
| **email-worker** | Processes email jobs (fetch, send, fetch_sent) from the Redis queue |
| **email-stale-recovery** | Recovers stale/stuck email jobs |
| **inbox-poller** | Auto-syncs inbound and sent emails by polling the Resend API. Designed for environments where webhooks are unreachable (e.g. behind NAT, no public URL). Checks every 30s for orgs with `auto_poll_enabled = true` that are due for polling. Respects per-org rate limits |
| **trash-collector** | Purges expired trash items (disabled by default, enable with `TRASH_COLLECTOR_ENABLED=true`) |
| **domain-heartbeat** | Periodically checks Resend domain verification status |
| **event-pruner** | Removes WebSocket events older than `EVENT_RETENTION_DAYS` |
| **status-recovery** | Polls Resend for stale outbound email delivery statuses |
| **grace-period** | Transitions expired cancelled/past_due plans to free tier |
| **stripe-event-pruner** | Removes Stripe dedup events older than 7 days |

---

## Health Checks

Both containers have Docker `HEALTHCHECK` directives:

| Service | Endpoint | Interval | Timeout | Retries |
|---------|----------|----------|---------|---------|
| backend | `curl -sf http://localhost:8080/api/health` | 30s | 5s | 3 |
| frontend | `curl -sf http://localhost:3000/` | 30s | 5s | 3 |
| postgres | `pg_isready -U inboxes -d inboxes` | 5s | 3s | 5 |
| redis | `redis-cli ping` | 5s | 3s | 5 |

The backend depends on both `postgres` and `redis` being healthy before starting. The frontend depends on the backend.

---

## Reverse Proxy Options

Neither the backend nor the frontend exposes host ports in docker-compose.yml. You need a reverse proxy to route external traffic. Three options:

### Option A: Uncomment the Caddy Service

A Caddy service is included in docker-compose.yml but commented out. To use it:

1. Uncomment the `caddy:` block in docker-compose.yml.
2. Add `caddy_data` and `caddy_config` to the `volumes:` section at the bottom:

```yaml
volumes:
  postgres_data:
  redis_data:
  caddy_data:
  caddy_config:
```

3. Ensure the `Caddyfile` in the repo root is correct. It routes by path:

```
{$DOMAIN} {
    handle /api/* {
        reverse_proxy backend:8080
    }
    handle /api/ws {
        reverse_proxy backend:8080
    }
    handle {
        reverse_proxy frontend:3000
    }
}
```

Caddy auto-provisions TLS via Let's Encrypt. Ports 80 and 443 must be open on the host.

### Option B: External Nginx / Traefik / HAProxy

If you already have a reverse proxy on the host, point it at the containers. The key routing rules:

- `/api/*` (including `/api/ws`) -> backend on port 8080
- Everything else -> frontend on port 3000
- WebSocket upgrade must be supported for `/api/ws`

You will need to expose ports on the backend and frontend services (add `ports:` sections) or attach them to the same Docker network as your proxy.

### Option C: Coolify

Coolify handles TLS and reverse proxying automatically. See the [Coolify Setup](#coolify-setup) section below.

---

## Coolify Setup

### Prerequisites

- Running Coolify instance
- GitHub repo access
- A domain with DNS pointing to your Coolify server
- Resend account with at least one verified domain

### 1. Create Resource

- **Type:** Docker Compose
- **Source:** GitHub repository
- **Base Directory:** `/`
- **Docker Compose Location:** `/docker-compose.yml`

In Advanced Settings, **deselect** "Strip Prefixes".

### 2. Domain Configuration

| Service | Domain |
|---------|--------|
| backend | *(leave blank -- proxied through frontend)* |
| frontend | `mail.yourdomain.com` |

Point your DNS at the Coolify server. Coolify handles TLS automatically.

### 3. Environment Variables

Set all variables in the Coolify **Environment Variables** panel. The minimum set:

```
DOMAIN=mail.yourdomain.com
PUBLIC_URL=https://mail.yourdomain.com
SESSION_SECRET=<openssl rand -hex 32>
ENCRYPTION_KEY=<openssl rand -base64 32>
POSTGRES_PASSWORD=<openssl rand -hex 16>
```

Leave the Caddy service commented out -- Coolify provides its own reverse proxy.

### 4. Deploy

1. Set env vars in Coolify
2. Click Deploy
3. Coolify builds both Docker images, starts postgres + redis, then backend + frontend
4. First deploy takes longer (building Go binary + Next.js standalone output)

### 5. Post-Deploy Setup

1. Open `https://mail.yourdomain.com`
2. Sign up -- creates your org + admin user (no email verification in self-hosted mode)
3. Complete onboarding -- enter your Resend API key, which is encrypted and stored per-org. This automatically registers the Resend webhook at `PUBLIC_URL/api/webhooks/resend/{orgId}`
4. Onboarding imports emails automatically after selecting domains and aliases
5. Verify Resend domain DNS (MX, SPF, DKIM) in the Resend dashboard

### 6. Updating

Push to the GitHub repo, then either let Coolify auto-deploy (if configured) or manually click Deploy. Postgres and Redis volumes persist across deploys.

### 7. Wiping and Starting Fresh

1. Stop the resource in Coolify
2. Delete the postgres volume (destroys all data)
3. Redeploy
4. Redo post-deploy setup

`ENCRYPTION_KEY` and `SESSION_SECRET` can stay the same or be regenerated. Regenerating `ENCRYPTION_KEY` is safe on a fresh database since there are no encrypted keys to decrypt.

---

## Local Development

### Prerequisites

macOS: PostgreSQL 13+, Go 1.23+, Node 18+, Redis 6+.

### Automated Setup

```bash
# From inside the repo:
./scripts/setup.sh

# Or via curl (clones the repo into ~/inboxes):
curl -fsSL https://raw.githubusercontent.com/headswim/inboxes/main/scripts/setup.sh | bash
```

`setup.sh` installs dependencies via Homebrew, starts Postgres and Redis, creates the database, generates `.env` with random secrets, and runs `npm install` for the frontend.

### Start Development Servers

```bash
./scripts/dev.sh
```

This starts the backend on `:8080` and frontend on `:3000`. The backend auto-runs database migrations on startup via goose. Press Ctrl+C to stop both.

### .env Loading Order

The backend loads `.env` files in this order (first match wins per variable):

1. `../.env` (parent directory -- where `setup.sh` creates it, i.e. the repo root)
2. `.env` (current working directory)

This means when running `go run ./cmd/api` from `backend/`, it picks up the root-level `.env` automatically.

### Frontend Build

The frontend uses Next.js `standalone` output mode. In development, `npm run dev` runs the Next.js dev server. In Docker, the production build copies `.next/standalone` and `.next/static` into a minimal Node.js image that runs `node server.js`.

The frontend's `next.config.js` includes a rewrite rule that proxies `/api/*` requests to the backend. In Docker, this defaults to `http://backend:8080`. In local dev, set `BACKEND_URL=http://localhost:8080` (or leave it -- the default `NEXT_PUBLIC_API_URL` makes the browser call the backend directly, bypassing the rewrite).

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Compose fails with `POSTGRES_PASSWORD` error | Missing env var | `POSTGRES_PASSWORD` has no default and is required. Set it in `.env` |
| Compose fails with `PUBLIC_URL` error | Missing env var | `PUBLIC_URL` has no default in docker-compose.yml and is required. Set it in `.env` |
| "ENCRYPTION_KEY is required" on startup | Missing env var | Set `ENCRYPTION_KEY` (must be `openssl rand -base64 32`) |
| "ENCRYPTION_KEY must decode to 32 bytes" | Wrong format | Must be base64-encoded 32 bytes, not hex or plain text |
| "SESSION_SECRET must be at least 16 characters" | Too short | Use `openssl rand -hex 32` for a 64-character hex string |
| Webhook emails not arriving | `PUBLIC_URL` wrong or unreachable from internet | Verify `PUBLIC_URL` is externally reachable. Check Resend dashboard for webhook delivery status |
| Login doesn't persist across refreshes | `APP_URL` doesn't match browser URL | Cookie domain is derived from `APP_URL`. Ensure it matches what the browser sees |
| WebSocket won't connect | `DOMAIN` mismatch or mixed http/https | Frontend derives `wss://${DOMAIN}` for WebSocket. Ensure `DOMAIN` matches the actual domain and TLS is working |
| Emails not arriving in Resend | DNS not configured | Check MX, SPF, DKIM records in Resend dashboard for each receiving domain |
| "PUBLIC_URL points to localhost" warning | Local dev setup | Expected in local dev. For production, set `PUBLIC_URL` to the externally reachable URL. Use ngrok or Cloudflare Tunnel during local testing if you need webhooks |
| Backend OOM killed | Resource limit too low | Increase `memory` under `backend.deploy.resources.limits` in docker-compose.yml |

---

## Quick Reference: Minimum Env Vars

```env
DOMAIN=mail.yourdomain.com
PUBLIC_URL=https://mail.yourdomain.com
SESSION_SECRET=<openssl rand -hex 32>
ENCRYPTION_KEY=<openssl rand -base64 32>
POSTGRES_PASSWORD=<openssl rand -hex 16>
```

Everything else has working defaults or is only needed for commercial mode.
