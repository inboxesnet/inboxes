-- +goose Up
CREATE TABLE org_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, name)
);

-- +goose Down
DROP TABLE org_labels;
