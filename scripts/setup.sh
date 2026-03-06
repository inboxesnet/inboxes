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
warn()  { echo -ne "${YELLOW}[…]${NC} $1\r"; }
ok()  { echo -e "\r\033[K${GREEN}[✓]${NC} $1"; }
fail()  { echo -e "\n${RED}[✗]${NC} $1"; exit 1; }

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
      ok "Cloned to $PROJECT_DIR"
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
  warn "Installing Homebrew...                    "
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

  if [[ -f /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  fi
  ok "Homebrew installed"
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
  warn "Installing Go...                          "
  brew install go
  ok "Go installed"
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
  warn "Installing Node...                        "
  brew install node
  ok "Node installed"
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
  warn "Installing PostgreSQL...                  "
  brew install postgresql@16
  ok "PostgreSQL installed"
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
  warn "Installing Redis...                       "
  brew install redis
  ok "Redis installed"
fi

# ─── 7. Start services ──────────────────────────────────────────────────────
# Prefer a postgres that's already running; otherwise start the highest version installed
PG_SERVICE="$(brew services list 2>/dev/null | grep -E 'postgresql.*started' | grep -oE 'postgresql(@[0-9]+)?' | tail -1 || echo "")"
if [[ -z "$PG_SERVICE" ]]; then
  PG_SERVICE="$(brew services list 2>/dev/null | grep -oE 'postgresql(@[0-9]+)?' | tail -1 || echo "")"
fi
if [[ -z "$PG_SERVICE" ]]; then
  fail "PostgreSQL is installed but not registered as a brew service. Try: brew services start postgresql@$PG_VER"
fi

if brew services list | grep "$PG_SERVICE" | grep -q started; then
  info "$PG_SERVICE already running"
else
  warn "Starting $PG_SERVICE...                   "
  brew services start "$PG_SERVICE"
  sleep 3
  ok "$PG_SERVICE started"
fi

if brew services list | grep "redis" | grep -q started; then
  info "Redis already running"
else
  warn "Starting Redis...                         "
  brew services start redis
  sleep 2
  ok "Redis started"
fi

# ─── 8. .env file ───────────────────────────────────────────────────────────
ENV_FILE="$PROJECT_DIR/.env"
DB_NAME="${DB_NAME:-inboxes}"
DB_USER="${DB_USER:-inboxes}"
DB_PASS="${DB_PASS:-inboxes}"
PUBLIC_URL="${PUBLIC_URL:-http://localhost:8080}"

if [[ -f "$ENV_FILE" ]]; then
  info ".env already exists (not overwriting)"
  # Read DB settings from existing .env
  _DB_URL="$(grep -E '^DATABASE_URL=' "$ENV_FILE" | cut -d= -f2- || true)"
  if [[ -n "$_DB_URL" ]]; then
    DB_NAME="$(echo "$_DB_URL" | grep -oE '/[^/?]+\?' | tr -d '/?')"
    DB_USER="$(echo "$_DB_URL" | grep -oE '://[^:]+:' | tr -d ':/' | head -1)"
    DB_PASS="$(echo "$_DB_URL" | sed -E 's|.*://[^:]+:([^@]+)@.*|\1|')"
  fi
else
  # ─── Install mode chooser ─────────────────────────────────────────────
  # When piped via curl, stdin isn't a terminal — skip prompts and use quick install
  INSTALL_MODE="quick"
  if [[ -t 0 ]]; then
    # Interactive arrow-key selector
    local_selected=0
    local_options=("Quick install" "Custom install (recommended)")
    local_descs=(
      "Default credentials, secrets generated automatically."
      "Set your own credentials for a more secure setup."
    )

    # Hide cursor
    printf '\033[?25l'

    # Draw menu
    draw_menu() {
      # Move cursor up to redraw (skip on first draw)
      if [[ "${1:-}" == "redraw" ]]; then
        printf '\033[6A'
      fi
      echo "  How would you like to set up?"
      echo ""
      for i in 0 1; do
        if [[ $local_selected -eq $i ]]; then
          echo -e "  ${GREEN}▸ ${local_options[$i]}${NC}"
          echo -e "    ${local_descs[$i]}"
        else
          echo -e "    ${local_options[$i]}"
          echo -e "    ${local_descs[$i]}"
        fi
      done
    }

    draw_menu

    # Read arrow keys
    while true; do
      read -rsn1 key </dev/tty
      if [[ "$key" == $'\x1b' ]]; then
        read -rsn2 rest </dev/tty
        key+="$rest"
      fi
      case "$key" in
        $'\x1b[A'|k) # Up
          if [[ $local_selected -gt 0 ]]; then
            local_selected=$((local_selected - 1))
            draw_menu redraw
          fi
          ;;
        $'\x1b[B'|j) # Down
          if [[ $local_selected -lt 1 ]]; then
            local_selected=$((local_selected + 1))
            draw_menu redraw
          fi
          ;;
        "") # Enter
          break
          ;;
      esac
    done

    # Show cursor
    printf '\033[?25h'
    echo ""

    if [[ $local_selected -eq 1 ]]; then
      INSTALL_MODE="custom"
    fi
  fi

  if [[ "$INSTALL_MODE" == "custom" ]]; then
    printf "  PostgreSQL database name [inboxes]: "
    read -r _input </dev/tty; [[ -n "$_input" ]] && DB_NAME="$_input"

    printf "  PostgreSQL user [inboxes]: "
    read -r _input </dev/tty; [[ -n "$_input" ]] && DB_USER="$_input"

    printf "  PostgreSQL password [inboxes]: "
    read -r _input </dev/tty; [[ -n "$_input" ]] && DB_PASS="$_input"

    echo ""
    echo "  Without a public URL, everything works except live inbound email."
    echo "  You can always update PUBLIC_URL in .env later."
    printf "  Public URL for webhooks [http://localhost:8080]: "
    read -r _input </dev/tty; [[ -n "$_input" ]] && PUBLIC_URL="$_input"
    echo ""
  fi

  warn "Generating .env...                        "
  SESSION_SECRET="$(openssl rand -hex 32)"
  ENCRYPTION_KEY="$(openssl rand -base64 32)"
  cat > "$ENV_FILE" <<EOF
DATABASE_URL=postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable
REDIS_URL=redis://localhost:6379
SESSION_SECRET="${SESSION_SECRET}"
ENCRYPTION_KEY="${ENCRYPTION_KEY}"
PUBLIC_URL=${PUBLIC_URL}
NEXT_PUBLIC_API_URL=http://localhost:8080
EOF
  ok ".env created"
fi

# ─── 9. Database ─────────────────────────────────────────────────────────────
if psql -lqt 2>/dev/null | cut -d\| -f1 | grep -qw "$DB_NAME"; then
  info "Database '$DB_NAME' already exists"
else
  warn "Creating database...                      "
  createdb "$DB_NAME" 2>/dev/null || true
  psql "$DB_NAME" -c "
    DO \$\$
    BEGIN
      IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '$DB_USER') THEN
        CREATE ROLE $DB_USER WITH LOGIN PASSWORD '$DB_PASS';
      END IF;
    END
    \$\$;
    GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;
    GRANT ALL ON SCHEMA public TO $DB_USER;
  " 2>/dev/null
  ok "Database '$DB_NAME' created"
fi

# ─── 10. Frontend dependencies ──────────────────────────────────────────────
if [[ -d "$PROJECT_DIR/frontend/node_modules" ]]; then
  info "Frontend dependencies already installed"
else
  warn "Installing frontend dependencies...       "
  (cd "$PROJECT_DIR/frontend" && npm install --silent 2>/dev/null)
  ok "Frontend dependencies installed"
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
