import { Page, expect } from "@playwright/test";

/**
 * Page object for authentication flows.
 *
 * Selectors are derived from the actual DOM:
 *   - Login: #email, #password, button "Sign in"
 *   - Signup: #orgName, #name, #email, #password, button "Create account"
 *   - Errors: div[role="alert"]
 */
export class AuthPage {
  constructor(private page: Page) {}

  // ---------------------------------------------------------------------------
  // Navigation
  // ---------------------------------------------------------------------------

  async gotoLogin() {
    await this.page.goto("/login");
    // Wait for the form to be interactive (login page fetches /api/setup/status)
    await this.page.waitForSelector("#email");
  }

  async gotoSignup() {
    await this.page.goto("/signup");
    // Signup page fetches /api/setup/status before rendering the form.
    // Wait for either the form OR the "blocked" card to appear.
    await this.page.waitForSelector("#email, h2:has-text('Registration closed')", {
      timeout: 10000,
    });
  }

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  async signup(email: string, password: string, orgName: string, name = "Test User") {
    await this.gotoSignup();
    await this.page.fill("#orgName", orgName);
    await this.page.fill("#name", name);
    await this.page.fill("#email", email);
    await this.page.fill("#password", password);
    await this.page.click('button:has-text("Create account")');
  }

  async login(email: string, password: string) {
    await this.gotoLogin();
    await this.page.fill("#email", email);
    await this.page.fill("#password", password);
    await this.page.click('button:has-text("Sign in")');
  }

  // ---------------------------------------------------------------------------
  // Assertions
  // ---------------------------------------------------------------------------

  /** After login, the user should be redirected away from the login page. */
  async expectLoggedIn() {
    await expect(this.page).not.toHaveURL(/\/login/, { timeout: 10000 });
  }

  /** After signup, the user should land on /onboarding or /verify-email. */
  async expectSignedUp() {
    await expect(this.page).toHaveURL(/\/(onboarding|verify-email)/, {
      timeout: 10000,
    });
  }

  /** Expect an inline error alert to be visible (optionally matching text). */
  async expectError(message?: string | RegExp) {
    const alert = this.page.locator('div[role="alert"]');
    await expect(alert).toBeVisible({ timeout: 5000 });
    if (message) {
      await expect(alert).toContainText(
        typeof message === "string" ? message : message,
      );
    }
  }

  /** Assert that no error alert is visible on the page. */
  async expectNoError() {
    await expect(this.page.locator('div[role="alert"]')).not.toBeVisible();
  }
}
