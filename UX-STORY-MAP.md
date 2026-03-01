# Inboxes — Complete User Experience Story Map

## 1. Authentication & Account Creation

**Sign Up (Commercial/SaaS)**
- As a user, I visit the root URL and see a marketing landing page with "Get Started"
- As a user, I fill in org name, name, email, and password to create an account
- As a user, I receive a 6-digit verification code and enter it to verify my email
- As a user, I am redirected to onboarding after verification

**Self-Hosted Setup**
- As a self-hosted admin, I visit the root URL and am redirected to `/setup`
- As a user, I create the admin account with optional system Resend API key
- As a user, I optionally configure the system email "from" address (for invites/resets)
- As a user, I am redirected to onboarding after setup

**Login**
- As a user, I enter email and password to sign in
- As a user with unverified email, I am redirected to the verification flow
- As a user, I am sent to onboarding if not completed, or to inbox if completed

**Forgot / Reset Password**
- As a user, I request a reset link by entering my email (no email existence leak)
- As a user, I click the link, enter a new password (complexity enforced), and am redirected to login
- As a user, all my other sessions are immediately invalidated on password reset

**Account Claiming (Invite)**
- As an invited user, I click the invite link from my email
- As a user, I set my name and password, and am logged in immediately
- As a user, invite tokens expire after 7 days

**Logout**
- As a user, I click "Sign out" — my cookie is deleted and my JWT is blacklisted in Redis

**Change Password (In-App)**
- As a user, I change my password in Settings — current password required, all other sessions invalidated, I stay logged in

---

## 2. Onboarding (4-Step Wizard)

**Step 1: Connect Resend**
- As a user, I paste my Resend API key — it's validated against Resend's API
- As a user, my key is AES-GCM encrypted and stored; my domains are imported

**Step 2: Select Domains**
- As a user, I see all my Resend domains with checkboxes to include/exclude
- As a user, deselected domains are hidden from the sidebar
- As a user, a webhook is registered with Resend for real-time email delivery

**Step 3: Import Emails**
- As a user, sync starts automatically — I see a progress bar and rotating tips
- As a user, emails are imported in the background while I can proceed to step 4

**Step 4: Set Up Addresses**
- As a user, I categorize each discovered email address as Person, Alias, or Skip
- As a user, completing this step marks onboarding done and redirects to my inbox

---

## 3. Email Composition

**New Email**
- As a user, I click Compose (sidebar) or press `Cmd+N` to open a floating compose window
- As a user, I select a From address from my aliases (dropdown if multiple, static if one)
- As a user, I type recipients in To/Cc/Bcc with autocomplete from contact history (after 2 chars)
- As a user, I type a subject (max 500 chars) and compose a rich-text body (bold, italic, underline, links, lists, blockquotes)
- As a user, I attach files via the paperclip button (max 10MB, executable types blocked)
- As a user, my draft auto-saves after 3 seconds of inactivity; title bar shows "Saving..."/"Saved"
- As a user, I send with `Cmd+Enter` or the Send button — the email is queued (202 Accepted) and the window closes
- As a user, I can minimize the compose window to a collapsed bar and restore it later
- As a user, I can discard the draft (with confirmation) via the trash icon

**Reply**
- As a user, I click Reply on the last message or any specific message in a thread
- As a user, To is pre-filled with the sender, subject gets "Re:" prefix, original message is quoted below
- As a user, my From address is auto-selected based on the last address I sent from in this thread

**Reply All**
- As a user, Reply All pre-fills To with the sender and Cc with all other recipients (minus me)

**Forward**
- As a user, Forward opens compose with empty To, "Fwd:" subject, and a forwarded-message header block
- As a user, original attachments are NOT carried over — I must re-attach manually

---

## 4. Reading Emails & Threads

**Thread List**
- As a user, I see threads sorted by most recent message, with sender, subject, snippet, and time
- As a user, unread threads have a blue background tint and bold sender name
- As a user, I see hover actions (Archive, Trash, Mark Read/Unread) on desktop
- As a user, I can select threads via checkboxes or keyboard `x`

**Thread View (Reading Pane)**
- As a user, clicking a thread opens a split-pane reading view (desktop) or full-screen (mobile)
- As a user, older messages are collapsed; the newest is expanded — I can click to expand/collapse any
- As a user, the view auto-scrolls to the newest message
- As a user, opening an unread thread automatically marks it as read (optimistic)
- As a user, I see HTML email rendered with sanitized content; plain text as fallback
- As a user, outbound emails show delivery status badges (Queued/Sent/Delivered/Bounced/Failed)
- As a user, I can click a sender's name to see a contact card popover with copy-email button
- As a user, attachments appear as downloadable chips below each message
- As a user, trashed threads show a red banner with days until permanent deletion

---

## 5. Thread Actions

**Single Thread (from toolbar or reading pane)**
- As a user, I can **archive** a thread (removes from inbox, toast with Undo)
- As a user, I can **trash** a thread (30-day auto-delete timer, toast with Undo)
- As a user, I can **permanently delete** from Trash (confirmation dialog, no undo)
- As a user, I can **report spam** (moves to spam folder)
- As a user, I can **move to inbox** from Archive/Trash/Spam
- As a user, I can **star/unstar** a thread (yellow star icon, appears in Starred folder)
- As a user, I can **mute/unmute** a thread (BellOff icon, suppresses future notifications)
- As a user, I can **mark read/unread** manually

**Bulk Actions (multiple selected threads)**
- As a user, I can select threads via checkboxes, `x` key, or Select All dropdown (All/None/Read/Unread/Starred/Unstarred)
- As a user, I can bulk archive, trash, delete, spam, read/unread, mute/unmute, and apply/remove custom labels
- As a user, bulk archive and trash show Undo toasts with count

---

## 6. Folder Navigation & Sidebar

**System Folders**: Inbox, Sent, Drafts, Archive, Starred, Spam, Trash
- As a user, each folder filters threads by label; only Inbox shows an unread count badge
- As a user, Trash shows a subtitle: "Messages are permanently deleted after 30 days"

**Custom Labels**
- As a user, custom labels appear below system folders in the sidebar
- As a user, I can create/rename/delete labels in Settings > Labels
- As a user, I can apply labels to threads via the toolbar Tag dropdown

**Domain Switching**
- As a user, I see domain icons in the left strip; clicking one navigates to that domain's inbox
- As a user, domains with unread mail show a red dot; the active domain has a ring indicator
- As a user, I can switch domains with `Cmd+1` through `Cmd+9`

**Mobile**
- As a mobile user, the sidebar is hidden behind a hamburger menu
- As a mobile user, tapping the hamburger opens a slide-over panel

---

## 7. Search

- As a user, I type in the search bar at the top of any folder and press Enter
- As a user, search uses PostgreSQL full-text search across subject, body, and sender address
- As a user, results show up to 50 threads sorted by recency, with folder labels shown per result
- As a user, search is scoped to the current domain but crosses all folders
- As a user, I can press `/` or `Cmd+K` to focus the search bar
- As a user, I can act on search results (star, archive, trash, etc.) just like normal threads

---

## 8. Drafts

- As a user, drafts auto-save while composing (3-second debounce)
- As a user, I navigate to the Drafts folder to see all saved drafts
- As a user, clicking a draft opens it in the compose window with all fields restored
- As a user, I can delete drafts from the list or via the compose window's discard button
- As a user, sending a draft deletes it from the drafts list (handled by the background worker)
- As a user, draft attachments are persisted but NOT visually restored when reopening a draft (gap)

---

## 9. Drag and Drop

- As a user, I can drag a thread (or multi-selected threads) to any sidebar folder
- As a user, the drop target highlights with a blue ring; a floating preview shows subject and sender
- As a user, dropping triggers the appropriate move action (archive, trash, spam, label, etc.)
- As a user, drag activates after 8px movement (mouse) or 250ms hold (touch)

---

## 10. Keyboard Shortcuts

| Key | Action |
|---|---|
| `j` / `k` | Navigate thread list down/up |
| `Enter` / `o` | Open focused thread |
| `x` | Toggle selection |
| `s` | Star/unstar |
| `e` | Archive |
| `#` | Trash |
| `m` | Mute/unmute |
| `r` | Refresh |
| `Shift+I` / `Shift+U` | Mark read / unread |
| `Cmd+N` | Compose new email |
| `Cmd+Enter` | Send (in compose) |
| `/` or `Cmd+K` | Focus search |
| `Cmd+1-9` | Switch domain |
| `?` | Keyboard shortcuts help dialog |

---

## 11. Contacts

- As a user, contacts are implicit — derived from email history, not a dedicated database
- As a user, typing 2+ chars in a recipient field shows autocomplete suggestions ranked by frequency
- As a user, clicking a sender name in thread view shows a contact card with avatar, name, email, and copy button
- As a user, there is no contacts page, no manual contact creation, no contact editing

---

## 12. Settings

**Profile**: Edit name, change password, toggle desktop notifications, trigger email sync

**Domains** (admin): Add/verify/reorder/delete domains, save visibility, re-register webhook, view DNS records

**Team** (admin): Invite members, resend invites, change roles, disable/enable users

**Aliases** (admin): Create/edit/delete aliases, assign users to aliases, set default send-from alias

**Labels**: Create/rename/delete custom labels

**Organization** (admin): Edit org name, update Resend API key, set API rate limit

**Billing** (commercial): View plan status, upgrade to Pro (Stripe checkout), manage subscription (Stripe portal)

**System** (owner, self-hosted): Configure system email from-address, send test email

**Jobs** (admin): View recent email sending jobs with status and error info

---

## 13. Billing & Payment (Commercial Mode Only)

- As a new user, my org starts on the "free" plan — all features gated behind a paywall
- As a user hitting a 402, I see a PaymentWall modal ("Upgrade to Pro" for admins, "Ask your admin" for members)
- As an admin, I click "Upgrade to Pro" which redirects to Stripe Checkout
- As a user, after checkout I'm redirected back with a green "Subscription activated!" banner
- As a user, subscription status updates in real-time via Stripe webhooks
- As a user, cancelled subscriptions get a 7-day grace period (3 days for failed payments)
- As an admin, I manage my subscription via the Stripe billing portal

---

## 14. Real-Time Updates & Notifications

- As a user, my inbox updates in real-time via WebSocket — new emails appear instantly
- As a user, I see a slide-in toast notification when a new email arrives (5-second auto-dismiss)
- As a user, thread actions by others (or in other tabs) are reflected immediately
- As a user, if WebSocket disconnects, it reconnects with exponential backoff and catches up on missed events
- As a user, the sidebar shows a yellow "Reconnecting..." banner after 3 seconds of disconnection
- As a user, cross-tab sync keeps multiple open tabs consistent via BroadcastChannel
- As a user, bounced/failed delivery triggers a native OS notification (if permission granted)

---

## 15. Background Processes (Invisible UX)

| Process | Schedule | User Impact |
|---|---|---|
| **Email Worker** | Continuous (Redis queue) | Processes inbound emails and dispatches outbound sends |
| **Sync Worker** | Continuous (Redis queue) | Imports historical emails during onboarding/re-sync |
| **Trash Collector** | Every 1 hour | Permanently deletes trashed threads after 30 days |
| **Domain Heartbeat** | Every 6 hours | Detects disconnected/reconnected domains |
| **Status Recovery** | Every 5 minutes | Fixes missed delivery status webhooks |
| **Event Pruner** | Every 6 hours | Cleans up old WebSocket replay events (default 90-day retention) |
| **Stale Job Recovery** | Every 60 seconds | Re-enqueues stuck email/sync jobs |

---

## 16. Inbound Email Pipeline (Behind the Scenes)

- As a user, Resend webhooks are verified via Svix HMAC signatures
- As a user, inbound emails are queued as async jobs, fetched from Resend API for full body
- As a user, emails are threaded by In-Reply-To header, then by subject+counterparty (90-day window)
- As a user, spam is classified heuristically (SPF/DKIM/DMARC failures, spam headers, keywords) — threshold 0.70
- As a user, replies to trashed threads un-trash them (unless muted)
- As a user, non-admin members only see threads delivered to their assigned aliases

---

## 17. Rate Limiting

| Endpoint | Limit |
|---|---|
| Login | 10 per IP per 15 min |
| Signup | 5 per IP per hour |
| Forgot Password | 3 per IP + 3 per email per hour |
| Reset Password | 5 per IP per 15 min |
| Verify Email | 5 per IP per 15 min |
| Resend Verification | 3 per IP per hour |
| Claim Invite | 5 per IP per 15 min |
| Setup | 3 per IP per 15 min |

Authenticated endpoints have no HTTP rate limits. Outbound email sending is throttled per-org via an internal Resend API rate limiter (default 2 RPS, configurable up to 100).

---

## 18. Attachments

- As a user, I attach files via the paperclip button (one at a time, max 10MB)
- As a user, blocked extensions: `.exe`, `.bat`, `.scr`, `.com`, `.msi`, `.cmd`, `.ps1`, `.sh`, `.vbs`, `.js`, `.wsh`, `.wsf`
- As a user, backend also blocks by MIME type detection (executable binaries)
- As a user, received email attachments show as download chips; sent attachments are base64-encoded to Resend
- As a user, there is no drag-and-drop upload, no multi-file selection, no inline image insertion, no attachment previews

---

## 19. Theme & Layout

- As a user, I can switch between light and dark mode (persists via next-themes)
- As a user, the app defaults to my OS preference on first visit
- As a user, the theme toggle is available on auth pages and in the sidebar settings
- As a user, the app uses `h-dvh` for proper mobile viewport handling
- As a user, error pages show "Something went wrong" with a "Try again" button
- As a user, 404 pages show "Page not found" with a "Go home" link
