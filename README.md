# Inboxes

## Dev Setup

```bash
# 1. Stop Homebrew postgres (conflicts with Docker on port 5432)
brew services stop postgresql@16

# 2. Start postgres + redis via Docker
docker compose up -d postgres redis

# 3. Backend (runs migrations automatically)
cd backend && go run ./cmd/api

# 4. Frontend
cd frontend && npm run dev
```

Backend: http://localhost:8080
Frontend: http://localhost:3000
