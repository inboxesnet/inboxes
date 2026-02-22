# Inboxes

A self-hostable email client powered by [Resend](https://resend.com). Connect your domain, import existing emails, and manage your inbox with a clean UI.

## Install (macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/headswim/inboxes/main/scripts/setup.sh | bash
```

This installs everything you need (Homebrew, Go, Node, PostgreSQL, Redis), creates the database, and generates your config. Then:

```bash
cd ~/inboxes
./scripts/dev.sh
```

Open **http://localhost:3000** and follow the onboarding.

### Already have the code?

```bash
./scripts/setup.sh   # one-time — installs deps, creates DB, generates .env
./scripts/dev.sh     # starts everything — Ctrl+C to stop
```

## Deploy (VPS with Docker)

For production on a VPS with a domain name:

```bash
git clone https://github.com/headswim/inboxes.git
cd inboxes

cp .env.example .env
# Edit .env — set DOMAIN, generate ENCRYPTION_KEY and SESSION_SECRET:
#   openssl rand -hex 32

docker compose up -d
```

Caddy handles HTTPS automatically via Let's Encrypt. Visit `https://your.domain.com` to get started.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOMAIN` | Production | — | Your domain (e.g. `mail.example.com`) |
| `ENCRYPTION_KEY` | Yes | — | `openssl rand -hex 32` |
| `SESSION_SECRET` | Yes | — | `openssl rand -hex 32` |
| `DATABASE_URL` | No | `postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable` | PostgreSQL connection |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection |
| `PUBLIC_URL` | No | `http://localhost:8080` | Backend URL (for webhooks) |
| `RESEND_SYSTEM_API_KEY` | No | — | For sending invite/reset emails |
| `API_PORT` | No | `8080` | Backend port |

## Architecture

- **Backend:** Go + Chi router + pgx + Redis
- **Frontend:** Next.js 15, React 19, Tailwind CSS, shadcn/ui
- **Database:** PostgreSQL 16
- **Cache/PubSub:** Redis 7
- **Reverse Proxy:** Caddy 2 (auto-HTTPS, production only)

## License

[MIT](LICENSE)
