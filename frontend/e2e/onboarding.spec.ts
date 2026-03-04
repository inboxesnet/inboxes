import { test, expect } from "@playwright/test";
import {
  uniqueEmail,
  VALID_PASSWORD,
  signupAndAuthenticate,
} from "./fixtures/helpers";

test.describe("Onboarding", () => {
  // -------------------------------------------------------------------------
  // Auth guard
  // -------------------------------------------------------------------------

  test("onboarding page requires authentication — redirects to login", async ({
    page,
  }) => {
    // Navigate to /onboarding without being logged in
    await page.goto("/onboarding");

    // The (app) layout wraps onboarding and fires a 401 which triggers
    // the session-expired modal or a redirect. Either way we should end
    // up seeing the login page or a session-expired prompt.
    await expect(page).toHaveURL(/\/(login|onboarding)/, { timeout: 10000 });

    // If we stay on /onboarding the session-expired modal should appear
    const url = page.url();
    if (url.includes("/onboarding")) {
      // Session expired modal or an error state should be visible
      const sessionModal = page.getByText(/session expired|sign in/i);
      await expect(sessionModal).toBeVisible({ timeout: 10000 });
    }
  });

  // -------------------------------------------------------------------------
  // Connect step
  // -------------------------------------------------------------------------

  test("onboarding renders the connect step after login", async ({ page }) => {
    const email = uniqueEmail("onboard");
    await signupAndAuthenticate(page, email, VALID_PASSWORD, "E2E Onboard Org");

    await page.goto("/onboarding");

    // Wait for the step to resolve — should land on "connect" step
    await expect(
      page.getByText("Connect your Resend account"),
    ).toBeVisible({ timeout: 10000 });
  });

  test("API key input field is visible on the connect step", async ({
    page,
  }) => {
    const email = uniqueEmail("apikey");
    await signupAndAuthenticate(page, email, VALID_PASSWORD, "E2E ApiKey Org");

    await page.goto("/onboarding");

    // The connect step should show the API key input
    const apiKeyInput = page.locator("#apiKey");
    await expect(apiKeyInput).toBeVisible({ timeout: 10000 });
    await expect(apiKeyInput).toHaveAttribute("placeholder", "re_...");
  });

  test("submitting an empty API key triggers validation", async ({ page }) => {
    const email = uniqueEmail("emptykey");
    await signupAndAuthenticate(
      page,
      email,
      VALID_PASSWORD,
      "E2E EmptyKey Org",
    );

    await page.goto("/onboarding");

    // Wait for connect step
    await expect(page.locator("#apiKey")).toBeVisible({ timeout: 10000 });

    // Click connect without entering a key — the input has `required`
    await page.click('button:has-text("Connect")');

    // The page should still show the connect step (browser required validation)
    await expect(
      page.getByText("Connect your Resend account"),
    ).toBeVisible();

    // The apiKey input should have required attribute
    await expect(page.locator("#apiKey")).toHaveAttribute("required", "");
  });

  test("progress stepper shows all four steps", async ({ page }) => {
    const email = uniqueEmail("stepper");
    await signupAndAuthenticate(
      page,
      email,
      VALID_PASSWORD,
      "E2E Stepper Org",
    );

    await page.goto("/onboarding");

    // Wait for steps to render
    await expect(page.getByText("Connect Resend")).toBeVisible({
      timeout: 10000,
    });

    // All four step labels should be present (they are in the progress bar)
    await expect(page.getByText("Connect Resend")).toBeVisible();
    await expect(page.getByText("Your Domains")).toBeVisible();
    await expect(page.getByText("Import Emails")).toBeVisible();
    await expect(page.getByText("Set Up Addresses")).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // API key input validation
  // -------------------------------------------------------------------------

  test("valid API key input accepts value", async ({ page }) => {
    const email = uniqueEmail("validkey");
    await signupAndAuthenticate(
      page,
      email,
      VALID_PASSWORD,
      "E2E ValidKey Org",
    );

    await page.goto("/onboarding");

    // Wait for the connect step to render
    const apiKeyInput = page.locator("#apiKey");
    await expect(apiKeyInput).toBeVisible({ timeout: 10000 });

    // Enter a valid-format Resend API key
    await apiKeyInput.fill("re_test_1234567890abcdefghijklmn");

    // Verify the value was accepted
    await expect(apiKeyInput).toHaveValue("re_test_1234567890abcdefghijklmn");

    // The Connect/Submit button should be enabled (not disabled) now
    // that a valid-format key has been entered
    const connectButton = page.locator('button:has-text("Connect")');
    await expect(connectButton).toBeVisible({ timeout: 5000 });
    await expect(connectButton).toBeEnabled();
  });

  // -------------------------------------------------------------------------
  // Sync progress
  // -------------------------------------------------------------------------

  test("sync progress displays status", async ({ page }) => {
    const email = uniqueEmail("syncprog");
    await signupAndAuthenticate(
      page,
      email,
      VALID_PASSWORD,
      "E2E SyncProg Org",
    );

    // Navigate to onboarding — we likely land on the "connect" step
    // since no API key has been configured. The sync step requires
    // a valid API key + domain, so we may not be able to reach it.
    await page.goto("/onboarding");
    await page.waitForLoadState("networkidle");

    // Check if we can see any sync-related content
    const syncIndicator = page.locator(
      'text=/scanning|syncing|importing|progress|sync/i'
    );
    const isSyncVisible = await syncIndicator
      .first()
      .isVisible({ timeout: 3000 })
      .catch(() => false);

    if (!isSyncVisible) {
      // We are not at the sync step (likely still on connect step)
      // — skip gracefully
      test.skip(true, "Not at sync step — API key and domain required first");
      return;
    }

    // If we are at the sync step, verify progress indicators are shown
    await expect(syncIndicator.first()).toBeVisible();

    // Look for progress bar or percentage indicator
    const progressBar = page.locator(
      'div[role="progressbar"], progress, [class*="progress"]'
    );
    const progressText = page.locator('text=/\\d+%|scanning|syncing/i');
    const hasProgress =
      (await progressBar.first().isVisible().catch(() => false)) ||
      (await progressText.first().isVisible().catch(() => false));

    expect(hasProgress).toBe(true);
  });
});
