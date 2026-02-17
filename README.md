# Inboxes

A self-hostable email client powered by [Resend](https://resend.com). Connect your domain, import existing emails, and manage your team's inbox with a Gmail-quality UI. One `docker compose up` on any VPS.

## Prerequisites

- A VPS with Docker and Docker Compose
- A domain name pointing to your server (A record)
- A [Resend](https://resend.com) account with a verified domain

## Quick Start

```bash
git clone https://github.com/inboxes/inboxes.git
cd inboxes

# Copy and fill in your config
cp .env.example .env

# Generate secrets
echo "ENCRYPTION_KEY=$(openssl rand -hex 32)" >> .env
echo "SESSION_SECRET=$(openssl rand -hex 32)" >> .env

# Set your domain
# Edit .env and set DOMAIN=mail.yourdomain.com

# Launch
docker compose up -d
```

Caddy automatically provisions HTTPS via Let's Encrypt. Visit `https://mail.yourdomain.com` to sign up and connect your Resend API key.

## Local Development

```bash
# Start postgres + redis (via Homebrew or Docker)
brew services start postgresql@16
brew services start redis

# Backend (runs migrations automatically)
cd backend && go run ./cmd/api

# Frontend (separate terminal)
cd frontend && npm run dev
```

- Backend: http://localhost:8080
- Frontend: http://localhost:3000

For webhooks in local dev, use [ngrok](https://ngrok.com) to expose port 8080.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOMAIN` | Yes | — | Your domain (e.g. `mail.example.com`) |
| `ENCRYPTION_KEY` | Yes | — | 32-byte hex key for encrypting Resend API keys |
| `SESSION_SECRET` | Yes | — | Secret for signing JWT tokens |
| `POSTGRES_USER` | No | `inboxes` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `inboxes` | PostgreSQL password |
| `POSTGRES_DB` | No | `inboxes` | PostgreSQL database name |
| `REDIS_URL` | No | `redis://redis:6379` | Redis connection URL |
| `API_PORT` | No | `8080` | Backend port (inside container) |
| `RESEND_SYSTEM_API_KEY` | No | — | For sending invite/reset emails from the app |

## Architecture

- **Backend:** Go + Chi router + pgx + Redis
- **Frontend:** Next.js 15, React 19, Tailwind CSS, shadcn/ui
- **Database:** PostgreSQL 16
- **Cache/PubSub:** Redis 7
- **Reverse Proxy:** Caddy 2 (auto-HTTPS)
- **Real-time:** WebSocket hub with Redis pub/sub

## License

[MIT](LICENSE)
