import { Page, expect } from "@playwright/test";

/**
 * Page object for the Settings modal.
 *
 * Selectors derived from the actual SettingsModal component:
 *   - Modal wrapper: Dialog > DialogContent (role="dialog")
 *   - Sidebar heading: h3 "Settings"
 *   - Tab buttons: button[role="tab"] with aria-selected, id="tab-{key}"
 *   - Tab keys: profile, domains, team, aliases, labels, organization, billing, system, jobs
 *   - Tab panel: div[role="tabpanel"] id="panel-{key}"
 *   - Profile tab: CardTitle "Profile", input for Name, disabled input for Email
 *   - Password card: CardTitle "Password"
 *   - Domains tab: CardTitle "Domains" or domain rows
 *   - Team tab: CardTitle with team members, invite form
 *   - Aliases tab: Alias management
 *   - Labels tab: Label management
 *   - Billing tab: CardTitle "Subscription"
 *   - Organization tab: Organization settings
 *   - System tab: System email settings
 *   - Jobs tab: Email Jobs table
 *
 * Opening the modal:
 *   Desktop: click the Settings icon button (title="Settings") in top-right toolbar
 *   Mobile sidebar: button with text "Settings"
 */
export class SettingsPage {
  constructor(private page: Page) {}

  /** The settings modal dialog locator. */
  private get dialog() {
    return this.page.getByRole("dialog");
  }

  /** Open settings via the desktop top-right toolbar Settings button. */
  async openSettings() {
    await this.page.locator('button[title="Settings"]').click();
  }

  /** Open settings via the mobile sidebar Settings button. */
  async openSettingsMobile() {
    await this.page.locator("button:has-text('Settings')").click();
  }

  /** Assert the settings dialog is visible. */
  async expectOpen() {
    await expect(this.dialog).toBeVisible({ timeout: 5000 });
    await expect(this.dialog.locator("h3", { hasText: "Settings" })).toBeVisible();
  }

  /** Assert the settings dialog is not visible. */
  async expectClosed() {
    await expect(this.dialog).not.toBeVisible({ timeout: 5000 });
  }

  /** Close settings by pressing Escape. */
  async closeSettings() {
    await this.page.keyboard.press("Escape");
  }

  /**
   * Navigate to a specific settings tab by clicking the tab button.
   * @param tabName - The visible label text (e.g. "Profile", "Domains", "Team")
   */
  async navigateToTab(tabName: string) {
    await this.dialog.locator(`button[role="tab"]:has-text("${tabName}")`).click();
  }

  /** Assert a specific tab is currently active (aria-selected="true"). */
  async expectTabActive(tabKey: string) {
    await expect(
      this.dialog.locator(`button[role="tab"]#tab-${tabKey}`),
    ).toHaveAttribute("aria-selected", "true");
  }

  /** Assert the tab panel is visible for a given tab key. */
  async expectPanelVisible(tabKey: string) {
    await expect(
      this.dialog.locator(`div[role="tabpanel"]#panel-${tabKey}`),
    ).toBeVisible();
  }

  /** Get the tabpanel content locator. */
  get panel() {
    return this.dialog.locator('div[role="tabpanel"]');
  }

  /** Assert a tab button is visible by label text. */
  async expectTabVisible(tabName: string) {
    await expect(
      this.dialog.locator(`button[role="tab"]:has-text("${tabName}")`),
    ).toBeVisible();
  }

  /** Assert a tab button is NOT visible by label text. */
  async expectTabNotVisible(tabName: string) {
    await expect(
      this.dialog.locator(`button[role="tab"]:has-text("${tabName}")`),
    ).not.toBeVisible();
  }
}
