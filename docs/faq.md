# FAQ

## BCC — Why can't I see that I was BCC'd?

**Short answer:** This is a limitation of how email (SMTP) and Resend work, not a bug.

When you send an email with BCC recipients through Inboxes, the BCC field is stored and displayed correctly on the **sender's side**. You'll see a "Bcc:" line on outbound emails you sent.

However, on the **receiving side**, BCC recipients will not see any indication that they were BCC'd. The email will appear as if it was sent directly to them (their address shows in the To field). This happens because:

1. **SMTP strips BCC headers before delivery.** This is the fundamental design of BCC — the receiving mail server never sees the BCC header. That's what makes it "blind."
2. **Resend's received emails API returns the envelope recipient as the `to` field.** When a BCC copy is delivered to your domain, Resend reports the BCC address as the TO recipient. The original TO/BCC context from the sender is not available.

**What works:**
- Sending emails with BCC recipients — stored and displayed correctly
- Reply-all from a BCC'd email — correctly replies only to the sender (won't expose you to other recipients)

**What doesn't work:**
- Seeing "Bcc: you" on emails where you were BCC'd — the data simply isn't available from the mail provider
- Seeing the original TO recipients on a BCC'd copy — the received copy only shows your address

This behavior is consistent with how Gmail, Outlook, and other major email clients handle BCC.

## I enabled desktop notifications but they're not showing up

**Most likely cause:** You clicked "Enable" on the in-app prompt, but then clicked "Don't Allow" (or "Block") on the browser's permission dialog. Once denied, the browser permanently blocks notifications for the site and won't ask again.

**To fix it:**

1. Open your browser's site settings for Inboxes:
   - **Chrome:** Click the lock/tune icon in the address bar → Site settings → Notifications → set to "Allow"
   - **Safari:** Safari → Settings → Websites → Notifications → find the site → set to "Allow"
   - **Firefox:** Click the lock icon in the address bar → Connection secure → More information → Permissions → Notifications → clear the "Block" setting
2. Hard refresh the page (Cmd+Shift+R / Ctrl+Shift+R)
3. Go to Settings → Profile → Notifications and check the "Desktop notifications" box

**To verify your current state**, open the browser console (Cmd+Option+J / Ctrl+Shift+J) and run:

```
Notification.permission
```

- `"granted"` — notifications should be working. If they're still not showing, check that your OS allows notifications for this browser (System Settings → Notifications).
- `"denied"` — follow the steps above to reset.
- `"default"` — the browser hasn't been asked yet. Check the "Desktop notifications" box in Settings to trigger the prompt.

**Incognito / private browsing:** Desktop notifications may not work in incognito windows. Some browsers expose the Notification API but silently deny permission requests. If you're having trouble, try a regular browser window instead.
