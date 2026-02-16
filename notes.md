# Notes & Ideas

## Open

### Resume sync on revisit
If a user closes the tab mid-import and comes back, the onboarding status endpoint sees `email_count > 0` and skips past sync to the addresses step — even though the background sync may still be running. This means partial address discovery.

Fix: status endpoint should detect an in-progress sync and send the user back to the sync step with the progress bar reconnecting. Ties into the job queue plan (SCALING_SYNC.md) — a `sync_jobs` row with status would make this trivial to check.

### Auto-trigger sync during onboarding
Skip the "start import" button on the sync step. Once the user proceeds past domain selection, auto-start the sync immediately. Show "Hold tight..." with the progress bar. One less decision point — they've already committed by selecting domains.

### Reply is broken — needs Gmail-style draft composer
Reply doesn't work at all. Needs a pop-up draft composer like Gmail:
- To, CC, BCC fields
- Subject (pre-filled with Re: ...)
- Rich body
- Attachments
- Send button — sends the email, shows up in Sent, but user stays on current view (inbox etc.)
- Floating/docked at bottom-right like Gmail, not a full-page modal

### Email open should be inline (Gmail-style), not navigate away
Clicking an email currently navigates to a new page, which clears selection state. Two problems:
1. **Selection lost on click** — had a bunch selected, accidentally clicked an email, lost them all. Clicking a thread row should only open it, not blow away selection.
2. **Inline thread view** — open the thread in a reading pane (right side or expand-in-place) like Gmail, with a "View full email" option. Don't navigate away from the list.

### Archive/spam/trash folder views 404
Viewing threads from archive 404s. Likely spam and trash do too. Probably missing routes or the thread detail page assumes inbox-only routing.

### Search is broken
Search doesn't work. We have a websocket connection already — could potentially leverage that for search instead of / in addition to the current REST approach. Needs investigation.

### Contact widget cards
Participant names in thread list / thread view should be clickable cards (like Gmail). Click shows: name, email address, avatar placeholder (initials). Image/gravatar support can come later but the card layout should be there.

### Webhook cleanup job
On sync or periodically, clean up stale Resend webhooks:
- List webhooks from Resend (`GET /webhooks`)
- Compare against the one stored in our DB (`resend_webhook_id`)
- Delete any that point to **our** URL pattern (e.g. `/api/webhooks/resend/`) but aren't the current one
- Never touch webhooks with endpoints we don't recognize (third-party integrations)

Prevents pile-up of stale webhooks when re-onboarding or URL changes (localhost -> staging -> production).

### Reply to specific email in thread (not just latest)
Currently reply is thread-level — always replies to the latest message. Need the ability to reply to a specific email within a thread. This means:
- UI: click "reply" on any individual email in the thread view, not just the bottom
- Set `In-Reply-To` and `References` headers against that specific email's `message_id`
- Pre-fill `To` from that email's sender, allow adding CC/BCC
- Gmail does this — each message in a thread has its own reply button

### Outbound emails missing display name in From field
We send raw `hello@cx.agency` with no display name. Should use `"Display Name <address>"` format. Logic:
- Alias addresses (hello@, support@) → org/company name → `"CX Agency <hello@cx.agency>"`
- Personal addresses (harrison@) → user's full name → `"Harrison <harrison@cx.agency>"`

### CC'd emails dropped on inbound
Webhook handler only checks `To` addresses when routing inbound emails to a domain. If our domain's address is in the CC field (not To), the email gets silently dropped — `"no matching domain for received email"` warning and gone. Fix: domain matching in `handleEmailReceived` should also iterate `emailData.CC`.

### Multiple TO addresses on reply/compose
Can't add multiple recipients to the TO field when replying or composing. Need a multi-recipient input (pills/tags pattern) for To, CC, and BCC fields.

## Parked

### Self-sends and cross-domain sends
Sending from one of our addresses to another (same domain or cross-domain) creates duplicates — but that's correct, there's a sender copy and a receiver copy. Currently works. The tricky part is if/when this causes issues — e.g. which thread does it land in, deduplication logic treating them as one, etc. Leave alone until it breaks.

## Done

- Inbox shows both sent and received threads correctly
- Threading bug: outbound emails with same subject but different recipients merged into one thread
- Thread actions (delete/archive/trash/spam) navigate back with optimistic updates
- Refresh button working with cache invalidation
- Settings as a modal with sidebar tabs (Profile, Domains)
- Search moved to header with inline results
