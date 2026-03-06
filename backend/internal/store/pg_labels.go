package store

import (
	"context"
	"log/slog"
)

func (s *PgStore) ListOrgLabels(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, name, created_at FROM org_labels WHERE org_id = $1 ORDER BY name`,
		orgID)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) CreateOrgLabel(ctx context.Context, orgID, name string) (string, error) {
	var id string
	err := s.q.QueryRow(ctx,
		`INSERT INTO org_labels (org_id, name) VALUES ($1, $2)
		 ON CONFLICT (org_id, name) DO NOTHING RETURNING id`,
		orgID, name).Scan(&id)
	return id, err
}

// RenameOrgLabel renames an org label and updates all thread_labels references.
// Must be called inside a transaction (caller uses store.WithTx).
func (s *PgStore) RenameOrgLabel(ctx context.Context, labelID, orgID, newName string) (string, error) {
	// Lock the org_label row to prevent concurrent renames
	var oldName string
	if err := s.q.QueryRow(ctx,
		`SELECT name FROM org_labels WHERE id = $1 AND org_id = $2 FOR UPDATE`,
		labelID, orgID).Scan(&oldName); err != nil {
		return "", err
	}

	// Update org_labels
	if _, err := s.q.Exec(ctx,
		`UPDATE org_labels SET name = $1 WHERE id = $2 AND org_id = $3`,
		newName, labelID, orgID); err != nil {
		slog.Error("labels: rename org_labels failed", "label_id", labelID, "error", err)
		return "", err
	}

	// Rename in thread_labels
	if _, err := s.q.Exec(ctx,
		`UPDATE thread_labels SET label = $1 WHERE org_id = $2 AND label = $3`,
		newName, orgID, oldName); err != nil {
		slog.Error("labels: rename thread_labels failed", "old_name", oldName, "new_name", newName, "error", err)
		return "", err
	}

	return oldName, nil
}

// DeleteOrgLabel deletes an org label and removes it from all thread_labels.
// Must be called inside a transaction (caller uses store.WithTx).
func (s *PgStore) DeleteOrgLabel(ctx context.Context, labelID, orgID string) (string, error) {
	var labelName string
	if err := s.q.QueryRow(ctx,
		`DELETE FROM org_labels WHERE id = $1 AND org_id = $2 RETURNING name`,
		labelID, orgID).Scan(&labelName); err != nil {
		return "", err
	}

	// Remove from all threads
	if _, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE org_id = $1 AND label = $2`,
		orgID, labelName); err != nil {
		slog.Error("labels: delete thread_labels failed", "label", labelName, "error", err)
		return "", err
	}

	return labelName, nil
}
