import { Page } from "@playwright/test";

// ---------------------------------------------------------------------------
// Test data helpers
// ---------------------------------------------------------------------------

/** Generate a unique email address for test isolation. */
export function uniqueEmail(prefix = "e2e"): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@e2e-test.com`;
}

/** A password that satisfies the validation rules (8+ chars, uppercase, lowercase, digit). */
export const VALID_PASSWORD = "TestPass1";

/** A password that is too short / missing complexity. */
export const WEAK_PASSWORD = "short";

// ---------------------------------------------------------------------------
// Direct API helpers (bypass the UI for fast setup)
// ---------------------------------------------------------------------------

const API_BASE = process.env.E2E_API_URL || "http://localhost:8080";

/**
 * Create an account directly through the backend API.
 * Returns the raw Response so callers can check status / read cookies.
 */
export async function apiSignup(
  email: string,
  password: string,
  orgName: string,
  name = "Test User",
) {
  const res = await fetch(`${API_BASE}/api/auth/signup`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, org_name: orgName, name }),
  });
  return res;
}

/**
 * Login through the backend API and return the Response (including Set-Cookie).
 */
export async function apiLogin(email: string, password: string) {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  return res;
}

/**
 * Signup via API then inject the auth cookie into the Playwright browser context
 * so subsequent page navigations are authenticated.
 */
export async function signupAndAuthenticate(
  page: Page,
  email: string,
  password: string,
  orgName: string,
  name = "Test User",
) {
  const res = await apiSignup(email, password, orgName, name);
  if (!res.ok) {
    throw new Error(`API signup failed: ${res.status} ${await res.text()}`);
  }

  // Extract Set-Cookie headers and inject into Playwright context
  const setCookies = res.headers.getSetCookie?.() ?? [];
  for (const raw of setCookies) {
    const [nameValue] = raw.split(";");
    const [cookieName, ...rest] = nameValue.split("=");
    const cookieValue = rest.join("=");
    await page.context().addCookies([
      {
        name: cookieName.trim(),
        value: cookieValue,
        domain: "localhost",
        path: "/",
      },
    ]);
  }
}

// ---------------------------------------------------------------------------
// Waiting helpers
// ---------------------------------------------------------------------------

/**
 * Wait until a network response matching `urlPattern` is received.
 * Useful for waiting on API calls triggered by UI interactions.
 */
export async function waitForRoute(page: Page, urlPattern: string | RegExp) {
  await page.waitForResponse((res) => {
    const url = res.url();
    if (typeof urlPattern === "string") return url.includes(urlPattern);
    return urlPattern.test(url);
  });
}
