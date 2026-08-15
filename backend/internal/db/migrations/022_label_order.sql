-- +goose Up
ALTER TABLE org_labels ADD COLUMN display_order INT NOT NULL DEFAULT 0;

-- Initialize existing labels in their current (alphabetical) order.
WITH ranked AS (
    SELECT id, row_number() OVER (PARTITION BY org_id ORDER BY name) - 1 AS rn
    FROM org_labels
)
UPDATE org_labels SET display_order = ranked.rn
FROM ranked WHERE org_labels.id = ranked.id;

-- +goose Down
ALTER TABLE org_labels DROP COLUMN display_order;
