-- +goose Up
-- MCP agent access: bearer tokens, OAuth clients/codes, and the org-level
-- send switch. Agents get drafts by default; send stays off until an admin
-- turns it on.
ALTER TABLE orgs ADD COLUMN agent_send_enabled BOOLEAN NOT NULL DEFAULT false;

-- One row per credential an agent can present. kind 'key' rows are
-- user-created API keys; kind 'oauth' rows are OAuth access tokens.
-- Raw tokens are never stored — only SHA-256 hashes.
CREATE TABLE agent_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('key', 'oauth')),
    name TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    refresh_hash TEXT UNIQUE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_agent_tokens_user ON agent_tokens(user_id);
CREATE INDEX idx_agent_tokens_org ON agent_tokens(org_id);

-- Dynamically registered OAuth clients (RFC 7591). Public clients only —
-- no client secret; PKCE is mandatory at the token endpoint.
CREATE TABLE oauth_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL DEFAULT '',
    redirect_uris JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Single-use authorization codes, stored hashed, short-lived.
CREATE TABLE oauth_codes (
    code_hash TEXT PRIMARY KEY,
    client_id UUID NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    redirect_uri TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE oauth_codes;
DROP TABLE oauth_clients;
DROP TABLE agent_tokens;
ALTER TABLE orgs DROP COLUMN agent_send_enabled;
