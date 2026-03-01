# Deployment Guide

## Generic Docker Compose

If you're deploying outside of Coolify (any VPS, bare metal, or Docker host):

```bash
git clone https://github.com/headswim/inboxes.git && cd inboxes
cp .env.example .env
# Edit .env — set DOMAIN, ENCRYPTION_KEY, SESSION_SECRET, PUBLIC_URL
docker compose up -d
```

**Important:** Add `PUBLIC_URL` to the backend service in `docker-compose.yml` since it's not included by default (Coolify injects it automatically, but other platforms don't):

```yaml
backend:
  environment:
    - PUBLIC_URL=${PUBLIC_URL}
```

Without `PUBLIC_URL`, Resend webhooks will register as `http://localhost:8080` and inbound email won't work.

The `docker-compose.yml` includes a commented-out Caddy service that handles HTTPS via Let's Encrypt — uncomment it if you need a reverse proxy. If you're deploying behind Coolify, nginx, or another reverse proxy that already terminates TLS, leave it commented out. See [Operations Guide](operations.md) for backup, monitoring, and worker details.

---

## Coolify Deployment

### Prerequisites

- Coolify instance running
- GitHub repo access (SSH key for private repo, or public repo works directly)
- A domain with DNS access (e.g. `mail.yourdomain.com`)
- Resend account with at least one verified domain

---

### 1. Create Resource in Coolify

- **Type:** Docker Compose
- **Source:** GitHub repository (SSH key auth for private repos)
- **Base Directory:** `/`
- **Docker Compose Location:** `/docker-compose.yml` (not `.yaml`)

### Advanced Settings

- **Deselect** "Strip Prefixes"

---

### 2. Domain Configuration

### Coolify Service Domains

| Service | Domain |
|---------|--------|
| backend | *(leave blank — proxied through frontend)* |
| frontend | `mail.yourdomain.com` |

### DNS Record

```
Type    Name    Value
CNAME   mail    yourdomain.com.
```

Point at your Coolify server IP/hostname. Coolify handles TLS automatically.

---

### 3. Environment Variables

Set these in the Coolify **Environment Variables** section for the resource.

### Required

| Variable | Example | Notes |
|----------|---------|-------|
| `DOMAIN` | `mail.yourdomain.com` | Used by docker-compose for APP_URL, WS_URL |
| `SESSION_SECRET` | `openssl rand -hex 32` | App won't start without this |
| `ENCRYPTION_KEY` | `openssl rand -base64 32` | Encrypts stored Resend API keys. **If you lose this, stored org API keys are unrecoverable** |
| `PUBLIC_URL` | `https://mail.yourdomain.com` | **Critical.** Used as the Resend webhook callback URL. docker-compose does NOT pass this to the backend — Coolify injects it directly. Without it, webhooks register as `http://localhost:8080` |

### Required (with defaults)

| Variable | Default | Notes |
|----------|---------|-------|
| `POSTGRES_USER` | `inboxes` | |
| `POSTGRES_PASSWORD` | `inboxes` | Change in production |
| `POSTGRES_DB` | `inboxes` | |

### Optional

| Variable | Default | Notes |
|----------|---------|-------|
| `RESEND_SYSTEM_API_KEY` | *(empty)* | Only needed for system emails: invite, email verification, password reset. Not needed if `STRIPE_KEY` is unset (verification is skipped in self-hosted mode) |
| `STRIPE_KEY` | *(empty)* | Enables commercial/hosted mode: billing tab, email verification, rate limiting |
| `STRIPE_WEBHOOK_SECRET` | *(empty)* | Required if STRIPE_KEY is set |
| `STRIPE_PRICE_ID` | *(empty)* | Required if STRIPE_KEY is set |

### How env vars flow

```
Coolify env vars
  │
  ├─► docker-compose.yml interpolation (${DOMAIN}, ${POSTGRES_*}, etc.)
  │     ├─► postgres container: POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
  │     ├─► backend container: DATABASE_URL, REDIS_URL, ENCRYPTION_KEY, SESSION_SECRET, APP_URL (from DOMAIN)
  │     └─► frontend container: NEXT_PUBLIC_API_URL, NEXT_PUBLIC_WS_URL (from DOMAIN)
  │
  └─► Injected directly into ALL containers by Coolify
        └─► PUBLIC_URL (only consumed by backend, not in docker-compose)
```

**Gotcha:** `PUBLIC_URL` is not in docker-compose.yml. It works because Coolify injects all env vars into every container. If you deploy outside Coolify, you need to add `PUBLIC_URL` to the backend service in docker-compose.yml.

---

### 4. What APP_URL vs PUBLIC_URL do

| Variable | Used for |
|----------|----------|
| `APP_URL` | Cookie domain, password reset links, Stripe redirect URLs, WebSocket URL derivation, `/api/config` response |
| `PUBLIC_URL` | **Only** the Resend webhook callback URL registered during onboarding (`PUBLIC_URL + /api/webhooks/resend/{orgId}`) |

In your setup both should be `https://mail.yourdomain.com`. They exist as separate vars because in some architectures the public-facing URL (for webhooks) differs from the app URL (for the browser).

---

### 5. Deploy

1. Set all env vars in Coolify
2. Click Deploy
3. Coolify builds both Docker images (backend + frontend), starts postgres + redis, then backend + frontend
4. First deploy takes longer (building Go binary + Next.js standalone)

---

### 6. Post-Deploy Setup

1. **Open** `https://mail.yourdomain.com`
2. **Sign up** — creates your org + admin user (no email verification in self-hosted mode)
3. **Onboarding** — enter your Resend API key
   - This encrypts and stores the key per-org in the database
   - Automatically registers the Resend webhook pointing at `PUBLIC_URL/api/webhooks/resend/{orgId}`
4. **Verify Resend domain configuration**
   - Resend dashboard → Domains → ensure MX, SPF, DKIM records are set for each domain you want to receive email on
5. **Sync emails** — Settings → Domains → Email Sync to pull historical emails from Resend

---

### 7. Wiping and Starting Fresh

If you need to start completely clean:

1. In Coolify: stop the resource
2. Delete the postgres volume (this destroys all data — emails, users, orgs, encrypted keys)
3. Redeploy
4. Re-do post-deploy setup (signup, onboarding, Resend API key, domain verification, sync)

**What you keep:** All env vars in Coolify persist. `ENCRYPTION_KEY` and `SESSION_SECRET` can stay the same or be regenerated (regenerating ENCRYPTION_KEY is fine on a fresh DB since there are no encrypted keys to decrypt).

---

### 8. Updating

Push to the GitHub repo, then either:
- Coolify auto-deploys (if configured)
- Or manually click Deploy in Coolify

Coolify rebuilds both images from the new commit. Postgres/Redis volumes persist across deploys — your data survives.

---

### 9. Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Webhook emails have no body | Old code before the fetch fix | Redeploy with latest commit. Existing empty emails need manual backfill or wipe+resync |
| Webhook not received at all | `PUBLIC_URL` wrong or missing | Check env var. Re-run onboarding or Settings → Domains → Refresh to re-register webhook |
| "ENCRYPTION_KEY is required" on startup | Missing env var | Set in Coolify env vars |
| Login doesn't persist | `APP_URL` doesn't match browser URL | Cookie domain derived from APP_URL must match |
| WebSocket won't connect | DOMAIN not matching or mixed http/https | Ensure DOMAIN matches the actual URL, Coolify handles TLS |
| Emails not arriving in Resend | MX/SPF/DKIM not configured | Check Resend dashboard → Domains → DNS records |

---

### Quick Reference: Minimum Env Vars for Clean Deploy

```env
DOMAIN=mail.yourdomain.com
PUBLIC_URL=https://mail.yourdomain.com
SESSION_SECRET=<openssl rand -hex 32>
ENCRYPTION_KEY=<openssl rand -base64 32>
POSTGRES_PASSWORD=<something-secure>
```

Everything else has working defaults or is optional.
