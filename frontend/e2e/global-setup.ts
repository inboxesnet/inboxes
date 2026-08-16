import { chromium, FullConfig } from "@playwright/test";

/**
 * Global setup: log in as the seeded instance admin and save the session
 * as storageState so specs start authenticated.
 *
 * Prerequisites (self-hosted mode):
 *   - Backend on :8080 with first-run setup completed for E2E_ADMIN_EMAIL
 *   - Set HIBP_CHECK_DISABLED=true on the backend if the test password
 *     appears in breach data
 */
const API_BASE = process.env.E2E_API_URL || "http://localhost:8080";
const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL || "e2e-admin@e2e-test.com";
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD || "TestPass1";

export const STORAGE_STATE = "e2e/.auth/admin.json";

async function globalSetup(_config: FullConfig) {
  // Best effort: clear leftover rate-limit keys from a previous run so
  // back-to-back local runs stay deterministic. Ignore failures (no
  // redis-cli, remote Redis, etc.).
  try {
    const { execSync } = await import("child_process");
    execSync('redis-cli --scan --pattern "rl:*" | xargs -r redis-cli del', {
      stdio: "ignore",
      timeout: 10000,
    });
  } catch {
    // ignore
  }

  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: ADMIN_EMAIL, password: ADMIN_PASSWORD }),
  });
  if (!res.ok) {
    throw new Error(
      `global-setup: admin login failed: ${res.status} ${await res.text()}`,
    );
  }

  const setCookies = res.headers.getSetCookie?.() ?? [];
  const browser = await chromium.launch();
  const context = await browser.newContext();
  for (const raw of setCookies) {
    const [nameValue] = raw.split(";");
    const [cookieName, ...rest] = nameValue.split("=");
    await context.addCookies([
      {
        name: cookieName.trim(),
        value: rest.join("="),
        domain: "localhost",
        path: "/",
      },
    ]);
  }
  await context.storageState({ path: STORAGE_STATE });
  await browser.close();
}

export default globalSetup;
