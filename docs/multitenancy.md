# Multi-tenant workspaces

Inboxes separates sign-in identity from workspace membership. `accounts` stores
the global email, password, and verification state. Each `users` row remains a
workspace-scoped membership with its own role, status, mailbox assignments, and
stable ID. Mail, domains, aliases, drafts, automation, billing, and integrations
remain owned by an `org_id`.

## Create a workspace

Use **Sign up** and complete the short workspace wizard. A new email creates an
account and its first workspace. An existing email creates another workspace
only after its current account password is verified. Signed-in users can open
the same wizard from the sidebar; it creates the workspace under the current
account. That workspace starts with its own billing and Resend configuration;
its Resend API key is not copied from any other workspace. Connect a key during
onboarding or in organization settings.

New email addresses still need email verification in hosted mode. Self-hosted
instances retain their one-time setup and closed-signup behavior.

## Invite a member

Inviting an email already used in another workspace creates a separate
membership linked to the existing account. The invitee accepts the invitation
without creating another password, then sees that workspace in the workspace
selector. New invitees set an account password while accepting their first
invitation.

## Switch workspaces

The sidebar selector lists active memberships for the signed-in account, oldest
workspace first. Login also asks the user to choose a workspace when the account
has more than one active membership. Switching issues a new tenant-scoped
session, then reloads the app so cached data and the WebSocket reconnect under
the selected tenant.

The backend validates the account-to-membership relationship before issuing a
session and validates the `(user_id, org_id)` pair on protected requests. A
membership in one workspace does not authorize access to another workspace's
data.

## Existing installations

Migration `032_accounts_and_org_memberships.sql` creates an account from each
existing non-placeholder user and links that user's existing membership. User
IDs and tenant data remain intact. The migration stops if existing emails
collide when compared case-insensitively, so an operator can resolve ambiguous
identities before deployment instead of merging them automatically.
