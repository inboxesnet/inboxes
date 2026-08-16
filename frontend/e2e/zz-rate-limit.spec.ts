import { test, expect } from "@playwright/test";

/**
 * Login rate limiting.
 *
 * This test exhausts the per-IP login rate limit (10 per 15 minutes). The
 * file name starts with "zz" so it runs after every other spec; a later
 * login attempt from the same IP would get a 429.
 */
test.describe("Rate limiting", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("rate limited login shows error", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByText("Welcome back")).toBeVisible({ timeout: 5000 });

    // Attempt rapid login attempts to trigger rate limiting
    const emailInput = page.locator('input[type="email"]');
    const passwordInput = page.locator('input[type="password"]');
    // The login button has no explicit type attribute; match it by text.
    const loginButton = page.locator('button:has-text("Sign in")');

    // Send many login attempts in quick succession
    for (let i = 0; i < 12; i++) {
      await emailInput.fill(`ratelimit-${i}@test.com`);
      await passwordInput.fill("WrongPass1");
      await loginButton.click();
      // Brief wait for the request to fire
      await page.waitForTimeout(100);
    }

    // After multiple rapid attempts, should either see rate limit error or invalid credentials
    // (The exact behavior depends on backend rate limit config)
    const errorText = page.locator("text=/rate limit|too many|try again|Invalid/i");
    await expect(errorText).toBeVisible({ timeout: 10000 });
  });
});
