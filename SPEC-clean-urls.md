# Spec: Remove UUIDs from User-Facing URLs

## Problem

Current URLs expose internal UUIDs:
```
/d/550e8400-e29b-41d4-a716-446655440000/inbox/7c9e6679-7425-40de-944b-e07fc1f90ae7
```

This is ugly, leaks internal IDs, and makes URLs non-shareable/non-memorable.

## Goal

Replace UUID-based routing with human-readable slugs or short IDs:
```
/d/acme-corp/inbox/thread-subject-abc123
```

Or at minimum, use short IDs:
```
/d/acme/inbox/t/abc123
```

## Scope

### URLs to Change
- `/d/[domainId]/...` → `/d/[domainSlug]/...` (domain slug = domain name minus TLD, e.g., `acme` for `acme.com`)
- `/d/[domainId]/inbox/[threadId]` → short thread reference
- `/d/[domainId]/[label]/[threadId]` → same pattern

### Backend Changes
- Add `slug` column to domains (auto-generated from domain name, unique per org)
- Add lookup endpoints that accept slug OR UUID (backwards compat)
- Threads: generate short IDs (nanoid or base62-encoded) alongside UUID primary keys
- Add `short_id` column to threads table

### Frontend Changes
- Update all `router.push()` and `Link` href patterns
- Update `useParams()` extraction in all page components
- Update sidebar domain links
- Update keyboard shortcut navigation
- Handle redirect from old UUID URLs to new slugs (optional, nice-to-have)

### Pages Affected
- `frontend/app/(app)/d/[domainId]/` — all nested routes (inbox, sent, archive, trash, spam, search, starred, drafts, custom labels)
- `frontend/components/domain-sidebar.tsx` — link generation
- `frontend/components/thread-list.tsx` — thread click navigation
- `frontend/components/keyboard-shortcuts.tsx` — navigation targets

## Migration Strategy
1. Add slug/short_id columns with migrations
2. Backfill existing records
3. Update frontend to use new slugs
4. Optionally: keep UUID routes working as redirects

## Considerations
- Short IDs must be unique per org (not globally) to keep them short
- Domain slugs should handle collisions (e.g., `acme` vs `acme-2`)
- Reset/invite token URLs are separate concern (already short-lived, less visible)
- Thread short IDs: 8-char nanoid gives ~2.8 trillion combinations per org, more than sufficient

## Priority
Low — cosmetic/UX improvement. No security or functional impact.
