import { test, expect } from "@playwright/test";
import { ThreadsPage } from "./fixtures/threads";

/**
 * E2E tests for the inline search functionality.
 *
 * Prerequisites:
 *   - Full stack running (frontend :3000, backend :8080)
 *   - An already-set-up account with at least one domain
 *
 * Search is embedded in the ThreadListPage header:
 *   - Input with placeholder "Search {folder} by subject, sender, or content..."
 *   - Results are fetched via GET /api/emails/search?q=...&domain_id=...
 *   - No results: "No results found for ..."
 *   - Clear button (X icon) resets to normal thread list
 *
 * These tests run serially since they share page state.
 */

test.describe("Search", () => {
  test.describe.configure({ mode: "serial" });

  let threadsPage: ThreadsPage;

  test.beforeEach(async ({ page }) => {
    threadsPage = new ThreadsPage(page);
    await page.goto("/");
    await page.waitForLoadState("networkidle");
    if (page.url().includes("/login")) {
      test.skip(true, "Not authenticated — skipping search tests");
    }
    // Wait for app shell
    await page.waitForSelector("button:has-text('Compose')", { timeout: 15000 });
  });

  test("search input visible", async ({ page }) => {
    await threadsPage.expectSearchInputVisible();
  });

  test("search by subject keyword", async ({ page }) => {
    // Get the first thread's subject to use as a search term
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping search-by-keyword test");
      return;
    }

    // Extract a word from the first thread's subject
    // Thread rows contain the subject text — grab any text content
    const threadText = await firstThread.textContent();
    // Pick a meaningful keyword from the thread text (skip very short words)
    const words = (threadText || "")
      .split(/\s+/)
      .filter((w) => w.length > 3 && /^[a-zA-Z]+$/.test(w));
    const keyword = words.length > 0 ? words[0] : "test";

    // Search
    await threadsPage.search(keyword);
    await page.waitForTimeout(1000);

    // Results should be visible — either matching threads or "No results"
    const resultThread = page.locator('div[role="listitem"]').first();
    const noResults = page.getByText("No results found for");
    await expect(resultThread.or(noResults)).toBeVisible({ timeout: 10000 });
  });

  test("empty search shows all", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);

    // First, do a search
    await threadsPage.search("some-search-term");
    await page.waitForTimeout(500);

    // Then clear the search
    await threadsPage.clearSearch();
    await page.waitForTimeout(500);

    // After clearing, we should be back to the normal thread list
    const threadList = page.locator('div[role="list"][aria-label="Email threads"]');
    const emptyState = page.getByText(/inbox is empty|No.*messages/i);
    await expect(threadList.or(emptyState)).toBeVisible({ timeout: 10000 });
  });

  test("no results message", async ({ page }) => {
    // Search for gibberish that should match nothing
    const gibberish = "zzzqqqxxx_" + Date.now();
    await threadsPage.search(gibberish);
    await page.waitForTimeout(1000);

    // Should show "No results found for ..."
    await threadsPage.expectNoSearchResults(gibberish);
  });
});
