-- +goose Up
ALTER TABLE orgs ADD COLUMN forwarding_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE orgs ADD COLUMN auto_reply_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE orgs ADD COLUMN external_forwarding_allowed BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE forwarding_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    user_id UUID NOT NULL REFERENCES users(id),
    alias_id UUID NOT NULL REFERENCES aliases(id),
    forward_to TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(alias_id, forward_to)
);
CREATE INDEX idx_forwarding_rules_org ON forwarding_rules(org_id) WHERE enabled;

CREATE TABLE auto_replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    user_id UUID NOT NULL REFERENCES users(id),
    alias_id UUID NOT NULL UNIQUE REFERENCES aliases(id),
    subject TEXT NOT NULL DEFAULT '',
    body_html TEXT NOT NULL DEFAULT '',
    body_plain TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One reply per sender per rule per 24 hours.
CREATE TABLE auto_reply_log (
    auto_reply_id UUID NOT NULL REFERENCES auto_replies(id) ON DELETE CASCADE,
    sender TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (auto_reply_id, sender)
);

-- +goose Down
DROP TABLE auto_reply_log;
DROP TABLE auto_replies;
DROP TABLE forwarding_rules;
ALTER TABLE orgs DROP COLUMN external_forwarding_allowed;
ALTER TABLE orgs DROP COLUMN auto_reply_enabled;
ALTER TABLE orgs DROP COLUMN forwarding_enabled;
