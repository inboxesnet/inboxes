# Changelog

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
