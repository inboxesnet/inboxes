-- +goose Up
-- Time of the last refresh-token rotation. A superseded refresh token that
-- arrives shortly after the rotation is a client race or a retry, not a
-- replay. The server rejects it but does not revoke the token.
ALTER TABLE agent_tokens ADD COLUMN refresh_rotated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE agent_tokens DROP COLUMN refresh_rotated_at;
