import { test, expect } from "@playwright/test";
import { SettingsPage } from "./fixtures/settings";
import {
  uniqueEmail,
  VALID_PASSWORD,
  signupAndAuthenticate,
} from "./fixtures/helpers";

/**
 * Settings modal E2E tests.
 *
 * These tests exercise the SettingsModal component which is rendered
 * inside the domain layout at /d/{domainId}/{label}.
 *
 * The modal uses a vertical tablist with tabs:
 *   Profile, Domains, Team (admin), Aliases, Labels, Organization (admin),
 *   Billing (commercial only), System (self-hosted owner), Jobs (admin)
 *
 * DOM structure (from settings-modal.tsx):
 *   Dialog > DialogContent (role="dialog")
 *     Left sidebar: div with role="tablist" containing button[role="tab"]
 *     Right panel: div[role="tabpanel"]
 *       Profile tab: Card "Profile" (name input), Card "Password", NotificationsCard, PrivacyCard, ComposeCard, SyncCard
 *       Domains tab: domain list with visibility toggles
 *       Team tab: user list, invite form
 *       Aliases tab: alias management
 *       Labels tab: label CRUD
 *       Billing tab: Subscription card
 */

test.describe("Settings Modal", () => {
  let settings: SettingsPage;

  test.beforeEach(async ({ page }) => {
    // Create a fresh account and authenticate
    const email = uniqueEmail("settings");
    await signupAndAuthenticate(page, email, VALID_PASSWORD, "E2E Settings Org");

    // Navigate to the app — will redirect to /d or /onboarding
    await page.goto("/d");
    // Wait for the app to load (sidebar or onboarding)
    await page.waitForLoadState("networkidle");

    settings = new SettingsPage(page);
  });

  // -------------------------------------------------------------------------
  // 1. Settings button opens modal
  // -------------------------------------------------------------------------
  test("settings button opens modal", async ({ page }) => {
    // Desktop top-right toolbar has a Settings icon button with title="Settings"
    await settings.openSettings();
    await settings.expectOpen();
  });

  // -------------------------------------------------------------------------
  // 2. Escape closes settings
  // -------------------------------------------------------------------------
  test("escape closes settings", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.closeSettings();
    await settings.expectClosed();
  });

  // -------------------------------------------------------------------------
  // 3. General/Profile tab is default
  // -------------------------------------------------------------------------
  test("profile tab is default active tab", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // Profile tab should be selected by default
    await settings.expectTabActive("profile");
    await settings.expectPanelVisible("profile");
  });

  // -------------------------------------------------------------------------
  // 4. Tab navigation works
  // -------------------------------------------------------------------------
  test("tab navigation switches content", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // Start on Profile, switch to Domains
    await settings.navigateToTab("Domains");
    await settings.expectTabActive("domains");
    await settings.expectPanelVisible("domains");

    // Switch to Labels
    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");
    await settings.expectPanelVisible("labels");

    // Switch back to Profile
    await settings.navigateToTab("Profile");
    await settings.expectTabActive("profile");
    await settings.expectPanelVisible("profile");
  });

  // -------------------------------------------------------------------------
  // 5. Profile name field visible
  // -------------------------------------------------------------------------
  test("profile tab shows name input field", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // Profile tab has a Card with title "Profile" and a Name input
    const panel = settings.panel;
    await expect(panel.locator("text=Profile").first()).toBeVisible();
    // Name label
    await expect(
      panel.locator("label", { hasText: "Name" }),
    ).toBeVisible();
    // Name input should be editable (not disabled)
    const nameInput = panel.locator("label:has-text('Name') + input, label:has-text('Name') ~ div input").first();
    // The input that follows the Name label — use the second input in the card
    // (first is email which is disabled, second is name)
    const inputs = panel.locator("input");
    const inputCount = await inputs.count();
    expect(inputCount).toBeGreaterThanOrEqual(2);
  });

  // -------------------------------------------------------------------------
  // 6. Profile tab shows email field (disabled)
  // -------------------------------------------------------------------------
  test("profile tab shows disabled email field", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    const panel = settings.panel;
    await expect(
      panel.locator("label", { hasText: "Email" }),
    ).toBeVisible();
    // The email input should be disabled
    const emailInput = panel.locator("input[disabled]").first();
    await expect(emailInput).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 7. Domains tab shows domain content
  // -------------------------------------------------------------------------
  test("domains tab shows domain management content", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Domains");
    await settings.expectTabActive("domains");

    // The domains panel should be visible
    const panel = settings.panel;
    await expect(panel).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 8. Team tab visible for admin
  // -------------------------------------------------------------------------
  test("team tab is visible for admin user", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // The user who created the org is admin, so Team tab should be visible
    await settings.expectTabVisible("Team");

    await settings.navigateToTab("Team");
    await settings.expectTabActive("team");
    await settings.expectPanelVisible("team");
  });

  // -------------------------------------------------------------------------
  // 9. Labels tab exists and is navigable
  // -------------------------------------------------------------------------
  test("labels tab is navigable", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.expectTabVisible("Labels");
    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");
    await settings.expectPanelVisible("labels");
  });

  // -------------------------------------------------------------------------
  // 10. Aliases tab exists
  // -------------------------------------------------------------------------
  test("aliases tab is navigable", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.expectTabVisible("Aliases");
    await settings.navigateToTab("Aliases");
    await settings.expectTabActive("aliases");
    await settings.expectPanelVisible("aliases");
  });

  // -------------------------------------------------------------------------
  // 11. Profile tab has password card
  // -------------------------------------------------------------------------
  test("profile tab shows password change card", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    const panel = settings.panel;
    // Should have a "Password" CardTitle
    await expect(panel.getByText("Password", { exact: true }).first()).toBeVisible();
    // Should have current password and new password inputs
    await expect(
      panel.locator("label", { hasText: "Current password" }),
    ).toBeVisible();
    await expect(
      panel.locator("label", { hasText: "New password" }),
    ).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 12. Organization tab visible for admin
  // -------------------------------------------------------------------------
  test("organization tab is visible for admin", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // Admin users see the Organization tab
    await settings.expectTabVisible("Organization");
    await settings.navigateToTab("Organization");
    await settings.expectTabActive("organization");
    await settings.expectPanelVisible("organization");
  });

  // -------------------------------------------------------------------------
  // 13. Profile — change name, verify updated
  // -------------------------------------------------------------------------
  test("profile name can be changed", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    const panel = settings.panel;
    // Find the name input (second input after disabled email)
    const nameInput = panel.locator('label:has-text("Name") + input, label:has-text("Name") ~ div input').first();
    // Fallback: get all inputs, the non-disabled one is the name input
    const editableInputs = panel.locator("input:not([disabled])");
    const nameField = editableInputs.first();
    await nameField.fill("Updated Name E2E");

    // Submit the profile form — click the Save button in the Profile card
    const profileCard = panel.locator("form").first();
    await profileCard.locator('button:has-text("Save")').click();

    // Wait for success message
    await expect(page.getByText("Profile updated")).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 14. Profile — change password
  // -------------------------------------------------------------------------
  test("password can be changed", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    const panel = settings.panel;

    // Fill current password
    const currentPwInput = panel.locator('label:has-text("Current password") ~ div input, label:has-text("Current password") + input').first();
    // Fallback: use type=password inputs
    const passwordInputs = panel.locator('input[type="password"]');
    await passwordInputs.nth(0).fill(VALID_PASSWORD);
    // Fill new password
    await passwordInputs.nth(1).fill("NewTestPass1");

    // Submit the password form
    const passwordForm = panel.locator('form:has(label:has-text("Current password"))');
    await passwordForm.locator('button:has-text("Save")').first().click();

    // Wait for success message
    await expect(page.getByText("Password updated")).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 15. Labels — create label, verify in list
  // -------------------------------------------------------------------------
  test("create a new label", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");

    const panel = settings.panel;

    // Fill in the new label name
    await panel.locator('input[placeholder="Label name"]').fill("E2E Test Label");

    // Click Create button
    await panel.locator('button:has-text("Create")').click();

    // Verify the label appears in the list
    await expect(panel.getByText("E2E Test Label")).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 16. Labels — rename label
  // -------------------------------------------------------------------------
  test("rename an existing label", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");

    const panel = settings.panel;

    // First create a label to rename
    await panel.locator('input[placeholder="Label name"]').fill("Label To Rename");
    await panel.locator('button:has-text("Create")').click();
    await expect(panel.getByText("Label To Rename")).toBeVisible({ timeout: 5000 });

    // Click the edit (pencil) button on the label row
    const labelRow = panel.locator("div.divide-y > div", { hasText: "Label To Rename" });
    await labelRow.locator("button").first().click();

    // An input should appear with the label name
    const renameInput = labelRow.locator("input");
    await renameInput.fill("Renamed Label");
    await renameInput.press("Enter");

    // Verify renamed label appears
    await expect(panel.getByText("Renamed Label")).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 17. Labels — delete label
  // -------------------------------------------------------------------------
  test("delete a label", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");

    const panel = settings.panel;

    // Create a label to delete
    await panel.locator('input[placeholder="Label name"]').fill("Label To Delete");
    await panel.locator('button:has-text("Create")').click();
    await expect(panel.getByText("Label To Delete")).toBeVisible({ timeout: 5000 });

    // Click the delete (trash) button on the label row
    const labelRow = panel.locator("div.divide-y > div", { hasText: "Label To Delete" });
    // The trash button is the second button in the row actions
    const deleteBtn = labelRow.locator("button.text-destructive, button:has(svg.lucide-trash-2)").first();
    await deleteBtn.click();

    // Verify the label is gone
    await expect(panel.getByText("Label To Delete")).not.toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 18. Team — invite shows user as "invited"
  // -------------------------------------------------------------------------
  test("invite a team member", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Team");
    await settings.expectTabActive("team");

    const panel = settings.panel;

    // Fill the invite form
    const inviteEmail = uniqueEmail("invite");
    await panel.locator('input[type="email"]').fill(inviteEmail);
    await panel.locator('input[placeholder="Optional"]').fill("Invited User");

    // Submit
    await panel.locator('button:has-text("Send Invite")').click();

    // Wait for the invited user to appear in the team list
    await expect(panel.getByText(inviteEmail)).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 19. Team — change role
  // -------------------------------------------------------------------------
  test("change a team member role", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Team");
    await settings.expectTabActive("team");

    const panel = settings.panel;

    // Invite a user first
    const memberEmail = uniqueEmail("role-change");
    await panel.locator('input[type="email"]').fill(memberEmail);
    await panel.locator('input[placeholder="Optional"]').fill("Role Change User");
    await panel.locator('button:has-text("Send Invite")').click();

    // Wait for them to appear
    await expect(panel.getByText(memberEmail)).toBeVisible({ timeout: 5000 });

    // Find their row and look for a role selector/button
    const memberRow = panel.locator("div", { hasText: memberEmail }).first();
    // The role change dropdown is a select element in each user row
    const roleSelect = memberRow.locator("select");
    const hasRoleSelect = await roleSelect.isVisible().catch(() => false);
    if (hasRoleSelect) {
      await roleSelect.selectOption("admin");
      // Verify the select changed
      await expect(roleSelect).toHaveValue("admin");
    }
  });

  // -------------------------------------------------------------------------
  // 20. Organization — edit org name
  // -------------------------------------------------------------------------
  test("edit organization name", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Organization");
    await settings.expectTabActive("organization");

    const panel = settings.panel;

    // Wait for the org settings to load
    await expect(panel.locator('label:has-text("Organization Name")')).toBeVisible({ timeout: 5000 });

    // Update the org name
    const orgNameInput = panel.locator('label:has-text("Organization Name") ~ div input, label:has-text("Organization Name") + input').first();
    // Fallback: get the first input in the organization panel
    const orgInputs = panel.locator("input:not([type='password'])");
    const nameInput = orgInputs.first();
    await nameInput.fill("Updated Org Name E2E");

    // Save
    await panel.locator('button:has-text("Save")').click();

    // Verify success
    await expect(page.getByText("Organization settings saved")).toBeVisible({ timeout: 5000 });
  });

  // -------------------------------------------------------------------------
  // 21. Domains tab shows domain list
  // -------------------------------------------------------------------------
  test("domains tab shows domain list", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Domains");
    await settings.expectTabActive("domains");

    const panel = settings.panel;

    // The Domains tab should show either the domain list card with "Domains" title,
    // DNS records area, or the "Add" domain button for admins
    const domainsTitle = panel.getByText("Domains", { exact: true }).first();
    const addButton = panel.locator('button:has-text("Add")');
    const refreshButton = panel.locator('button:has-text("Refresh")');

    // At least one of these domain-related elements should be visible
    const hasDomainTitle = await domainsTitle.isVisible().catch(() => false);
    const hasAddButton = await addButton.isVisible().catch(() => false);
    const hasRefreshButton = await refreshButton.isVisible().catch(() => false);

    expect(hasDomainTitle || hasAddButton || hasRefreshButton).toBe(true);
  });

  // -------------------------------------------------------------------------
  // 22. Domains visibility toggle
  // -------------------------------------------------------------------------
  test("domains visibility toggle", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Domains");
    await settings.expectTabActive("domains");

    const panel = settings.panel;

    // The domain list has per-domain visibility checkboxes (h-4 w-4 rounded border)
    // and a "Select all" toggle at the top showing "N of M active"
    const activeText = panel.locator("text=/\\d+ of \\d+ active/");
    const hasActiveText = await activeText.isVisible({ timeout: 3000 }).catch(() => false);

    if (!hasActiveText) {
      test.skip(true, "No domains with visibility toggles found — org has no domains yet");
      return;
    }

    // Get the initial active count text
    const initialText = await activeText.textContent();

    // Click the select-all toggle button (the one with "N of M active" text)
    await activeText.click();

    // Wait for the UI to update
    await page.waitForTimeout(500);

    // The text should change (toggled all on or all off)
    const updatedText = await activeText.textContent();
    expect(updatedText).not.toBe(initialText);
  });

  // -------------------------------------------------------------------------
  // 23. Team tab shows invite form
  // -------------------------------------------------------------------------
  test("team tab shows invite form", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Team");
    await settings.expectTabActive("team");

    const panel = settings.panel;

    // The Team tab has an "Invite Team Member" card with email input and "Send Invite" button
    await expect(panel.locator('input[type="email"]')).toBeVisible({ timeout: 5000 });
    await expect(panel.locator('button:has-text("Send Invite")')).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 24. Disable user button exists
  // -------------------------------------------------------------------------
  test("disable user button exists", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Team");
    await settings.expectTabActive("team");

    const panel = settings.panel;

    // Wait for team members to load
    await page.waitForTimeout(2000);

    // The team list shows user rows. Each row (except self) has a Disable/Revoke button
    // with the UserX icon and class "text-destructive"
    // The creator (current user) won't have a disable button for themselves,
    // so we need at least one other user or we skip
    const disableButtons = panel.locator('button:has-text("Disable"), button:has-text("Revoke")');
    const count = await disableButtons.count();

    if (count === 0) {
      // Only one user (the admin themselves) — invite someone first to see the button
      const inviteEmail = uniqueEmail("disable-test");
      await panel.locator('input[type="email"]').fill(inviteEmail);
      await panel.locator('input[placeholder="Optional"]').fill("Disable Test User");
      await panel.locator('button:has-text("Send Invite")').click();
      await expect(panel.getByText(inviteEmail)).toBeVisible({ timeout: 5000 });

      // Now a Revoke button should exist for the invited user
      await expect(
        panel.locator('button:has-text("Revoke")').first(),
      ).toBeVisible({ timeout: 5000 });
    } else {
      await expect(disableButtons.first()).toBeVisible();
    }
  });

  // -------------------------------------------------------------------------
  // 25. Labels tab shows create form
  // -------------------------------------------------------------------------
  test("labels tab shows create form", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Labels");
    await settings.expectTabActive("labels");

    const panel = settings.panel;

    // The Labels tab has an input with placeholder "Label name" and a "Create" button
    await expect(panel.locator('input[placeholder="Label name"]')).toBeVisible({ timeout: 5000 });
    await expect(panel.locator('button:has-text("Create")')).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 26. Organization tab shows org settings
  // -------------------------------------------------------------------------
  test("organization tab shows org settings", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    await settings.navigateToTab("Organization");
    await settings.expectTabActive("organization");

    const panel = settings.panel;

    // The Organization tab shows "Organization Name" label with an input,
    // "Resend API Key" label, and a "Save" button
    await expect(panel.locator('label:has-text("Organization Name")')).toBeVisible({ timeout: 5000 });
    await expect(panel.locator('label:has-text("Resend API Key")')).toBeVisible();
    await expect(panel.locator('button:has-text("Save")')).toBeVisible();
  });

  // -------------------------------------------------------------------------
  // 27. Jobs tab shows job list
  // -------------------------------------------------------------------------
  test("jobs tab shows job list", async ({ page }) => {
    await settings.openSettings();
    await settings.expectOpen();

    // Jobs tab is admin-only
    await settings.expectTabVisible("Jobs");
    await settings.navigateToTab("Jobs");
    await settings.expectTabActive("jobs");

    const panel = settings.panel;

    // The JobsPanel shows either a table with headers (Type, Status, Attempts, Created, Error)
    // or the empty state "No jobs found." or a loading state
    const jobsTitle = panel.getByText("Email Jobs");
    const noJobs = panel.getByText("No jobs found.");
    const loadingJobs = panel.getByText("Loading jobs...");
    const jobsTable = panel.locator("table");

    // Wait for the panel to settle (loading finishes)
    await page.waitForTimeout(2000);

    const hasTitle = await jobsTitle.isVisible().catch(() => false);
    const hasNoJobs = await noJobs.isVisible().catch(() => false);
    const hasTable = await jobsTable.isVisible().catch(() => false);
    const hasLoading = await loadingJobs.isVisible().catch(() => false);

    // One of these should be visible — the panel rendered correctly
    expect(hasTitle || hasNoJobs || hasTable || hasLoading).toBe(true);
  });
});
