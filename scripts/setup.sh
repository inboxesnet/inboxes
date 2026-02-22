#!/usr/bin/env bash
set -euo pipefail

# ─── Inboxes — First-time setup ──────────────────────────────────────────────
# Safe to re-run. Checks what you already have and only installs what's missing.
#
# Minimum requirements: PostgreSQL 13+, Go 1.23+, Node 18+, Redis 6+
#
# Two ways to use:
#   1. curl one-liner (clones the repo for you):
#      curl -fsSL https://raw.githubusercontent.com/headswim/inboxes/main/scripts/setup.sh | bash
#
#   2. From inside the repo:
#      ./scripts/setup.sh

REPO_URL="https://github.com/headswim/inboxes.git"
INSTALL_DIR="$HOME/inboxes"

# Minimum versions
MIN_PG=13
MIN_GO="1.23"
MIN_NODE=18
MIN_REDIS=6

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }

# Compare version strings: returns 0 if $1 >= $2
version_gte() {
  printf '%s\n%s' "$2" "$1" | sort -V -C
}

echo ""
echo "  ┌──────────────────────────────┐"
echo "  │   Inboxes — Setup            │"
echo "  └──────────────────────────────┘"
echo ""

# ─── 0. Detect: are we inside the repo, or running via curl? ─────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "")"

if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/../backend/go.mod" ]]; then
  PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
  info "Running from existing checkout: $PROJECT_DIR"
else
  if [[ -d "$INSTALL_DIR" && -f "$INSTALL_DIR/backend/go.mod" ]]; then
    PROJECT_DIR="$INSTALL_DIR"
    info "Found existing install at $PROJECT_DIR"
  else
    warn "Cloning Inboxes into $INSTALL_DIR..."
    if git clone "$REPO_URL" "$INSTALL_DIR" 2>/dev/null; then
      PROJECT_DIR="$INSTALL_DIR"
      info "Cloned to $PROJECT_DIR"
    else
      echo ""
      fail "Could not clone $REPO_URL
   The repo may be private or the URL may have changed.
   Try cloning manually and run ./scripts/setup.sh from inside the repo."
    fi
  fi
fi

# ─── 1. macOS check ─────────────────────────────────────────────────────────
if [[ "$(uname)" != "Darwin" ]]; then
  fail "This script is for macOS. For Linux/Docker, see the README."
fi

# ─── 2. Homebrew ─────────────────────────────────────────────────────────────
if ! command -v brew &>/dev/null; then
  warn "Homebrew not found. Installing..."
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

  if [[ -f /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  fi
else
  info "Homebrew found"
fi

# ─── 3. Go ───────────────────────────────────────────────────────────────────
if command -v go &>/dev/null; then
  GO_VER="$(go version | grep -oE '[0-9]+\.[0-9]+')"
  if version_gte "$GO_VER" "$MIN_GO"; then
    info "Go $GO_VER found"
  else
    fail "Go $GO_VER found but $MIN_GO+ required. Run: brew upgrade go"
  fi
else
  warn "Go not found. Installing..."
  brew install go
  info "Go installed"
fi

# ─── 4. Node ─────────────────────────────────────────────────────────────────
if command -v node &>/dev/null; then
  NODE_VER="$(node -v | grep -oE '[0-9]+' | head -1)"
  if [[ "$NODE_VER" -ge "$MIN_NODE" ]]; then
    info "Node v$NODE_VER found"
  else
    fail "Node v$NODE_VER found but v$MIN_NODE+ required. Run: brew upgrade node"
  fi
else
  warn "Node not found. Installing..."
  brew install node
  info "Node installed"
fi

# ─── 5. PostgreSQL ───────────────────────────────────────────────────────────
if command -v psql &>/dev/null; then
  PG_VER="$(psql --version | grep -oE '[0-9]+' | head -1)"
  if [[ "$PG_VER" -ge "$MIN_PG" ]]; then
    info "PostgreSQL $PG_VER found"
  else
    fail "PostgreSQL $PG_VER found but $MIN_PG+ required. Run: brew upgrade postgresql"
  fi
else
  warn "PostgreSQL not found. Installing..."
  brew install postgresql@16
  info "PostgreSQL installed"
fi

# ─── 6. Redis ────────────────────────────────────────────────────────────────
if command -v redis-cli &>/dev/null; then
  REDIS_VER="$(redis-cli --version | grep -oE '[0-9]+' | head -1)"
  if [[ "$REDIS_VER" -ge "$MIN_REDIS" ]]; then
    info "Redis $REDIS_VER found"
  else
    fail "Redis $REDIS_VER found but $MIN_REDIS+ required. Run: brew upgrade redis"
  fi
else
  warn "Redis not found. Installing..."
  brew install redis
  info "Redis installed"
fi

# ─── 7. Start services ──────────────────────────────────────────────────────
# Find the actual brew service name for whatever postgres version is installed
PG_SERVICE="$(brew services list 2>/dev/null | grep -oE 'postgresql(@[0-9]+)?' | head -1 || echo "")"
if [[ -z "$PG_SERVICE" ]]; then
  fail "PostgreSQL is installed but not registered as a brew service. Try: brew services start postgresql@$PG_VER"
fi

if brew services list | grep "$PG_SERVICE" | grep -q started; then
  info "$PG_SERVICE already running"
else
  warn "Starting $PG_SERVICE..."
  brew services start "$PG_SERVICE"
  sleep 3
fi

if brew services list | grep "redis" | grep -q started; then
  info "Redis already running"
else
  warn "Starting Redis..."
  brew services start redis
  sleep 2
fi

# ─── 8. Database ─────────────────────────────────────────────────────────────
if psql -lqt 2>/dev/null | cut -d\| -f1 | grep -qw inboxes; then
  info "Database 'inboxes' already exists"
else
  warn "Creating database..."
  createdb inboxes 2>/dev/null || true
  psql inboxes -c "
    DO \$\$
    BEGIN
      IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'inboxes') THEN
        CREATE ROLE inboxes WITH LOGIN PASSWORD 'inboxes';
      END IF;
    END
    \$\$;
    GRANT ALL PRIVILEGES ON DATABASE inboxes TO inboxes;
    GRANT ALL ON SCHEMA public TO inboxes;
  " 2>/dev/null
  info "Database 'inboxes' created"
fi

# ─── 9. .env file ───────────────────────────────────────────────────────────
ENV_FILE="$PROJECT_DIR/.env"
if [[ -f "$ENV_FILE" ]]; then
  info ".env already exists (not overwriting)"
else
  warn "Generating .env with random secrets..."
  cat > "$ENV_FILE" <<EOF
DATABASE_URL=postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable
REDIS_URL=redis://localhost:6379
SESSION_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -base64 32)
PUBLIC_URL=http://localhost:8080
NEXT_PUBLIC_API_URL=http://localhost:8080
EOF
  info ".env created"
fi

# ─── 10. Frontend dependencies ──────────────────────────────────────────────
if [[ -d "$PROJECT_DIR/frontend/node_modules" ]]; then
  info "Frontend dependencies already installed"
else
  warn "Installing frontend dependencies..."
  (cd "$PROJECT_DIR/frontend" && npm install)
  info "Frontend dependencies installed"
fi

# ─── Done ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}  Setup complete!${NC}"
echo ""
echo "  To start the app:"
echo ""
echo "    cd $PROJECT_DIR"
echo "    ./scripts/dev.sh"
echo ""
