import { Page, expect } from "@playwright/test";

/**
 * Page object for the floating compose window.
 *
 * Selectors derived from FloatingComposeWindow component:
 *   - Desktop compose: div[role="dialog"][aria-label="Compose email"] (hidden md:flex)
 *   - Title bar text: "New Message"
 *   - Minimize button: button[title="Minimize"]
 *   - Close button: button[title="Close"]
 *   - From label / field: label "From"
 *   - To label / RecipientInput: label "To", inner input[role="combobox"]
 *   - Cc Bcc toggle: button with text "Cc Bcc"
 *   - Cc/Bcc fields appear after toggle
 *   - Subject label "Subj", inner <input> with placeholder "Subject"
 *   - TipTap editor: div.ProseMirror[contenteditable="true"]
 *   - Send button: button with text "Send"
 *   - Minimized bar: fixed div with "New Message" text at bottom-right
 *   - Restore from minimized: button[aria-label="Restore compose window"]
 */
export class ComposePage {
  constructor(private page: Page) {}

  /** The desktop compose dialog locator. */
  private get dialog() {
    return this.page.locator('div[role="dialog"][aria-label="Compose email"]').last();
  }

  /** Click the Compose button in the sidebar (desktop). */
  async openCompose() {
    // The sidebar compose button contains a PenSquare icon + "Compose" text.
    // On desktop the sidebar is always visible.
    await this.page.locator("button:has-text('Compose')").first().click();
    // Wait for the dialog to appear
    await expect(this.dialog).toBeVisible({ timeout: 5000 });
  }

  /** Fill the "To" recipient field by typing an email and pressing Enter. */
  async fillTo(email: string) {
    const toInput = this.dialog.locator('input[role="combobox"]').first();
    await toInput.fill(email);
    await toInput.press("Enter");
  }

  /** Fill the Subject input. */
  async fillSubject(subject: string) {
    const subjectInput = this.dialog.locator('input[placeholder="Subject"]');
    await subjectInput.fill(subject);
  }

  /** Get the current value of the Subject input. */
  async getSubject(): Promise<string> {
    const subjectInput = this.dialog.locator('input[placeholder="Subject"]');
    return subjectInput.inputValue();
  }

  /** Fill the TipTap rich-text editor body. */
  async fillBody(text: string) {
    const editor = this.dialog.locator("div.ProseMirror[contenteditable='true']");
    await editor.click();
    await editor.fill(text);
  }

  /** Click the Send button. */
  async send() {
    await this.dialog.locator("button:has-text('Send')").click();
  }

  /** Click the Minimize button (desktop title bar). */
  async minimize() {
    await this.dialog.locator('button[title="Minimize"]').click();
  }

  /** Click the Close button (desktop title bar). */
  async close() {
    await this.dialog.locator('button[title="Close"]').click();
  }

  /** Click the "Cc Bcc" toggle button to reveal Cc and Bcc fields. */
  async toggleCcBcc() {
    await this.dialog.locator("button:has-text('Cc Bcc')").click();
  }

  /** Restore the compose window from its minimized bar. */
  async restoreFromMinimized() {
    // The minimized bar has aria-label="Restore compose window"
    await this.page.locator('button[aria-label="Restore compose window"]').click();
  }

  /** Assert the compose window is open and visible (desktop). */
  async expectOpen() {
    await expect(this.dialog).toBeVisible({ timeout: 5000 });
    await expect(this.dialog.getByText("New Message")).toBeVisible();
  }

  /** Assert the compose window is not visible. */
  async expectClosed() {
    // The desktop compose dialog should not be visible
    await expect(
      this.page.locator('div[role="dialog"][aria-label="Compose email"]').last(),
    ).not.toBeVisible({ timeout: 5000 });
  }

  /** Assert the minimized bar is visible. */
  async expectMinimized() {
    await expect(
      this.page.locator('button[aria-label="Restore compose window"]'),
    ).toBeVisible({ timeout: 5000 });
  }

  /** Assert From label row is visible. */
  async expectFromVisible() {
    await expect(this.dialog.locator("label", { hasText: "From" })).toBeVisible();
  }

  /** Assert To label row is visible. */
  async expectToVisible() {
    await expect(this.dialog.locator("label", { hasText: "To" })).toBeVisible();
  }

  /** Assert Subject label row is visible. */
  async expectSubjectVisible() {
    await expect(this.dialog.locator("label", { hasText: "Subj" })).toBeVisible();
  }

  /** Assert the TipTap editor is visible. */
  async expectEditorVisible() {
    await expect(
      this.dialog.locator("div.ProseMirror[contenteditable='true']"),
    ).toBeVisible();
  }

  /** Assert the Cc input row is visible. */
  async expectCcVisible() {
    await expect(this.dialog.locator("label", { hasText: "Cc" })).toBeVisible();
  }

  /** Assert the Bcc input row is visible. */
  async expectBccVisible() {
    await expect(this.dialog.locator("label", { hasText: "Bcc" })).toBeVisible();
  }

  /** Assert a recipient chip is displayed with the given email. */
  async expectRecipientChip(email: string) {
    await expect(this.dialog.getByText(email)).toBeVisible();
  }
}
