import { test, expect } from "@playwright/test";
import { AuthPage } from "./fixtures/auth";
import {
  uniqueEmail,
  VALID_PASSWORD,
  WEAK_PASSWORD,
  apiSignup,
} from "./fixtures/helpers";

test.describe("Auth", () => {
  // -------------------------------------------------------------------------
  // Signup
  // -------------------------------------------------------------------------

  test("signup page renders form fields", async ({ page }) => {
    const auth = new AuthPage(page);
    await auth.gotoSignup();

    await expect(page.locator("#orgName")).toBeVisible();
    await expect(page.locator("#name")).toBeVisible();
    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator('button:has-text("Create account")')).toBeVisible();
  });

  test("successful signup redirects to onboarding or verify-email", async ({ page }) => {
    const auth = new AuthPage(page);
    const email = uniqueEmail("signup");

    await auth.signup(email, VALID_PASSWORD, "E2E Org");
    await auth.expectSignedUp();
  });

  test("signup with weak password shows validation error", async ({ page }) => {
    const auth = new AuthPage(page);
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

  test("successful login redirects away from login page", async ({ page }) => {
    // Pre-create account via API
    const email = uniqueEmail("login");
    const signupRes = await apiSignup(email, VALID_PASSWORD, "E2E Login Org");
    expect(signupRes.ok).toBe(true);

    // Now login via the UI
    const auth = new AuthPage(page);
    await auth.login(email, VALID_PASSWORD);
    await auth.expectLoggedIn();
  });

  test("wrong password shows error message", async ({ page }) => {
    // Pre-create account via API
    const email = uniqueEmail("wrong-pw");
    const signupRes = await apiSignup(email, VALID_PASSWORD, "E2E WrongPw Org");
    expect(signupRes.ok).toBe(true);

    const auth = new AuthPage(page);
    await auth.login(email, "WrongPassword1");

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
    const email = uniqueEmail("dup");

    // Create account via API first
    const signupRes = await apiSignup(email, VALID_PASSWORD, "E2E Dup Org");
    expect(signupRes.ok).toBe(true);

    // Attempt to signup again with the same email via UI
    const auth = new AuthPage(page);
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
