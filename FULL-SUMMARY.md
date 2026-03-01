# Inboxes — Full Findings Summary

Plain-language summary of everything found across the Pre-Launch Audit (46 items) and Gap Finder (82 items). Grouped by theme, not by source document.

---

## Security — Fix Before Launch

**1. Anyone can spy on another org's sync status.**
The sync job endpoint doesn't check if you belong to the org you're asking about. Add an org ownership check to the query.

**2. Attachment filenames can break HTTP headers.**
A specially crafted filename with quotes in it could mess up how browsers handle downloads. Sanitize the filename before putting it in the header.

**3. The Stripe redirect check can be fooled.**
The payment wall checks if a URL "ends with stripe.com" — but `stripe.com.evil.com` also passes that check. Use exact domain matching instead.

**4. Docker containers run as root.**
If someone breaks out of the container, they get full control of the host machine. Add a non-root user to both Dockerfiles.

**5. No Content Security Policy header.**
Without CSP, if the HTML sanitizer ever has a bug, malicious scripts in emails could run. Add a strict CSP header.

**6. Billing checkout URLs can be tampered with.**
The domain ID from the database gets dropped into a URL without checking if it's actually a valid ID. Validate it's a UUID first.

**7. Email addresses are case-sensitive in the database.**
`User@Example.com` and `user@example.com` can be two different accounts. Login fails if you don't match the exact case you signed up with. Normalize to lowercase everywhere.

**8. Verification codes can be brute-forced.**
The 6-digit code has only 1 million possibilities and the verify endpoint has no rate limit. An attacker could try all codes within the 15-minute window.

**9. Disabled users can keep using the app for up to 7 days.**
The login check only looks at the JWT token, not whether the user is still active. A disabled user's token keeps working until it naturally expires.

**10. Webhook secret lookup failures are silently ignored.**
If the database lookup for the webhook secret fails, the code uses an empty secret for verification. This could let fake webhooks through.

**11. LIKE wildcards aren't escaped in contact search.**
Typing `%` in the contact search returns every contact in the system. The special characters need to be escaped before the database query.

**12. BCC recipients are leaked.**
BCC addresses get stored in the thread's participant list, which the frontend can see. Anyone in the thread can figure out who was BCC'd by comparing participant lists.

**13. Email tracking pixels load automatically.**
Every image in every email loads immediately, including invisible tracking pixels. There's no "load images" button or privacy setting.

**14. Malicious CSS in emails can break the layout.**
The HTML sanitizer allows the `style` attribute, so a sender could use CSS to cover the entire screen or hide content.

---

## Security — Fix Soon After Launch

**15. Admin and cron endpoints have no rate limits.**
A compromised admin token could trigger unlimited trash purges or webhook cleanups.

**16. Database connection defaults to no encryption.**
The default config uses `sslmode=disable`. If someone uses a remote database, passwords travel in plain text.

**17. Redis has no password by default.**
Anyone on the network can connect to Redis and read/write data.

**18. No CSRF protection beyond cookies.**
The app relies on SameSite cookies alone. It's fine today since no GET endpoints change data, but it's fragile.

**19. Token blacklist fails open when Redis is down.**
If Redis goes down, logged-out users can keep using their old tokens. The system should either reject all tokens or log a loud warning.

**20. Attachments could serve malicious HTML.**
A user could upload an HTML file disguised as text. Force all downloads to use a safe content type.

**21. No limit on login attempts per account.**
Rate limiting exists per IP address, but there's no per-account lockout after repeated failures.

**22. The change-password endpoint has no rate limit.**
An attacker could brute-force your current password by calling this endpoint over and over.

**23. The resend-verification endpoint has no rate limit.**
An attacker could flood someone's inbox with hundreds of verification emails.

**24. No "last admin" protection.**
An admin can disable the only other admin, leaving the org with a single point of failure. There's no check ensuring at least one admin remains.

**25. No limit on concurrent sessions.**
A compromised password means unlimited simultaneous logins with no way to detect it.

**26. Password change + Redis failure = silent failure.**
The app tells you "other sessions logged out" but if Redis is down, they actually aren't.

---

## Drafts & Compose Issues

**27. Closing compose silently loses your draft if save fails.**
If auto-save fails and you close the window, the error is swallowed and your draft is gone. The window should stay open and show an error.

**28. Rapid saves can create duplicate drafts.**
If you type fast, multiple save requests fire before the first one returns with a draft ID. This creates duplicate drafts in the database.

**29. Double-clicking Send can send the email twice.**
There's no idempotency protection on the send endpoint. Two quick clicks create two separate send jobs.

**30. Draft auto-save failures show no error.**
If saving fails, the status indicator just goes blank. You think your draft is saved when it isn't.

---

## Search & Navigation

**31. Empty search results show nothing — literally nothing.**
When search finds zero results, there's no "No results found" message. You can't tell if it's still loading, broken, or just empty.

**32. Search placeholder doesn't explain what it searches.**
The input just says "Search..." — users don't know it can search subjects, senders, and email bodies. *(Idea: placeholder could say "Search [current folder]..." to hint at scope.)*

**33. Thread list scroll position is lost when you go back.**
View a thread, press back, and you're dumped back to the top of the list instead of where you were.

**34. No visible pagination controls.**
There are no page numbers, no "load more" button, and no indication of how many pages exist. *(Note: arrow-based pagination may already exist — needs confirmation before fixing.)*

---

## Session & Auth UX

**35. Session expiry kills your compose window without warning.**
If your session expires while writing an email, you're instantly redirected to the login page. Everything you typed is lost. Should show a warning modal first.

**36. Trash countdown banner never updates.**
"This will be deleted in 3 days" is calculated once when you open the thread. It never recalculates, even if you leave the tab open for hours.

**37. Keyboard shortcuts exist but nobody knows about them.**
There are useful shortcuts, but no visible hint anywhere in the UI that they exist. You'd have to accidentally press `?`. *(Fix: add a small `?` icon in the top-right of the app bar that opens the shortcuts dialog.)*

---

## Form Validation Mismatches

**38. Subject line: frontend allows 998 characters, backend allows 500.**
You can type a 600-character subject, hit send, and get a confusing backend error. *(Fix: enforce 500 on frontend to match backend.)*

**39. Password rules aren't shown on the frontend.**
Frontend only checks minimum 8 characters. Backend also requires uppercase, lowercase, and a digit. Users get rejected with no useful explanation.

**40. Setup form doesn't validate the email address.**
Unlike signup and login which properly validate emails, the initial setup accepts anything.

**41. Claim (invite) form doesn't validate name length.**
Signup checks that names are under 255 characters, but the invite claim form doesn't.

**42. Rate limit setting has no visible range.**
Backend enforces 1-100 for the Resend rate limit, but the frontend doesn't show this. Entering "abc" fails silently.

**43. No limit on email recipients.**
You can add hundreds of To/Cc/Bcc recipients with no cap on either frontend or backend. *(Fix: cap at 50 per field for now — check Resend's limits to confirm.)*

---

## Race Conditions

**44. Label operations aren't wrapped in transactions.**
Bulk archive, trash, star, mute — they all run multiple SQL statements without a transaction. Concurrent operations on the same threads cause inconsistent state.

**45. Two simultaneous draft sends can create duplicate threads.**
If two sends happen at the same time without a thread ID, both create new threads with identical subjects. *(Note: may already have duplicate send prevention — needs confirmation.)*

**46. Rapid star/unstar clicks get out of sync.**
Quick toggling can receive API responses out of order, so the final state depends on network timing, not your last click.

**47. Bulk action clears your selection before the API call finishes.**
If the bulk operation fails, your selection is already gone and you can't retry.

**48. Label rename isn't atomic.**
The org label and thread labels are updated in separate queries. A concurrent label creation during rename can create orphaned labels.

---

## Error Handling — Backend

**49. 20+ database query results are never checked for errors.**
Across threads, auth, setup, onboarding, webhooks, users, and billing — if a query fails, the code just uses whatever garbage is in the variable.

**50. 15+ database write operations are fire-and-forget.**
Trash expiry updates, label operations, read/unread marking — if they fail, nobody knows.

**51. 30+ JSON encoding errors are ignored.**
Corrupted JSON gets silently stored in the database or sent to external APIs.

**52. Redis failures return success to webhook callers.**
When Redis is down and a job can't be queued, the webhook endpoint still returns 200 OK. Resend thinks delivery succeeded, but the email is stuck.

**53. Workers have no panic recovery.**
A single unexpected error kills a background worker permanently. No jobs get processed, no alert fires.

---

## Error Handling — Frontend

**54. Settings modal tabs show infinite spinners on API failure.**
Team, Aliases, Labels, Org, Billing — if the API call fails, the loading spinner never stops. The catch block swallows the error.

**55. Search failure shows infinite loading.**
A failed search query shows a spinner forever with no retry option.

**56. File upload errors aren't displayed.**
The upload function uses raw `fetch()` instead of the app's error-handling wrapper. Network errors are completely ignored.

---

## Database Performance

**57. N+1 query: trash expiry date fetched per thread.**
Inside the thread list loop, a separate query runs for each thread to get its trash expiry date. Should be in the main query.

**58. No index on deleted_at column.**
Every thread list query filters on `deleted_at IS NULL` but there's no index, so it scans the whole table.

**59. Labels subquery runs once per row in thread list.**
The labels for each thread are fetched via a correlated subquery — for 50 threads, that's 50 extra queries.

**60. Bulk delete runs 300+ queries for 100 threads.**
Each thread needs 3+ separate queries in a loop. Should be a single batch operation.

**61. Alias lookup runs once per recipient in inbound emails.**
An email with 10 recipients fires 10 separate queries. Should batch with `WHERE address = ANY(...)`.

**62. No indexes on JSON columns used for filtering.**
Queries filtering on JSON arrays (to_addresses, cc_addresses) do full table scans.

**63. No composite index for the most common query.**
Every thread list query filters by org + not deleted + ordered by date. No index covers all three.

**64. Foreign key columns are missing indexes.**
Several tables have foreign keys without indexes, causing slow joins.

**65. Onboarding address query has no LIMIT.**
An org with millions of discovered addresses fetches them all at once.

---

## Email Threading & Rendering

**66. The References header is stored but never used for threading.**
This is the most reliable way to thread emails per RFC 5322, but it's completely ignored.

**67. Emails with no subject all merge into one thread.**
A blank-subject query matches ALL no-subject emails, so unrelated conversations get lumped together.

**68. Cross-domain replies create separate threads.**
A reply from domain B to a thread on domain A starts a new thread instead of continuing the existing one.

**69. No right-to-left text support.**
Hebrew, Arabic, and Persian emails display incorrectly because there's no `dir="auto"` on email content.

**70. Bounce notifications show up as regular threads.**
Bounce emails from mailer-daemon create user-visible threads that shouldn't be there.

**71. Reply-To header is ignored.**
The Reply-To address is stored but never used. Replies go to the From address instead, which isn't always correct.

---

## Domain & User Lifecycle

**72. Deleting a domain doesn't clean up its data.**
Aliases, threads, emails, and discovered addresses are left orphaned when a domain is soft-deleted.

**73. Deleted domains still receive and process emails.**
The webhook handler doesn't check domain status. Emails to deleted domains still get processed.

**74. Domain uniqueness is global, not per-org.**
A soft-deleted domain in one org blocks any other org from adding it. *(Note: intentional for now — prevents domain claiming. May need "deleted in our system" vs "deleted from Resend" distinction. Domain transfers = support ticket like Gmail.)*

**75. DNS record changes are never detected.**
The domain heartbeat only checks if the domain still exists in Resend, not whether MX/SPF/DKIM records changed.

**76. Disabled users' aliases still receive emails.**
No check that the recipient alias belongs to an active user. Emails to disabled users' aliases fail silently. *(Note: by design — emails roll up to whoever the disabled user was merged into. Admin must reactivate to undo the rollup.)*

**77. No hard delete for GDPR compliance.**
Only soft-delete exists for users and orgs. Personal data persists forever. Email addresses stay locked by uniqueness constraints. *(Action: search entire repo for hard delete gaps and address comprehensively.)*

**78. Webhooks for deleted orgs aren't cleaned up.**
Deleted orgs' webhooks silently fail instead of unregistering and returning 410 Gone.

**79. No notification when a domain disconnects.**
When the heartbeat detects a disconnected domain, it only logs it. No email alert, no in-app notification.

---

## Billing & Stripe

**80. Stripe events aren't deduplicated.**
Webhook retries process the same event multiple times. No tracking of already-processed event IDs.

**81. Only 2 of ~17 required Stripe events are handled.**
Missing handlers for subscription updates, successful payments, failed payments, and many more. *(Note: need to define the full list of Stripe events to handle — likely ~17, not 5.)*

**82. Brief paywall flash after checkout.**
User returns from Stripe before the webhook arrives. For 3-5 seconds, the app still thinks they're on the free plan.

**83. Double-clicking the upgrade button creates two checkout sessions.**
No request deduplication on the frontend.

**84. Grace period is never enforced by a background job.**
After the grace period expires, the plan stays "cancelled" forever. No job sets it back to "free."

**85. Past-due is treated as immediate cancellation.**
When Stripe marks a subscription as past-due (still retrying payment), the app shows "cancelled." If Stripe's retry succeeds later, it fixes itself, but users panic in between.

**86. Emails still send after org deletion.**
Deleting an org doesn't drain the Redis queue. Already-queued emails go out after the org is gone. *(Fix: domain/org soft-delete should drain or discard queued jobs as cleanup.)*

**87. No plan check at actual send time.**
The plan is checked when the email is queued, not when it's sent. If the subscription lapses between queue and send, the email still goes out.

**88. No cancellation confirmation in the UI.**
Deleting an org immediately cancels the Stripe subscription with no confirmation dialog.

---

## WebSocket & Real-Time

**89. No limit on WebSocket connections per user.**
A compromised account can open thousands of connections, eating server memory. *(Fix: cap at ~3-5 connections per user.)*

**90. Revoked users keep receiving events for up to 5 minutes.**
Token validity is only checked every 5 minutes on WebSocket connections.

**91. Database errors in alias filtering broadcast to everyone.**
If the alias lookup fails, the event goes to ALL org members instead of just the ones who should see it.

**92. Event catchup has no age limit.**
A client offline for 30 days replays the entire 30-day event history. *(Fix: cap catchup at ~24-48 hours — anything older, force a full API refetch instead.)*

**93. Reconnection jitter is too small.**
Only 1 second of randomness. A hundred users reconnecting at once creates a server spike.

---

## Accessibility

*Low priority — app is 100% behind auth, no SEO or AI indexing impact. No legal risk at current scale. Address when there's traction and real screen reader users.*

**94. Missing ARIA labels on icon-only buttons.**
Star, archive, trash buttons are announced as just "button" to screen readers.

**95. Compose window has no focus trap.**
The compose window acts like a modal but Tab key escapes to elements behind it. No `role="dialog"` or `aria-modal`.

**96. Recipient autocomplete has no ARIA combobox pattern.**
Screen readers can't navigate the suggestion dropdown.

**97. Thread list has no semantic list markup.**
Screen readers can't announce "list with 50 items" or navigate by item.

**98. No skip navigation link.**
Keyboard users must tab through the entire sidebar to reach the main content.

**99. Page title doesn't update on navigation.**
Moving between inbox, sent, drafts doesn't change the browser title. Screen readers can't announce the current page.

**100. Color contrast likely fails WCAG standards.**
144 uses of muted text colors with reduced opacity — many probably fail the 4.5:1 contrast ratio.

**101. Settings modal tabs aren't accessible.**
No proper tablist/tab/tabpanel ARIA roles.

**102. Collapsed emails have no aria-expanded attribute.**
Screen readers can't tell if an email message is expanded or collapsed.

**103. Toast notifications aren't announced.**
The notification toasts have no live region markup, so screen readers ignore them.

**104. Drag-and-drop has no keyboard alternative.**
Moving threads between folders only works with a mouse.

**105. Form errors aren't announced to screen readers.**
Error messages on auth forms lack `role="alert"` and `aria-live`.

---

## Configuration & Environment

**106. Several required env vars are missing from .env.example.**
`TRASH_COLLECTOR_ENABLED`, `BACKEND_URL`, `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_WS_URL` are used in code but not documented.

**107. No validation on critical config values.**
Encryption key format, session secret length, URL formats, port ranges — none are checked at startup.

**108. Stripe vars not in docker-compose.yml.**
Commercial mode can't work in Docker because the Stripe environment variables aren't passed through.

**109. All worker intervals are hardcoded.**
Heartbeat, trash collection, event pruning, status recovery — none can be tuned via environment variables.

**110. Frontend WebSocket has a localhost fallback.**
If the WebSocket URL can't be derived, it falls back to `ws://localhost:8080`, which would leak connection attempts in production.

---

## Infrastructure

**111. CI doesn't build the Go binary.**
Tests run, but `go build` never runs. Linker errors won't be caught until the Docker build.

**112. PUBLIC_URL missing from docker-compose.**
Required for webhook registration but easy to forget since it's not in the compose file.

**113. No container resource limits.**
A runaway container can consume all host memory and CPU.

**114. Default database password is "inboxes".**
Easy to accidentally copy to production. *(Note: deployment instructions should enforce changing this — add a required env var or startup check.)*

**115. No migration rollback documentation.**
If a migration breaks production, there's no documented procedure to roll back.

---

## Dead Code

**116. `/api/contacts/lookup` endpoint is never called.**
Route exists, handler works, but no frontend code uses it.

**117. `/api/threads/unread-count` is superseded.**
Frontend uses a different endpoint (`/api/domains/unread-counts`) instead.

**118. Attachment download endpoint appears unused.**
No frontend code calls it — only the upload endpoint is used. *(Note: may be a stub for future use — keep for now.)*

**119. `truncate()` utility function is never imported.**
Defined in utils but never used anywhere.

---

## Data Model Concerns

**120. Users table email uniqueness is global.**
Same email can't exist in two different orgs. Fine for single-org, but blocks multi-org in the future.

**121. No cascade deletes on foreign keys.**
Deleting an org doesn't automatically clean up threads, emails, aliases, etc. Manual SQL deletes leave orphaned rows. *(Note: CASCADE would conflict with soft-delete pattern — real fix is ensuring soft-delete handlers clean up properly, see #72.)*

**122. Attachments stored as binary in the database.**
Every query on the attachments table loads full file contents into memory. Database backups grow fast. Should eventually move to object storage.

---

## Missing Features (Expected but Not Built)

**123. No email signatures.**
No per-alias or global signature support. Planned — see REACT-EMAIL-FEATURE.md.

**124. No "select all across pages."**
Bulk selection only works on the current page.

**125. Browser notification permission is never requested.**
The backend supports push notifications, but the frontend never asks for permission.

**126. Draft attachments aren't restored when reopening.**
Attachment IDs are saved but not displayed when you reopen a draft.

---

*126 total findings. 46 from the Pre-Launch Audit, 78 from the Gap Finder (4 removed as out of scope), zero overlap.*
