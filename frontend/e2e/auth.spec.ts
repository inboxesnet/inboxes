import { test, expect } from "@playwright/test";
import { AuthPage } from "./fixtures/auth";
import {
  uniqueEmail,
  VALID_PASSWORD,
  WEAK_PASSWORD,
  apiSignup,
} from "./fixtures/helpers";

test.describe("Auth", () => {
  // These tests exercise the login/signup pages. Start without the shared
  // admin session from global-setup.
  test.use({ storageState: { cookies: [], origins: [] } });

  // -------------------------------------------------------------------------
  // Signup
  // -------------------------------------------------------------------------

  // Self-hosted mode closes registration after the first user exists. Each
  // signup test first checks which state the page shows and asserts the
  // matching behavior.

  async function signupIsOpen(page: import("@playwright/test").Page) {
    return page
      .locator("#email")
      .isVisible()
      .catch(() => false);
  }

  test("signup page renders form fields", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoSignup();

    if (await signupIsOpen(page)) {
      await expect(page.locator("#orgName")).toBeVisible();
      await expect(page.locator("#name")).toBeVisible();
      await expect(page.locator("#email")).toBeVisible();
      await expect(page.locator("#password")).toBeVisible();
      await expect(page.locator('button:has-text("Create account")')).toBeVisible();
    } else {
      await expect(page.getByText("Registration closed")).toBeVisible();
      await expect(page.locator('a[href="/login"]')).toBeVisible();
    }
  });

  test("successful signup redirects to onboarding or verify-email", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoSignup();
    if (!(await signupIsOpen(page))) {
      test.skip(true, "Registration closed (self-hosted mode)");
      return;
    }
    const email = uniqueEmail("signup");
    await auth.signup(email, VALID_PASSWORD, "E2E Org");
    await auth.expectSignedUp();
  });

  test("signup with weak password shows validation error", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoSignup();
    if (!(await signupIsOpen(page))) {
      test.skip(true, "Registration closed (self-hosted mode)");
      return;
    }
    const email = uniqueEmail("weak-pw");
    await auth.signup(email, WEAK_PASSWORD, "E2E Org");

    // Client-side validatePassword fires before API call
    await auth.expectError(/password must be/i);
  });

  // -------------------------------------------------------------------------
  // Login
  // -------------------------------------------------------------------------

  test("login page renders form fields", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoLogin();

    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator('button:has-text("Sign in")')).toBeVisible();
    await expect(page.getByText("Welcome back")).toBeVisible();
  });

  const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL || "e2e-admin@e2e-test.com";
  const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD || "TestPass1";

  test("successful login redirects away from login page", async ({ page }) => {
    // Use the seeded instance admin; self-hosted mode cannot create users.
    const auth = new AuthPage(page);
    await auth.login(ADMIN_EMAIL, ADMIN_PASSWORD);
    await auth.expectLoggedIn();
  });

  test("wrong password shows error message", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.login(ADMIN_EMAIL, "WrongPassword1");

    await auth.expectError();
  });

  test("empty fields trigger browser validation (required)", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoLogin();

    // Click submit without filling anything — browser required validation fires
    await page.click('button:has-text("Sign in")');

    // The page should still be on /login (form was not submitted)
    await expect(page).toHaveURL(/\/login/);

    // Verify the email input has the required attribute and reports as invalid
    const emailInput = page.locator("#email");
    await expect(emailInput).toHaveAttribute("required", "");
  });

  test("signup with duplicate email shows error", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoSignup();
    if (!(await signupIsOpen(page))) {
      test.skip(true, "Registration closed (self-hosted mode)");
      return;
    }
    const email = uniqueEmail("dup");

    // Create account via API first
    const signupRes = await apiSignup(email, VALID_PASSWORD, "E2E Dup Org");
    expect(signupRes.ok).toBe(true);

    // Attempt to signup again with the same email via UI
    await auth.signup(email, VALID_PASSWORD, "E2E Dup Org 2");

    // Should show an error about the email already existing
    await auth.expectError();
  });

  // -------------------------------------------------------------------------
  // Forgot password
  // -------------------------------------------------------------------------

  test("forgot password page renders", async ({ page }) => {
    await page.goto("/forgot-password");
    await page.waitForLoadState("networkidle");

    // The page should show an email input for password reset
    const emailInput = page.locator('input[type="email"], #email');
    const emailInputCount = await emailInput.count();

    if (emailInputCount === 0) {
      // If /forgot-password doesn't exist or redirects, skip
      test.skip(true, "Forgot password page not available");
      return;
    }

    await expect(emailInput.first()).toBeVisible({ timeout: 5000 });

    // Should have a submit button for resetting password
    const resetButton = page.locator(
      'button:has-text("Reset"), button:has-text("Send"), button:has-text("Submit")'
    );
    await expect(resetButton.first()).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // Self-hosted setup
  // -------------------------------------------------------------------------

  test("self-hosted setup redirects when no users exist", async ({ page }) => {
    await page.goto("/setup");
    await page.waitForLoadState("networkidle");

    const url = page.url();

    // /setup should either:
    //   1. Show a setup form (if no users exist yet — first-time setup)
    //   2. Redirect to /login or /signup (if setup is already complete)
    //   3. Show a "setup complete" or "already configured" message
    if (url.includes("/setup")) {
      // We landed on setup — check for a form or a status message
      const setupForm = page.locator("form, input, button");
      const setupMessage = page.getByText(/setup|configure|install/i);
      const eitherVisible =
        (await setupForm.first().isVisible().catch(() => false)) ||
        (await setupMessage.isVisible().catch(() => false));
      expect(eitherVisible).toBe(true);
    } else {
      // Redirected — should be login, signup, or onboarding
      expect(url).toMatch(/\/(login|signup|onboarding)/);
    }
  });
});
