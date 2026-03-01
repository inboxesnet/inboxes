-- +goose Up
CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    user_id UUID NOT NULL REFERENCES users(id),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attachments_org ON attachments(org_id);

ALTER TABLE drafts ADD COLUMN attachment_ids JSONB DEFAULT '[]';

-- +goose Down
ALTER TABLE drafts DROP COLUMN attachment_ids;

DROP INDEX idx_attachments_org;

DROP TABLE attachments;
