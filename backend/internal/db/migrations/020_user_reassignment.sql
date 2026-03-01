-- +goose Up
CREATE TABLE user_reassignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    source_user_id UUID NOT NULL REFERENCES users(id),
    target_user_id UUID NOT NULL REFERENCES users(id),
    reassigned_by UUID NOT NULL REFERENCES users(id),
    threads_moved INT NOT NULL DEFAULT 0,
    aliases_moved INT NOT NULL DEFAULT 0,
    drafts_moved INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS user_reassignments;
