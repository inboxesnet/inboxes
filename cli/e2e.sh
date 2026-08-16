#!/bin/bash
# End-to-end test for `npx inboxes setup`: real backend, real CLI, headless
# OAuth approval, then assert every harness config it writes.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
SCRATCH="$(mktemp -d)"
PORT=8091
BASE="http://127.0.0.1:$PORT"
SECRET="cli-e2e-session-secret-0123456789abcdef"
DB="postgres://inboxes:inboxes@localhost:5432/inboxes_test?sslmode=disable"

cleanup() {
  [ -n "${BACKEND_PID:-}" ] && kill "$BACKEND_PID" 2>/dev/null || true
  # `go run` leaves the compiled child alive when only the parent dies —
  # kill by port so no orphan keeps consuming the shared test Redis queue.
  lsof -ti ":$PORT" 2>/dev/null | xargs kill 2>/dev/null || true
  [ -n "${CLI_PID:-}" ] && kill "$CLI_PID" 2>/dev/null || true
}
trap cleanup EXIT

# --- 1. Start the backend against the test DB -------------------------
cd "$REPO/backend"
DATABASE_URL="$DB" \
REDIS_URL="redis://localhost:6379/1" \
SESSION_SECRET="$SECRET" \
ENCRYPTION_KEY="$(printf '01234567890123456789012345678901' | base64)" \
APP_URL="$BASE" \
PUBLIC_URL="$BASE" \
API_PORT="$PORT" \
HIBP_CHECK_DISABLED=true \
go run ./cmd/api >"$SCRATCH/backend.log" 2>&1 &
BACKEND_PID=$!

for i in $(seq 1 60); do
  curl -sf "$BASE/api/health" >/dev/null 2>&1 && break
  sleep 0.5
  [ "$i" = 60 ] && { echo "backend never came up"; tail -20 "$SCRATCH/backend.log"; exit 1; }
done
echo "backend up"

# --- 2. Seed org + admin user ------------------------------------------
ORG_ID=$(psql "$DB" -tA -c "INSERT INTO orgs (name) VALUES ('cli-e2e') RETURNING id" | head -1)
USER_ID=$(psql "$DB" -tA -c "INSERT INTO users (org_id, email, name, role, status, password_hash, email_verified) VALUES ('$ORG_ID', 'cli-e2e@test.com', 'CLI E2E', 'admin', 'active', 'x', true) RETURNING id" | head -1)
JWT=$(go run ./cmd/mintjwt "$SECRET" "$USER_ID" "$ORG_ID")
echo "seeded org=$ORG_ID"

# --- 3. Fake HOME + fake claude shim ------------------------------------
export HOME="$SCRATCH/home"
mkdir -p "$HOME/.codex" "$HOME/.config/opencode"
mkdir -p "$SCRATCH/bin"
cat > "$SCRATCH/bin/claude" <<EOF
#!/bin/bash
echo "\$@" >> "$SCRATCH/claude-shim.log"
exit 0
EOF
chmod +x "$SCRATCH/bin/claude"
export PATH="$SCRATCH/bin:$PATH"

# --- 4. Run the CLI ------------------------------------------------------
node "$REPO/cli/bin.js" setup --url "$BASE" --yes --no-browser >"$SCRATCH/cli.log" 2>&1 &
CLI_PID=$!

# Wait for the authorize URL to appear.
AUTH_URL=""
for i in $(seq 1 40); do
  AUTH_URL=$(grep -o 'http://[^ ]*oauth/authorize?[^ ]*' "$SCRATCH/cli.log" | head -1 || true)
  [ -n "$AUTH_URL" ] && break
  sleep 0.5
done
[ -z "$AUTH_URL" ] && { echo "no authorize URL from CLI"; cat "$SCRATCH/cli.log"; exit 1; }
echo "authorize URL captured"

# --- 5. Headless consent: approve as the seeded user --------------------
param() { echo "$AUTH_URL" | sed -n "s/.*[?&]$1=\([^&]*\).*/\1/p"; }
CLIENT_ID=$(param client_id)
REDIRECT_URI=$(python3 -c "import urllib.parse,sys; print(urllib.parse.unquote(sys.argv[1]))" "$(param redirect_uri)")
STATE=$(param state)
CHALLENGE=$(param code_challenge)

REDIRECT_TO=$(curl -sf -X POST "$BASE/api/oauth/approve" \
  -H "Content-Type: application/json" \
  -H "X-Requested-With: XMLHttpRequest" \
  -H "Cookie: token=$JWT" \
  -d "{\"client_id\":\"$CLIENT_ID\",\"redirect_uri\":\"$REDIRECT_URI\",\"state\":\"$STATE\",\"code_challenge\":\"$CHALLENGE\",\"code_challenge_method\":\"S256\"}" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['redirect_to'])")
echo "approved; hitting CLI callback"
curl -sf "$REDIRECT_TO" >/dev/null

# --- 6. Wait for CLI to finish and assert --------------------------------
wait "$CLI_PID"; CLI_EXIT=$?
CLI_PID=""
echo "--- cli output ---"; cat "$SCRATCH/cli.log"; echo "------------------"
[ "$CLI_EXIT" = 0 ] || { echo "CLI exited $CLI_EXIT"; exit 1; }

grep -q "mcp add -s user --transport http inboxes $BASE/mcp" "$SCRATCH/claude-shim.log" \
  || { echo "FAIL: claude shim not called correctly"; cat "$SCRATCH/claude-shim.log"; exit 1; }
grep -q "\[mcp_servers.inboxes\]" "$HOME/.codex/config.toml" \
  || { echo "FAIL: codex config missing"; exit 1; }
grep -q '"inboxes"' "$HOME/.config/opencode/opencode.json" \
  || { echo "FAIL: opencode config missing"; exit 1; }

# --- 7. The provisioned key actually works on /mcp -----------------------
KEY=$(python3 -c "import json; print(json.load(open('$HOME/.config/opencode/opencode.json'))['mcp']['inboxes']['headers']['Authorization'].split()[-1])")
TOOLS=$(curl -sf -X POST "$BASE/mcp" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KEY" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}')
echo "$TOOLS" | grep -q '"create_draft"' || { echo "FAIL: key does not work on /mcp"; echo "$TOOLS"; exit 1; }

# The key row is named and attributed correctly.
psql "$DB" -tA -c "SELECT name FROM agent_tokens WHERE org_id='$ORG_ID' AND kind='key' AND name LIKE 'cli-%'" | grep -q cli- \
  || { echo "FAIL: exchanged key row missing"; exit 1; }

# --- 8. Cleanup seeded data ----------------------------------------------
psql "$DB" -c "DELETE FROM agent_tokens WHERE org_id='$ORG_ID'; DELETE FROM oauth_codes WHERE org_id='$ORG_ID'; DELETE FROM users WHERE org_id='$ORG_ID'; DELETE FROM orgs WHERE id='$ORG_ID';" >/dev/null

echo ""
echo "CLI E2E: ALL PASS"
