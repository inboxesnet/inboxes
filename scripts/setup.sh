#!/usr/bin/env bash
set -euo pipefail

# ─── Inboxes — First-time setup ──────────────────────────────────────────────
# Safe to re-run. Checks what you already have and only installs what's missing.
#
# Two ways to use:
#   1. curl one-liner (clones the repo for you):
#      curl -fsSL https://raw.githubusercontent.com/headswim/inboxes/main/scripts/setup.sh | bash
#
#   2. From inside the repo:
#      ./scripts/setup.sh

REPO_URL="https://github.com/headswim/inboxes.git"
INSTALL_DIR="$HOME/inboxes"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }

echo ""
echo "  ┌──────────────────────────────┐"
echo "  │   Inboxes — Setup            │"
echo "  └──────────────────────────────┘"
echo ""

# ─── 0. Detect: are we inside the repo, or running via curl? ─────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "")"

if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/../backend/go.mod" ]]; then
  # Running from inside the repo (./scripts/setup.sh)
  PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
  info "Running from existing checkout: $PROJECT_DIR"
else
  # Running via curl | bash — need to clone
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

  # Add brew to PATH for Apple Silicon Macs (this session only)
  if [[ -f /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  fi
else
  info "Homebrew found"
fi

# ─── 3. System dependencies ─────────────────────────────────────────────────
install_if_missing() {
  local cmd="$1"
  local formula="$2"
  if ! command -v "$cmd" &>/dev/null; then
    warn "$cmd not found. Installing $formula..."
    brew install "$formula"
  else
    info "$cmd found"
  fi
}

install_if_missing go       go
install_if_missing node     node
install_if_missing psql     postgresql@16
install_if_missing redis-cli redis

# ─── 4. Start services ──────────────────────────────────────────────────────
start_service() {
  local service="$1"
  if brew services list | grep "$service" | grep -q started; then
    info "$service already running"
  else
    warn "Starting $service..."
    brew services start "$service"
    sleep 2
  fi
}

start_service postgresql@16
start_service redis

# ─── 5. Database ─────────────────────────────────────────────────────────────
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

# ─── 6. .env file ───────────────────────────────────────────────────────────
ENV_FILE="$PROJECT_DIR/.env"
if [[ -f "$ENV_FILE" ]]; then
  info ".env already exists (not overwriting)"
else
  warn "Generating .env with random secrets..."
  cat > "$ENV_FILE" <<EOF
DATABASE_URL=postgres://inboxes:inboxes@localhost:5432/inboxes?sslmode=disable
REDIS_URL=redis://localhost:6379
SESSION_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
PUBLIC_URL=http://localhost:8080
EOF
  info ".env created"
fi

# ─── 7. Frontend dependencies ───────────────────────────────────────────────
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
