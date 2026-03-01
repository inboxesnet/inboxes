# Feature Request: React Email Templates & Drop-in Components

## Overview

Integrate [React Email](https://react.email) into the Inboxes project to enable branded, responsive email templates built with React + Tailwind — the same stack we already use on the frontend.

React Email is built by the Resend team (our existing email provider), so the integration is first-class: `render()` a React component to HTML, pass it to Resend's `html` param. Done.

## What We Have Today

- **Backend**: Go + Resend API integration (`backend/internal/service/resend.go`) — sends raw HTML/text
- **Frontend**: Next.js 15, React 19, Tailwind, shadcn/ui
- **Compose flow**: Users compose and send emails through the UI, relayed via Resend
- **Webhooks**: Inbound email handling via Resend webhooks

## What React Email Gives Us

### 1. Pre-built, Email-Client-Safe Components
Battle-tested across Gmail, Outlook, Apple Mail, Yahoo, Superhuman, HEY:
- `<Button>`, `<Link>`, `<Img>`, `<Section>`, `<Column>`, `<Row>`
- `<Heading>`, `<Text>`, `<Hr>`, `<Preview>` (inbox preview text)
- `<Html>`, `<Head>`, `<Body>`, `<Container>` (document structure)
- Full Tailwind CSS support via `@react-email/tailwind`

### 2. Template System
React components as email templates — props-driven, composable, reusable:
```tsx
<WelcomeEmail userName="Dimon" teamName="Acme" />
<ThreadNotification subject="Re: Launch plan" snippet="Looks great, let's..." />
<InviteEmail inviterName="Sarah" role="admin" acceptUrl="https://..." />
```

### 3. Drop-in Components (the dream)
Reusable building blocks users can mix and match inside templates:
- **Header** — logo, brand colors, navigation links
- **Footer** — unsubscribe link, social icons, legal text
- **CTA Block** — styled button with heading + subtext
- **Quote/Reply Block** — styled previous message for thread context
- **Signature Block** — name, title, avatar, contact info
- **Alert/Banner** — info, warning, success states
- **Table** — structured data (order confirmations, reports, etc.)

Users (or we, in the admin UI) could eventually assemble templates from these blocks like building with Lego.

### 4. Local Preview Server
`email dev` spins up a local preview UI to visually iterate on templates without actually sending emails.

### 5. Dark Mode
Built-in dark mode support — email clients that support it get automatic theming.

## Proposed Architecture

```
frontend/
  emails/                    # React Email templates
    components/              # Drop-in reusable blocks
      header.tsx
      footer.tsx
      cta-block.tsx
      signature.tsx
    templates/               # Full email templates
      welcome.tsx
      invite.tsx
      thread-notification.tsx
      password-reset.tsx
    render.ts                # render() utility wrapper
```

**Rendering options:**
- **Option A**: Render in Next.js API route, send HTML to backend → Resend
- **Option B**: Render in a shared package, backend calls a render endpoint
- **Option C**: Pre-render at build time for static templates, runtime for dynamic ones

## Use Cases

| Template | Trigger | Dynamic Props |
|----------|---------|---------------|
| Welcome | User signup | name, team |
| Team invite | Admin invites member | inviter, role, accept URL |
| Thread notification | New reply in thread | subject, snippet, sender, thread URL |
| Password reset | User requests reset | reset URL, expiry |
| Digest | Scheduled (daily/weekly) | unread count, top threads |
| Custom (user-created) | User-defined | user-defined variables |

## Email Signatures (Priority — User-Expected Feature)

Users expect to set a signature that auto-appends to outgoing emails. This fits naturally into the React Email + drop-in component architecture.

**What we need:**
- Per-alias signatures (each alias can have its own signature)
- Global/default signature fallback (used when alias has no override)
- Rich text support (name, title, phone, links, small logo/avatar)
- Signature Block component (`emails/components/signature.tsx`) already planned above
- Settings UI: text editor per alias in Settings > Aliases, plus a default in Settings > Profile
- Compose integration: signature auto-inserted at bottom of new emails and replies (above quoted text)
- Toggle: users should be able to remove/edit the signature per-email before sending

**Data model:**
- `signature_html TEXT` column on `aliases` table (per-alias)
- `default_signature_html TEXT` column on `users` table (fallback)
- Frontend fetches active alias's signature on compose open; falls back to user default

**Rendering:**
- Signature stored as HTML (rendered from the Signature Block React Email component or plain rich-text editor)
- Injected into email body at send time, separated by `--` convention
- Inbound emails: strip signature from quoted replies (best-effort, not critical)

## Future Possibilities

- **Template editor in UI** — let users customize templates visually (colors, logo, layout)
- **Template marketplace** — pre-designed templates users can install
- **Dynamic composition** — drag-and-drop email builder using the drop-in components
- **Per-domain branding** — different templates/themes per connected domain
- **A/B testing** — variant templates for outbound campaigns

## Resources

- [React Email Docs](https://react.email/docs/introduction)
- [Component Library](https://react.email/components)
- [Pre-built Templates](https://react.email/templates)
- [render() Utility](https://react.email/docs/utilities/render)
- [GitHub](https://github.com/resend/react-email)
