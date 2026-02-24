#!/usr/bin/env bash
set -euo pipefail

# ─── Inboxes — Dev runner ────────────────────────────────────────────────────
# Starts Postgres, Redis, backend, and frontend.
# Ctrl+C stops everything cleanly.
# Safe to run multiple times — kills previous instances first.
# Usage: ./scripts/dev.sh

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Add brew to PATH for Apple Silicon Macs
if [[ -f /opt/homebrew/bin/brew ]]; then
  eval "$(/opt/homebrew/bin/brew shellenv)"
fi

echo ""
echo "  ┌──────────────────────────────┐"
echo "  │   Inboxes — Starting...      │"
echo "  └──────────────────────────────┘"
echo ""

# ─── Check setup ran ─────────────────────────────────────────────────────────
if [[ ! -f "$PROJECT_DIR/.env" ]]; then
  echo -e "${RED}No .env file found. Run ./scripts/setup.sh first.${NC}"
  exit 1
fi

# ─── Ensure services are running ─────────────────────────────────────────────
PG_SERVICE="$(brew services list 2>/dev/null | grep -oE 'postgresql(@[0-9]+)?' | head -1 || echo "")"
if [[ -z "$PG_SERVICE" ]]; then
  echo -e "${RED}PostgreSQL not found. Run ./scripts/setup.sh first.${NC}"
  exit 1
fi

if ! brew services list | grep "$PG_SERVICE" | grep -q started; then
  warn "Starting PostgreSQL..."
  brew services start "$PG_SERVICE"
  sleep 2
fi
info "PostgreSQL running ($PG_SERVICE)"

if ! brew services list | grep "redis" | grep -q started; then
  warn "Starting Redis..."
  brew services start redis
  sleep 2
fi
info "Redis running"

# ─── Kill anything already on our ports ───────────────────────────────────────
kill_port() {
  local port="$1"
  local pids
  pids=$(lsof -ti :"$port" 2>/dev/null || true)
  if [[ -n "$pids" ]]; then
    warn "Killing existing process on port $port..."
    echo "$pids" | xargs kill 2>/dev/null || true
    sleep 1
    # Force kill anything that didn't die gracefully
    pids=$(lsof -ti :"$port" 2>/dev/null || true)
    if [[ -n "$pids" ]]; then
      echo "$pids" | xargs kill -9 2>/dev/null || true
      sleep 1
    fi
  fi
}

kill_port 8080
kill_port 3000

# ─── Trap: clean shutdown on Ctrl+C ──────────────────────────────────────────
BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
  echo ""
  echo -e "${YELLOW}Shutting down...${NC}"

  # Kill our direct children
  [[ -n "$BACKEND_PID" ]]  && kill "$BACKEND_PID"  2>/dev/null
  [[ -n "$FRONTEND_PID" ]] && kill "$FRONTEND_PID" 2>/dev/null

  # Give them a moment, then force-kill anything still on our ports
  # (Next.js spawns child processes that survive parent kill)
  sleep 1
  lsof -ti :8080 2>/dev/null | xargs kill -9 2>/dev/null || true
  lsof -ti :3000 2>/dev/null | xargs kill -9 2>/dev/null || true

  wait 2>/dev/null
  info "Stopped."
  exit 0
}
trap cleanup SIGINT SIGTERM

# ─── Load .env into this shell ────────────────────────────────────────────────
set -a
source "$PROJECT_DIR/.env"
set +a

# Ensure frontend knows where the backend is
export NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://localhost:8080}"
export BACKEND_URL="${BACKEND_URL:-http://localhost:8080}"

# ─── Start backend ───────────────────────────────────────────────────────────
echo ""
info "Starting backend on :8080..."
cd "$PROJECT_DIR/backend" && go run ./cmd/api &
BACKEND_PID=$!

# ─── Start frontend ──────────────────────────────────────────────────────────
info "Starting frontend on :3000..."
cd "$PROJECT_DIR/frontend" && npm run dev &
FRONTEND_PID=$!

echo ""
echo -e "${GREEN}  ┌──────────────────────────────────────┐${NC}"
echo -e "${GREEN}  │  App:  http://localhost:3000         │${NC}"
echo -e "${GREEN}  │  API:  http://localhost:8080         │${NC}"
echo -e "${GREEN}  │                                      │${NC}"
echo -e "${GREEN}  │  Press Ctrl+C to stop                │${NC}"
echo -e "${GREEN}  └──────────────────────────────────────┘${NC}"
echo ""

# ─── Wait for either process to exit ─────────────────────────────────────────
wait
