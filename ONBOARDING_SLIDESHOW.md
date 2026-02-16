# Onboarding Slideshow — Import Tips

## Overview

During the email import step, show an auto-rotating slideshow of product tips below a compact progress indicator. Keeps users engaged during what could be a 30-60+ second wait.

## Layout

```
┌──────────────────────────────────────────────┐
│  Importing emails...  ████████░░░  124/312   │  ← compact, single line
├──────────────────────────────────────────────┤
│                                              │
│        [illustration / screenshot]           │
│                                              │
│    "Did you know?"                           │
│    Inboxes automatically threads your        │
│    conversations — replies, forwards,        │
│    and CCs all in one view.                  │
│                                              │
│            ○  ●  ○  ○  ○                     │  ← dot indicators
│                                              │
└──────────────────────────────────────────────┘
```

- Progress bar is **compact** — single line, minimal margin between it and the slideshow
- Slideshow takes up the main card area
- Auto-rotates every **6 seconds**
- Dot indicators at bottom show position, clickable to jump
- Smooth fade/slide transition between tips

## Tip Content

Each tip has:
- **Illustration**: product screenshot, diagram, or custom graphic (can start with styled Lucide icon compositions, upgrade to real illustrations later)
- **Headline**: short hook ("Did you know?" / "Pro tip" / feature name)
- **Body**: 1-2 sentences explaining the feature/benefit

### Tip 1: Threaded Conversations
- **Illustration**: Screenshot of a thread view with multiple messages stacked
- **Headline**: Threaded conversations
- **Body**: Every reply, forward, and CC gets grouped into one thread — just like you'd expect.

### Tip 2: Multi-Domain Inbox
- **Illustration**: Discord-style sidebar with multiple domain icons
- **Headline**: All your domains, one place
- **Body**: Switch between domains instantly. Each one gets its own inbox, sent, and spam folders.

### Tip 3: Team Collaboration
- **Illustration**: Diagram showing email routing to different team members
- **Headline**: Built for teams
- **Body**: Invite your team, assign addresses, and set up shared aliases. Everyone sees what's theirs.

### Tip 4: Aliases & Routing
- **Illustration**: Diagram: support@domain → fans out to 3 team members
- **Headline**: Smart email routing
- **Body**: Create aliases like support@ or hello@ and route incoming mail to the right people automatically.

### Tip 5: Real-Time Updates
- **Illustration**: Browser notification / WebSocket lightning bolt
- **Headline**: Instant notifications
- **Body**: New emails appear in real-time — no refresh needed. You'll never miss an incoming message.

### Tip 6: Spam Protection
- **Illustration**: Shield icon with spam email being filtered
- **Headline**: Built-in spam filtering
- **Body**: Suspicious emails get caught automatically. SPF, DKIM, and content analysis keep your inbox clean.

### Tip 7: Keyboard Shortcuts (Future)
- **Illustration**: Keyboard with highlighted keys
- **Headline**: Work at the speed of thought
- **Body**: Archive, reply, navigate — all from your keyboard. Power users welcome.

### Tip 8: Your Data, Your Key
- **Illustration**: Lock/key icon with Resend logo
- **Headline**: You own your data
- **Body**: We use YOUR Resend API key. Your emails stay in your Resend account — we're just the interface.

## Implementation Notes

- Tips array lives in a constant — easy to add/remove/reorder
- Start with Lucide icon compositions as placeholder illustrations
- Replace with real screenshots/illustrations once the product UI is more stable
- Slideshow only renders during active import (when `syncProgress` exists and `syncResult` is null)
- On import complete, slideshow fades out and the result summary takes over
- Transitions: CSS `opacity` + `transform` fade-slide, 300ms ease-out
