-- +goose Up
-- Refresh-token reuse detection. When a refresh token rotates, the old hash
-- moves to prev_refresh_hash. If a superseded token is ever presented again,
-- the server detects the replay and revokes the token, per OAuth 2.1.
ALTER TABLE agent_tokens ADD COLUMN prev_refresh_hash TEXT;
CREATE INDEX idx_agent_tokens_prev_refresh_hash ON agent_tokens (prev_refresh_hash) WHERE prev_refresh_hash IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agent_tokens_prev_refresh_hash;
ALTER TABLE agent_tokens DROP COLUMN prev_refresh_hash;
