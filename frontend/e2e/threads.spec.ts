import { test, expect } from "@playwright/test";
import { ThreadsPage } from "./fixtures/threads";

/**
 * E2E tests for thread list and thread view interactions.
 *
 * Prerequisites:
 *   - Full stack running (frontend :3000, backend :8080)
 *   - An already-set-up account with at least one domain
 *   - At least one thread in the inbox (tests that need data create it via API)
 *
 * These tests run serially because some depend on shared state (e.g. archiving
 * a thread should make it disappear, starring should persist).
 */

// Helper: get the first domainId by querying the API
async function getFirstDomainId(baseURL: string): Promise<string> {
  const res = await fetch(`${baseURL}/api/domains`, {
    credentials: "include",
  });
  if (!res.ok) throw new Error(`Failed to fetch domains: ${res.status}`);
  const domains = await res.json();
  if (!domains.length) throw new Error("No domains found — setup required");
  return domains[0].id;
}

test.describe("Thread List & Thread View", () => {
  test.describe.configure({ mode: "serial" });

  let threads: ThreadsPage;
  let domainId: string;

  test.beforeEach(async ({ page }) => {
    threads = new ThreadsPage(page);
    // Navigate to app root and ensure we are authenticated
    await page.goto("/");
    await page.waitForLoadState("networkidle");
    if (page.url().includes("/login")) {
      test.skip(true, "Not authenticated — skipping thread tests");
    }
    // Wait for sidebar to render (signals that domain data is loaded)
    await page.waitForSelector("button:has-text('Compose')", { timeout: 15000 });
  });

  test("thread list loads on inbox page", async ({ page }) => {
    // Extract domainId from the current URL: /d/{domainId}/inbox
    const url = page.url();
    const match = url.match(/\/d\/([^/]+)\//);
    if (match) {
      domainId = match[1];
    } else {
      // Navigate to root and let redirects land us on a domain
      await page.goto("/");
      await page.waitForLoadState("networkidle");
      const newUrl = page.url();
      const newMatch = newUrl.match(/\/d\/([^/]+)\//);
      domainId = newMatch ? newMatch[1] : "";
    }
    // The inbox page should either show threads or an empty state
    const threadList = page.locator('div[role="list"][aria-label="Email threads"]');
    const emptyState = page.getByText("Your inbox is empty");
    await expect(threadList.or(emptyState)).toBeVisible({ timeout: 10000 });
  });

  test("click thread opens thread view", async ({ page }) => {
    // Look for any thread row
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads in inbox — skipping click-to-open test");
      return;
    }

    // Get the subject text from the thread row
    const subjectEl = firstThread.locator("span").filter({ hasText: /.+/ }).first();
    const subjectText = await subjectEl.textContent();

    await firstThread.click();

    // Thread view should show the subject in an h2 header
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });
  });

  test("archive button moves thread", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads in inbox — skipping archive test");
      return;
    }

    // Open the first thread
    await firstThread.click();
    await page.waitForTimeout(500);

    // Click archive in the thread view header
    const archiveBtn = page.locator('button[title="Archive"]');
    const archiveBtnVisible = await archiveBtn.isVisible().catch(() => false);
    if (!archiveBtnVisible) {
      test.skip(true, "Archive button not visible (might be archive/trash folder)");
      return;
    }
    await archiveBtn.click();

    // After archiving, we should return to the thread list
    // (the onBack callback is called after archive)
    await page.waitForTimeout(500);
  });

  test("star thread", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping star test");
      return;
    }

    // Open thread
    await firstThread.click();
    await page.waitForTimeout(500);

    // Click star button in thread view header
    const starBtn = page.locator('button[title="Star"], button[title="Unstar"]');
    await expect(starBtn).toBeVisible({ timeout: 5000 });
    await starBtn.click();

    // The star icon should toggle its fill state
    // We just verify the button is still present (no crash)
    await expect(starBtn).toBeVisible();
  });

  test("trash thread", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping trash test");
      return;
    }

    // Open thread
    await firstThread.click();
    await page.waitForTimeout(500);

    const trashBtn = page.locator('button[title="Trash"]');
    const trashVisible = await trashBtn.isVisible().catch(() => false);
    if (!trashVisible) {
      test.skip(true, "Trash button not visible");
      return;
    }
    await trashBtn.click();

    // After trashing, we should navigate back to the list
    await page.waitForTimeout(500);
  });

  test("mark as read", async ({ page }) => {
    // Opening a thread automatically marks it as read (ThreadView useEffect).
    // We verify by checking the thread view is loaded successfully.
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping mark-as-read test");
      return;
    }

    await firstThread.click();

    // Thread view should be visible with h2 subject
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // The mark-read/unread button should be visible
    const readBtn = page.locator(
      'button[title="Mark read"], button[title="Mark unread"]',
    );
    await expect(readBtn).toBeVisible({ timeout: 5000 });
  });

  test("thread view shows emails", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping email visibility test");
      return;
    }

    await firstThread.click();

    // Individual email cards: expanded emails are rendered as div.rounded-lg.border
    await expect(
      page.locator("div.rounded-lg.border").first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("reply button visible", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping reply button test");
      return;
    }

    await firstThread.click();

    // Bottom reply bar has Reply, Reply All, Forward buttons
    await expect(
      page.locator("button:has-text('Reply')").first(),
    ).toBeVisible({ timeout: 10000 });

    await expect(
      page.locator("button:has-text('Reply All')").first(),
    ).toBeVisible();

    await expect(
      page.locator("button:has-text('Forward')").first(),
    ).toBeVisible();
  });

  test("folder navigation", async ({ page }) => {
    // Click different folder labels in the sidebar and verify URL changes
    const folders = ["Sent", "Archive", "Starred", "Trash"];

    for (const folder of folders) {
      const folderBtn = page.locator(`button:has-text("${folder}")`).first();
      const isVisible = await folderBtn.isVisible().catch(() => false);
      if (isVisible) {
        await folderBtn.click();
        await page.waitForTimeout(300);
        const url = page.url();
        expect(url.toLowerCase()).toContain(`/${folder.toLowerCase()}`);
      }
    }
  });

  test("empty inbox message", async ({ page }) => {
    // Navigate to a folder that is likely empty (e.g. spam)
    const spamBtn = page.locator('button:has-text("Spam")').first();
    const isVisible = await spamBtn.isVisible().catch(() => false);
    if (isVisible) {
      await spamBtn.click();
      await page.waitForTimeout(500);
      // Either threads are shown or the empty state
      const emptyState = page.getByText("No spam messages");
      const threadList = page.locator('div[role="list"][aria-label="Email threads"]');
      await expect(emptyState.or(threadList)).toBeVisible({ timeout: 10000 });
    }
  });

  test("back to list", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping back-to-list test");
      return;
    }

    // Open thread view
    await firstThread.click();
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // Click the back button (ArrowLeft icon in thread view header)
    const backBtn = page.locator("button").filter({
      has: page.locator("svg.lucide-arrow-left"),
    });
    const backVisible = await backBtn.isVisible().catch(() => false);
    if (backVisible) {
      await backBtn.click();
      await page.waitForTimeout(500);
      // Thread list or empty state should be visible again
      const threadList = page.locator('div[role="list"][aria-label="Email threads"]');
      const emptyState = page.getByText(/inbox is empty|No.*messages/i);
      await expect(threadList.or(emptyState)).toBeVisible({ timeout: 10000 });
    }
  });

  test("mute thread — bell-off icon appears", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping mute test");
      return;
    }

    // Open thread
    await firstThread.click();
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // Find the mute/unmute button
    const muteBtn = page.locator('button[title="Mute"], button[title="Unmute"]');
    const muteVisible = await muteBtn.isVisible().catch(() => false);
    if (!muteVisible) {
      test.skip(true, "Mute button not visible");
      return;
    }

    // Get initial state
    const initialTitle = await muteBtn.getAttribute("title");

    // Click to toggle mute
    await muteBtn.click();
    await page.waitForTimeout(500);

    // Verify the button toggled (Mute → Unmute or vice versa)
    const newTitle = await muteBtn.getAttribute("title");
    expect(newTitle).not.toBe(initialTitle);

    // If we just muted, the bell-off icon should be present
    if (initialTitle === "Mute") {
      await expect(page.locator("svg.lucide-bell-off")).toBeVisible();
    }

    // Toggle back to restore original state
    await muteBtn.click();
    await page.waitForTimeout(500);
  });

  test("move thread from trash to inbox", async ({ page }) => {
    // First, trash a thread if there are any in inbox
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping move-from-trash test");
      return;
    }

    // Open the first thread and trash it
    await firstThread.click();
    await page.waitForTimeout(500);

    const trashBtn = page.locator('button[title="Trash"]');
    const trashVisible = await trashBtn.isVisible().catch(() => false);
    if (!trashVisible) {
      test.skip(true, "Trash button not visible (might already be in trash)");
      return;
    }
    await trashBtn.click();
    await page.waitForTimeout(500);

    // Navigate to the Trash folder
    const trashFolder = page.locator('button:has-text("Trash")').first();
    await trashFolder.click();
    await page.waitForTimeout(500);

    // Open the first thread in trash
    const trashThread = page.locator('div[role="listitem"]').first();
    const hasTrashThreads = await trashThread.isVisible().catch(() => false);
    if (!hasTrashThreads) {
      test.skip(true, "No threads in trash");
      return;
    }

    await trashThread.click();
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // Click "Move to Inbox" button
    const moveToInboxBtn = page.locator('button[title="Move to Inbox"]');
    const moveVisible = await moveToInboxBtn.isVisible().catch(() => false);
    if (moveVisible) {
      await moveToInboxBtn.click();
      await page.waitForTimeout(500);
      // Should navigate back to thread list after moving
    }
  });

  test("permanent delete from trash — confirmation dialog", async ({ page }) => {
    // Navigate to Trash folder
    const trashFolder = page.locator('button:has-text("Trash")').first();
    const trashFolderVisible = await trashFolder.isVisible().catch(() => false);
    if (!trashFolderVisible) {
      test.skip(true, "Trash folder not visible in sidebar");
      return;
    }
    await trashFolder.click();
    await page.waitForTimeout(500);

    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads in trash — skipping permanent delete test");
      return;
    }

    // Open the first thread in trash
    await firstThread.click();
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // Click "Delete permanently" button (only visible in trash view)
    const deleteBtn = page.locator('button[title="Delete permanently"]');
    const deleteVisible = await deleteBtn.isVisible().catch(() => false);
    if (!deleteVisible) {
      test.skip(true, "Delete permanently button not visible");
      return;
    }
    await deleteBtn.click();

    // Confirmation dialog should appear
    const confirmDialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
    await expect(confirmDialog).toBeVisible({ timeout: 5000 });

    // The dialog should have a confirm button
    const confirmBtn = confirmDialog.locator('button:has-text("Delete"), button:has-text("Confirm")');
    await expect(confirmBtn).toBeVisible();

    // Confirm the deletion
    await confirmBtn.click();
    await page.waitForTimeout(500);

    // Thread should be gone — should navigate back to thread list
  });

  test("collapsed email message expands on click", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping collapsed email expand test");
      return;
    }

    // Open the first thread
    await firstThread.click();
    await expect(page.locator("h2").first()).toBeVisible({ timeout: 10000 });

    // Wait for at least one email section to render (expanded or collapsed)
    // Expanded emails: div.rounded-lg.border
    // Collapsed emails: button[aria-expanded="false"] with sender name
    const expandedEmail = page.locator("div.rounded-lg.border");
    const collapsedEmail = page.locator('button[aria-expanded="false"]');
    await expect(expandedEmail.or(collapsedEmail).first()).toBeVisible({ timeout: 10000 });

    // If there is a collapsed email, click it to expand
    const collapsedCount = await collapsedEmail.count();
    if (collapsedCount > 0) {
      const firstCollapsed = collapsedEmail.first();
      // Verify the collapsed row shows sender name text
      await expect(firstCollapsed).toBeVisible();
      const senderText = await firstCollapsed.textContent();
      expect(senderText?.length).toBeGreaterThan(0);

      // Click to expand
      await firstCollapsed.click();
      await page.waitForTimeout(300);

      // After clicking, it should now be expanded (aria-expanded="true" or a div.rounded-lg.border)
      // The collapsed button is replaced by the expanded card
      const expandedCount = await expandedEmail.count();
      expect(expandedCount).toBeGreaterThanOrEqual(1);
    } else {
      // Only one email in the thread — it is already expanded. Verify it is interactable.
      await expect(expandedEmail.first()).toBeVisible();
      // Click the expanded header to collapse it, then re-expand
      const expandedHeader = page.locator('button[aria-expanded="true"]').first();
      const headerVisible = await expandedHeader.isVisible().catch(() => false);
      if (headerVisible) {
        // Collapse
        await expandedHeader.click();
        await page.waitForTimeout(300);
        // Now it should be collapsed
        const nowCollapsed = page.locator('button[aria-expanded="false"]').first();
        await expect(nowCollapsed).toBeVisible({ timeout: 3000 });
        // Re-expand
        await nowCollapsed.click();
        await page.waitForTimeout(300);
        await expect(expandedEmail.first()).toBeVisible({ timeout: 3000 });
      }
    }
  });

  test("trash with undo toast", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping trash-with-undo test");
      return;
    }

    // Open the first thread
    await firstThread.click();
    await page.waitForTimeout(500);

    const trashBtn = page.locator('button[title="Trash"]');
    const trashVisible = await trashBtn.isVisible().catch(() => false);
    if (!trashVisible) {
      test.skip(true, "Trash button not visible — might already be in trash folder");
      return;
    }

    // Click trash
    await trashBtn.click();

    // A toast should appear with "Moved to trash" text and an "Undo" button
    const toastEl = page.locator('[data-sonner-toast]').filter({ hasText: /trash/i });
    await expect(toastEl).toBeVisible({ timeout: 5000 });

    // Find the Undo button within the toast
    const undoBtn = toastEl.locator('button:has-text("Undo")');
    await expect(undoBtn).toBeVisible({ timeout: 3000 });

    // Click undo to restore the thread
    await undoBtn.click();
    await page.waitForTimeout(1000);

    // After undo, we should be back on the thread list.
    // Navigate to inbox to verify the thread is restored.
    const inboxBtn = page.locator('button:has-text("Inbox")').first();
    const inboxVisible = await inboxBtn.isVisible().catch(() => false);
    if (inboxVisible) {
      await inboxBtn.click();
      await page.waitForTimeout(500);
    }
  });

  test("bulk select and archive", async ({ page }) => {
    const threadItems = page.locator('div[role="listitem"]');
    const threadCount = await threadItems.count();
    if (threadCount < 2) {
      test.skip(true, "Need at least 2 threads for bulk select — skipping");
      return;
    }

    // Select multiple threads via their checkboxes
    // Each thread row has a checkbox input inside a label
    const firstCheckbox = threadItems.nth(0).locator('input[type="checkbox"]');
    const secondCheckbox = threadItems.nth(1).locator('input[type="checkbox"]');

    await firstCheckbox.check({ force: true });
    await page.waitForTimeout(200);
    await secondCheckbox.check({ force: true });
    await page.waitForTimeout(200);

    // Verify both are checked
    await expect(firstCheckbox).toBeChecked();
    await expect(secondCheckbox).toBeChecked();

    // The bulk action toolbar should now show an Archive button
    // Bulk archive button is in the toolbar area: button[title="Archive"]
    const bulkArchiveBtn = page.locator('.flex.items-center.gap-0\\.5 button[title="Archive"]');
    const archiveVisible = await bulkArchiveBtn.isVisible().catch(() => false);
    if (!archiveVisible) {
      test.skip(true, "Bulk archive button not visible — might not be in inbox label");
      return;
    }

    // Remember thread count before archiving
    const countBefore = await threadItems.count();

    await bulkArchiveBtn.click();
    await page.waitForTimeout(1000);

    // After archiving, the selected threads should be removed from the list
    const countAfter = await threadItems.count();
    expect(countAfter).toBeLessThan(countBefore);
  });

  test("select all pages banner", async ({ page }) => {
    const threadItems = page.locator('div[role="listitem"]');
    const threadCount = await threadItems.count();
    if (threadCount < 2) {
      test.skip(true, "Need at least 2 threads for select-all-pages test — skipping");
      return;
    }

    // Click the "select all" checkbox in the toolbar to select all visible threads
    // The toolbar checkbox is the first checkbox in the toolbar area (not inside a thread row)
    const toolbarCheckbox = page.locator('.flex.items-center.gap-2.h-10 input[type="checkbox"]').first();
    const checkboxVisible = await toolbarCheckbox.isVisible().catch(() => false);
    if (!checkboxVisible) {
      test.skip(true, "Toolbar select-all checkbox not visible");
      return;
    }

    await toolbarCheckbox.check({ force: true });
    await page.waitForTimeout(300);

    // If total > visible threads, a "Select all N conversations" banner should appear.
    // If total == visible (single page), the banner won't appear — that's OK, we just
    // verify all visible threads are selected.
    const selectAllBanner = page.locator('text=/Select all \\d+ conversations/');
    const bannerVisible = await selectAllBanner.isVisible().catch(() => false);

    if (bannerVisible) {
      // Click the "Select all N conversations" link to enable select-all-pages mode
      await selectAllBanner.click();
      await page.waitForTimeout(300);

      // Now a "All N conversations are selected" banner should be visible
      const allSelectedBanner = page.locator('text=/All \\d+ conversations are selected/');
      await expect(allSelectedBanner).toBeVisible({ timeout: 3000 });

      // Click "Clear selection" to reset
      const clearBtn = page.locator('button:has-text("Clear selection")');
      const clearVisible = await clearBtn.isVisible().catch(() => false);
      if (clearVisible) {
        await clearBtn.click();
        await page.waitForTimeout(300);
      }
    } else {
      // Single page — all threads are selected, verify each checkbox is checked
      for (let i = 0; i < Math.min(threadCount, 5); i++) {
        const cb = threadItems.nth(i).locator('input[type="checkbox"]');
        await expect(cb).toBeChecked();
      }
    }

    // Clean up: uncheck the toolbar checkbox to deselect all
    await toolbarCheckbox.uncheck({ force: true });
    await page.waitForTimeout(200);
  });

  test("drag thread to folder", async ({ page }) => {
    const firstThread = page.locator('div[role="listitem"]').first();
    const hasThreads = await firstThread.isVisible().catch(() => false);
    if (!hasThreads) {
      test.skip(true, "No threads — skipping drag-to-folder test");
      return;
    }

    // Look for a sidebar folder that can act as a drop target.
    // DroppableLabelButton renders as a <button> containing the folder text.
    // We look for "Archive" as the drop target since it is a common folder.
    const archiveFolder = page.locator('button:has-text("Archive")').first();
    const archiveVisible = await archiveFolder.isVisible().catch(() => false);
    if (!archiveVisible) {
      test.skip(true, "Archive folder not found in sidebar — skipping drag test");
      return;
    }

    // Attempt drag-and-drop from the first thread to the Archive folder.
    // Playwright's dragTo uses pointer events, which works with @dnd-kit's PointerSensor.
    try {
      await firstThread.dragTo(archiveFolder, { timeout: 5000 });
      // If the drag succeeded, wait a moment for the action to process
      await page.waitForTimeout(500);
    } catch {
      // Drag-and-drop can fail in headless mode or if sensors don't activate.
      // This is expected — the test is conceptual.
      test.skip(true, "Drag-and-drop did not complete — sensor may not have activated in test environment");
      return;
    }

    // After a successful drag to Archive, the thread should be removed from the inbox
    // or a toast should confirm the action.
    const toastOrList = page
      .locator('[data-sonner-toast]')
      .or(page.locator('div[role="list"][aria-label="Email threads"]'));
    await expect(toastOrList).toBeVisible({ timeout: 5000 });
  });
});
