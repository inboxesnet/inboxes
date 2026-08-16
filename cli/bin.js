#!/usr/bin/env node
// inboxes — connect coding agents to an Inboxes server over MCP.
//
//   npx inboxes setup [--url https://your-host] [--key inbx_k_...] [--yes]
//
// The setup command signs you in through the browser (OAuth 2.1 + PKCE),
// trades the session for a durable API key, and writes MCP config for the
// harnesses it finds: Claude Code, Codex, and opencode.
// Zero dependencies; Node 18+.

"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");
const readline = require("node:readline/promises");
const { spawnSync } = require("node:child_process");

const DEFAULT_URL = "https://app.inboxes.net";

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--url" || a === "--key" || a === "--name") args[a.slice(2)] = argv[++i];
    else if (a === "--yes" || a === "-y") args.yes = true;
    else if (a === "--no-browser") args.noBrowser = true;
    else args._.push(a);
  }
  return args;
}

function b64url(buf) {
  return buf.toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function openBrowser(url) {
  const platform = process.platform;
  const cmd = platform === "darwin" ? "open" : platform === "win32" ? "cmd" : "xdg-open";
  const args = platform === "win32" ? ["/c", "start", "", url] : [url];
  const res = spawnSync(cmd, args, { stdio: "ignore" });
  return res.status === 0;
}

async function fetchJSON(url, options = {}) {
  const res = await fetch(url, options);
  const text = await res.text();
  let body;
  try {
    body = JSON.parse(text);
  } catch {
    body = { raw: text };
  }
  if (!res.ok) {
    const msg = body.error_description || body.error || text || res.statusText;
    throw new Error(`${url} -> HTTP ${res.status}: ${msg}`);
  }
  return body;
}

// ---- OAuth flow --------------------------------------------------------

async function oauthLogin(baseURL, noBrowser) {
  const meta = await fetchJSON(`${baseURL}/.well-known/oauth-authorization-server`);
  for (const k of ["authorization_endpoint", "token_endpoint", "registration_endpoint"]) {
    if (!meta[k]) throw new Error(`server metadata is missing ${k} — is this an Inboxes server?`);
  }

  // Local callback server on a random port.
  const server = http.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  const redirectURI = `http://127.0.0.1:${port}/callback`;

  const client = await fetchJSON(meta.registration_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      client_name: `Inboxes CLI (${os.hostname()})`,
      redirect_uris: [redirectURI],
    }),
  });

  const verifier = b64url(crypto.randomBytes(48));
  const challenge = b64url(crypto.createHash("sha256").update(verifier).digest());
  const state = b64url(crypto.randomBytes(16));

  const authURL = new URL(meta.authorization_endpoint);
  authURL.searchParams.set("response_type", "code");
  authURL.searchParams.set("client_id", client.client_id);
  authURL.searchParams.set("redirect_uri", redirectURI);
  authURL.searchParams.set("state", state);
  authURL.searchParams.set("code_challenge", challenge);
  authURL.searchParams.set("code_challenge_method", "S256");

  console.log("\nOpen this URL to approve the connection:");
  console.log("  " + authURL.toString() + "\n");
  if (!noBrowser) openBrowser(authURL.toString());

  const code = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      server.close();
      reject(new Error("timed out waiting for browser approval (5 minutes)"));
    }, 5 * 60 * 1000);
    server.on("request", (req, res) => {
      const u = new URL(req.url, redirectURI);
      if (u.pathname !== "/callback") {
        res.writeHead(404).end();
        return;
      }
      res.writeHead(200, { "Content-Type": "text/html" });
      const err = u.searchParams.get("error");
      if (err) {
        res.end("<h3>Connection denied.</h3>You can close this tab.");
        clearTimeout(timer);
        server.close();
        reject(new Error("authorization denied: " + err));
        return;
      }
      if (u.searchParams.get("state") !== state) {
        res.end("<h3>State mismatch.</h3>You can close this tab.");
        clearTimeout(timer);
        server.close();
        reject(new Error("OAuth state mismatch"));
        return;
      }
      res.end("<h3>Inboxes is connected.</h3>You can close this tab and return to the terminal.");
      clearTimeout(timer);
      server.close();
      resolve(u.searchParams.get("code"));
    });
  });

  const form = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    code_verifier: verifier,
    client_id: client.client_id,
    redirect_uri: redirectURI,
  });
  const tok = await fetchJSON(meta.token_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: form.toString(),
  });
  return tok.access_token;
}

// ---- Harness config writers -------------------------------------------

function configureClaude(mcpURL, key) {
  const probe = spawnSync("claude", ["--version"], { stdio: "ignore" });
  if (probe.error || probe.status !== 0) return { found: false };
  const res = spawnSync(
    "claude",
    ["mcp", "add", "-s", "user", "--transport", "http", "inboxes", mcpURL,
      "--header", `Authorization: Bearer ${key}`],
    { encoding: "utf8" }
  );
  if (res.status === 0) return { found: true, ok: true };
  const out = (res.stdout || "") + (res.stderr || "");
  if (/already exists/i.test(out)) {
    return { found: true, ok: false, note: "already configured — run `claude mcp remove -s user inboxes` first to reconnect" };
  }
  return { found: true, ok: false, note: out.trim().slice(0, 200) };
}

function configureCodex(mcpURL, key, home) {
  const dir = path.join(home, ".codex");
  const file = path.join(dir, "config.toml");
  if (!fs.existsSync(dir)) return { found: false };
  let current = "";
  if (fs.existsSync(file)) current = fs.readFileSync(file, "utf8");
  if (current.includes("[mcp_servers.inboxes]")) {
    return { found: true, ok: false, note: `already configured — edit ${file} to update` };
  }
  // Stdio bridge via mcp-remote: works on every Codex build, no native
  // streamable-HTTP support required.
  const block = [
    "",
    "[mcp_servers.inboxes]",
    'command = "npx"',
    `args = ["-y", "mcp-remote", ${JSON.stringify(mcpURL)}, "--header", ${JSON.stringify("Authorization: Bearer " + key)}]`,
    "",
  ].join("\n");
  fs.appendFileSync(file, block);
  return { found: true, ok: true, file };
}

function configureOpencode(mcpURL, key, home) {
  const dir = path.join(home, ".config", "opencode");
  const file = path.join(dir, "opencode.json");
  if (!fs.existsSync(dir) && !fs.existsSync(file)) return { found: false };
  let cfg = {};
  if (fs.existsSync(file)) {
    try {
      cfg = JSON.parse(fs.readFileSync(file, "utf8"));
    } catch {
      return { found: true, ok: false, note: `${file} is not valid JSON — fix it and rerun` };
    }
  }
  cfg.mcp = cfg.mcp || {};
  if (cfg.mcp.inboxes) {
    return { found: true, ok: false, note: `already configured — edit ${file} to update` };
  }
  cfg.mcp.inboxes = {
    type: "remote",
    url: mcpURL,
    headers: { Authorization: `Bearer ${key}` },
    enabled: true,
  };
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(file, JSON.stringify(cfg, null, 2) + "\n");
  return { found: true, ok: true, file };
}

// ---- setup command -----------------------------------------------------

async function setup(args) {
  let base = args.url;
  if (!base) {
    if (args.yes) {
      base = DEFAULT_URL;
    } else {
      const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
      base = (await rl.question(`Inboxes server URL [${DEFAULT_URL}]: `)).trim() || DEFAULT_URL;
      rl.close();
    }
  }
  base = base.replace(/\/+$/, "");
  if (!/^https?:\/\//.test(base)) base = "https://" + base;
  const mcpURL = `${base}/mcp`;

  let key = args.key;
  if (!key) {
    console.log(`Connecting to ${base} ...`);
    const accessToken = await oauthLogin(base, args.noBrowser || process.env.INBOXES_NO_BROWSER === "1");
    // Trade the OAuth token for a durable API key so harnesses that cannot
    // refresh OAuth keep working. Revocable in Settings -> Agents.
    const exchanged = await fetchJSON(`${base}/api/agent-keys/exchange`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ name: args.name || `cli-${os.hostname()}` }),
    });
    key = exchanged.key;
    console.log("Signed in. Created API key: " + exchanged.name);
  }

  const home = os.homedir();
  const results = {
    "Claude Code": configureClaude(mcpURL, key),
    Codex: configureCodex(mcpURL, key, home),
    opencode: configureOpencode(mcpURL, key, home),
  };

  console.log("");
  let configured = 0;
  for (const [name, r] of Object.entries(results)) {
    if (!r.found) {
      console.log(`  -  ${name}: not detected, skipped`);
    } else if (r.ok) {
      configured++;
      console.log(`  ✓  ${name}: connected${r.file ? " (" + r.file + ")" : ""}`);
    } else {
      console.log(`  !  ${name}: ${r.note}`);
    }
  }

  if (configured === 0) {
    console.log("\nNo harness was configured. Manual setup:");
    console.log(`  claude mcp add -s user --transport http inboxes ${mcpURL} --header "Authorization: Bearer ${key}"`);
  }
  console.log("\nAgents can read mail and create drafts. Sending stays off until an");
  console.log("admin enables it in Settings -> Agents. Revoke this key there anytime.");
}

// ---- main --------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const cmd = args._[0];
  if (cmd === "setup") {
    await setup(args);
    return;
  }
  console.log(`inboxes — connect agents to your Inboxes email server

Usage:
  npx inboxes setup                 sign in via browser and auto-configure
  npx inboxes setup --url <server>  point at a self-hosted server
  npx inboxes setup --key <key>     skip browser auth, use an existing API key
  npx inboxes setup --yes           accept defaults, no prompts

Configures: Claude Code, Codex (~/.codex/config.toml), opencode
(~/.config/opencode/opencode.json). Keys are managed in Settings -> Agents.`);
}

main().catch((err) => {
  console.error("\nerror: " + err.message);
  process.exit(1);
});
