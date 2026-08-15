# Changelog

## v1.2.0

### Compose and sending

- Per-user rich HTML signatures, auto-inserted on new messages and replies
- Undo send: a 0/5/10/30 second hold with an Undo action on the send toast
- Scheduled send: pick a preset or a custom time up to 30 days out; cancel
  from the drafts list, which keeps the draft
- The schedule state lives in Postgres, so a Redis restart cannot lose it

### Inbox

- Snooze: hide a thread until a wake time; a Snoozed sidebar view lists
  active snoozes and a scheduler wakes them on time
- Custom label ordering with up/down controls in settings

### Rules

- Forwarding rules: forward inbound alias mail to another address
- Auto-replies: per-alias message with an optional time window, one reply
  per sender per 24 hours
- Org policy controls: admins allow or block forwarding, external targets,
  and auto-replies; users manage rules only for their own aliases
- Loop guards: automated senders are skipped, self-forwards rejected, and
  rule sends carry an Auto-Submitted header

### Fixes

- A move to spam now also restores the thread from trash and clears the
  purge timer
- The onboarding "skip" now parks the alias instead of leaving it assigned
- New passwords are checked against the Pwned Passwords breach list
  (k-anonymity, fail-open; HIBP_CHECK_DISABLED=true turns it off)
- The domain heartbeat auto-registers a missing Resend webhook
- A cancelled send job can never run, even in a dispatch race

## v1.1.0

### Billing (commercial mode)

- Add a grace period lifecycle: 14 days for failed payments, 7 days after a cancellation
- Send a warning email to admins 3 days before a plan downgrade
- Give lapsed orgs read-only access: mail stays readable, writes and sends are blocked
- Handle Stripe `payment_action_required` (3DS) events; email admins the hosted invoice link
- Add a persistent plan-state banner and a `/billing` page in the app

### Domains

- Show the API key status (valid/invalid, last check time) in Organization settings
- On a revoked API key, mark all domains disconnected instead of failing silently
- Pass real Resend error messages through to the UI
- Delete the domain in Resend too when a domain is deleted in the app
- Import a domain that already exists in Resend instead of failing on a duplicate create
- Fix the add-domain request body (a double JSON encode broke domain creation)

### Sending reliability

- Fix retry accounting for batch sends; every failed item now retries correctly
- Add a send reconciler that finds emails stuck in `queued` and fails them with a recovery draft
- Add a retry endpoint and a "Failed" sidebar view for failed sends
- Expire bounce blocks after 30 days; block only the recipient that bounced
- Add a blocked-addresses list in settings with unblock

### Inbox and navigation

- Keep search, page, and open thread in the URL; the Back button works
- Add search pagination, a folder-scope toggle, and `from:`, `to:`, `has:attachment`, `before:`, `after:` operators
- Add shift-click range selection, "mark all read", "empty folder", and a move-to-folder menu
- Add undo toasts for archive, trash, spam, and move
- Add per-label sidebar counts (drafts, spam, failed, custom labels)
- Add drag-and-drop to sidebar folders

### Compose

- Set threading headers (In-Reply-To, References) on replies sent from drafts
- Flush unsaved compose changes before send and on window close
- Support multi-file attachment upload with progress
- Fix a draft-clobber race between autosave and send

### General

- Add desktop notification deep links to the thread
- Unify toasts on one system
- Add print styles, a PWA manifest, accessible dialogs, and a three-state theme toggle
- Add favicon.ico and a Google-compliant 96px favicon

## v1.0.1

### Security

- Rate limit health, config, and webhook endpoints
- Remove infrastructure details (db/redis status) from health endpoint response
- Sanitize inbound email HTML server-side to prevent stored XSS
- Validate webhook `orgId` parameter as UUID
- Scope draft queries by org to prevent cross-org access
- Make webhook secret decryption failure fatal (no silent plaintext fallback)
- Increase `SESSION_SECRET` minimum from 16 to 32 characters
- Remove Stripe signature header from error logs
- Replace `Cache-Control: public` with `no-store` on `/api/config`

## v1.0.0

Initial release.
