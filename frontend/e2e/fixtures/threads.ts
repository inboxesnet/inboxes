import { Page, expect } from "@playwright/test";

/**
 * Page object for thread list and thread view interactions.
 *
 * Selectors derived from actual components:
 *
 * ThreadListPage (thread-list-page.tsx):
 *   - Search input: input[placeholder*="Search"]
 *   - Clear search: button with X icon next to search
 *   - Empty state: "Your inbox is empty", "No sent messages", etc.
 *   - Search no results: "No results found for"
 *   - Thread list: div[role="list"][aria-label="Email threads"]
 *
 * ThreadList (thread-list.tsx):
 *   - Thread rows: div[role="listitem"] containing subject text
 *   - Star button: button[aria-label="Star"] / button[aria-label="Unstar"]
 *   - Archive button: button[aria-label="Archive"]
 *   - Trash button: button[aria-label="Trash"]
 *   - Read/unread: button[aria-label="Mark as read"] / button[aria-label="Mark as unread"]
 *   - Unread indicator: font-semibold class on subject span
 *
 * ThreadView (thread-view.tsx):
 *   - Header: h2 with thread subject
 *   - Star button: button[title="Star"] / button[title="Unstar"]
 *   - Archive button: button[title="Archive"]
 *   - Trash button: button[title="Trash"]
 *   - Back button: ArrowLeft icon button
 *   - Reply bar: button with "Reply" text
 *   - Individual emails: expanded EmailMessage cards (div.rounded-lg.border)
 *   - Email from address: displayed in expanded email header
 */
export class ThreadsPage {
  constructor(private page: Page) {}

  // ---------------------------------------------------------------------------
  // Navigation
  // ---------------------------------------------------------------------------

  /** Navigate to a specific folder for a domain. */
  async navigateToFolder(domainId: string, folder: string) {
    await this.page.goto(`/d/${domainId}/${folder}`);
    await this.page.waitForLoadState("networkidle");
  }

  /** Navigate to inbox for a domain. */
  async navigateToInbox(domainId: string) {
    await this.navigateToFolder(domainId, "inbox");
  }

  // ---------------------------------------------------------------------------
  // Thread list interactions
  // ---------------------------------------------------------------------------

  /** Click a thread row by its subject text. */
  async clickThread(subject: string) {
    await this.page.locator(`div[role="listitem"]`, { hasText: subject }).first().click();
  }

  /** Assert a thread with the given subject is visible in the list. */
  async expectThreadVisible(subject: string) {
    await expect(
      this.page.locator(`div[role="listitem"]`, { hasText: subject }).first(),
    ).toBeVisible({ timeout: 10000 });
  }

  /** Assert a thread with the given subject is NOT visible in the list. */
  async expectThreadNotVisible(subject: string) {
    await expect(
      this.page.locator(`div[role="listitem"]`, { hasText: subject }),
    ).not.toBeVisible({ timeout: 10000 });
  }

  /** Assert the thread list container is visible. */
  async expectThreadListVisible() {
    await expect(
      this.page.locator('div[role="list"][aria-label="Email threads"]'),
    ).toBeVisible({ timeout: 10000 });
  }

  /** Assert the empty state message is shown for the current folder. */
  async expectEmptyState(message: string) {
    await expect(this.page.getByText(message)).toBeVisible({ timeout: 10000 });
  }

  /** Get the count of thread rows currently visible. */
  async getThreadCount(): Promise<number> {
    return this.page.locator('div[role="listitem"]').count();
  }

  // ---------------------------------------------------------------------------
  // Thread view interactions (inside the detail pane / full page)
  // ---------------------------------------------------------------------------

  /** Assert the thread view header shows the given subject. */
  async expectThreadViewSubject(subject: string) {
    await expect(this.page.locator("h2", { hasText: subject })).toBeVisible({
      timeout: 10000,
    });
  }

  /** Assert individual email messages are visible in the thread view. */
  async expectEmailsVisible() {
    // Expanded emails render as div.rounded-lg.border with email content
    await expect(
      this.page.locator("div.rounded-lg.border").first(),
    ).toBeVisible({ timeout: 10000 });
  }

  /** Assert the Reply button is visible in the thread view reply bar. */
  async expectReplyButtonVisible() {
    // The reply bar at the bottom has explicit "Reply" button
    await expect(
      this.page.locator("button:has-text('Reply')").first(),
    ).toBeVisible({ timeout: 5000 });
  }

  /** Click the back button (ArrowLeft) to return to thread list. */
  async goBack() {
    // The back button is rendered by ThreadView when onBack is provided
    await this.page.locator("button", { has: this.page.locator('svg.lucide-arrow-left') }).click();
  }

  // ---------------------------------------------------------------------------
  // Thread actions (in thread view header toolbar)
  // ---------------------------------------------------------------------------

  /** Star or unstar the currently open thread. */
  async starThread() {
    // In thread view header: button[title="Star"] or button[title="Unstar"]
    const starBtn = this.page.locator('button[title="Star"], button[title="Unstar"]');
    await starBtn.click();
  }

  /** Archive the currently open thread. */
  async archiveThread() {
    await this.page.locator('button[title="Archive"]').click();
  }

  /** Trash the currently open thread. */
  async trashThread() {
    await this.page.locator('button[title="Trash"]').click();
  }

  /** Mark the currently open thread as read or unread. */
  async toggleReadStatus() {
    const btn = this.page.locator(
      'button[title="Mark read"], button[title="Mark unread"]',
    );
    await btn.click();
  }

  // ---------------------------------------------------------------------------
  // Sidebar folder navigation
  // ---------------------------------------------------------------------------

  /** Click a folder/label in the sidebar by its visible text. */
  async clickSidebarFolder(label: string) {
    // Desktop sidebar label buttons contain the label text
    await this.page.locator(`button:has-text("${label}")`).first().click();
  }

  // ---------------------------------------------------------------------------
  // Search
  // ---------------------------------------------------------------------------

  /** Type into the search input and submit. */
  async search(query: string) {
    const searchInput = this.page.locator('input[placeholder*="Search"]');
    await searchInput.fill(query);
    await searchInput.press("Enter");
  }

  /** Clear the search by clicking the X button. */
  async clearSearch() {
    // The clear button is next to the search input, only visible when searchQuery is set
    const clearBtn = this.page.locator("form").locator("button").filter({ has: this.page.locator('svg.lucide-x') });
    if (await clearBtn.isVisible()) {
      await clearBtn.click();
    } else {
      // Fallback: clear input and submit empty
      const searchInput = this.page.locator('input[placeholder*="Search"]');
      await searchInput.fill("");
      await searchInput.press("Enter");
    }
  }

  /** Assert the search no-results message is visible. */
  async expectNoSearchResults(query: string) {
    await expect(
      this.page.getByText(`No results found for`),
    ).toBeVisible({ timeout: 10000 });
  }

  /** Assert the search input is visible. */
  async expectSearchInputVisible() {
    await expect(
      this.page.locator('input[placeholder*="Search"]'),
    ).toBeVisible({ timeout: 5000 });
  }
}
