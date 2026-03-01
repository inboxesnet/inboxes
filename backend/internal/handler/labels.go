package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LabelHandler struct {
	DB *pgxpool.Pool
}

var systemLabels = map[string]bool{
	"inbox": true, "sent": true, "trash": true, "spam": true,
	"starred": true, "archive": true, "drafts": true,
}

func (h *LabelHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, name, created_at FROM org_labels WHERE org_id = $1 ORDER BY name`,
		claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list labels")
		return
	}
	defer rows.Close()

	var labels []map[string]interface{}
	for rows.Next() {
		var id, name string
		var createdAt interface{}
		if rows.Scan(&id, &name, &createdAt) == nil {
			labels = append(labels, map[string]interface{}{
				"id": id, "name": name, "created_at": createdAt,
			})
		}
	}

	if labels == nil {
		labels = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, labels)
}

func (h *LabelHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validateLength(req.Name, "label name", 100); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if systemLabels[req.Name] {
		writeError(w, http.StatusBadRequest, "cannot use system label name")
		return
	}

	var id string
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO org_labels (org_id, name) VALUES ($1, $2)
		 ON CONFLICT (org_id, name) DO NOTHING RETURNING id`,
		claims.OrgID, req.Name).Scan(&id)
	if err != nil {
		writeError(w, http.StatusConflict, "label already exists")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "name": req.Name})
}

func (h *LabelHandler) Rename(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	labelID := chi.URLParam(r, "id")

	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validateLength(req.Name, "label name", 100); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if systemLabels[req.Name] {
		writeError(w, http.StatusBadRequest, "cannot use system label name")
		return
	}

	ctx := r.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename label")
		return
	}
	defer tx.Rollback(ctx)

	// Lock the org_label row to prevent concurrent renames
	var oldName string
	if err := tx.QueryRow(ctx,
		`SELECT name FROM org_labels WHERE id = $1 AND org_id = $2 FOR UPDATE`,
		labelID, claims.OrgID).Scan(&oldName); err != nil {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}

	// Update org_labels
	if _, err := tx.Exec(ctx,
		`UPDATE org_labels SET name = $1 WHERE id = $2 AND org_id = $3`,
		req.Name, labelID, claims.OrgID); err != nil {
		slog.Error("labels: rename org_labels failed", "label_id", labelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to rename label")
		return
	}

	// Rename in thread_labels
	if _, err := tx.Exec(ctx,
		`UPDATE thread_labels SET label = $1 WHERE org_id = $2 AND label = $3`,
		req.Name, claims.OrgID, oldName); err != nil {
		slog.Error("labels: rename thread_labels failed", "old_name", oldName, "new_name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to rename label")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename label")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": labelID, "name": req.Name})
}

func (h *LabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	labelID := chi.URLParam(r, "id")

	ctx := r.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete label")
		return
	}
	defer tx.Rollback(ctx)

	var labelName string
	if err := tx.QueryRow(ctx,
		`DELETE FROM org_labels WHERE id = $1 AND org_id = $2 RETURNING name`,
		labelID, claims.OrgID).Scan(&labelName); err != nil {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}

	// Remove from all threads
	if _, err := tx.Exec(ctx,
		`DELETE FROM thread_labels WHERE org_id = $1 AND label = $2`,
		claims.OrgID, labelName); err != nil {
		slog.Error("labels: delete thread_labels failed", "label", labelName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete label")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete label")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Querier abstracts *pgxpool.Pool and pgx.Tx so label helpers work in both contexts.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func addLabel(ctx context.Context, q Querier, threadID, orgID, label string) error {
	_, err := q.Exec(ctx,
		`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		threadID, orgID, label)
	return err
}

func removeLabel(ctx context.Context, q Querier, threadID, label string) error {
	_, err := q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = $1 AND label = $2`,
		threadID, label)
	return err
}

func removeAllLabels(ctx context.Context, q Querier, threadID string) error {
	_, err := q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = $1`,
		threadID)
	return err
}

func getLabels(ctx context.Context, q Querier, threadID string) []string {
	rows, err := q.Query(ctx,
		`SELECT label FROM thread_labels WHERE thread_id = $1 ORDER BY label`, threadID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var l string
		if rows.Scan(&l) == nil {
			labels = append(labels, l)
		}
	}
	if labels == nil {
		labels = []string{}
	}
	return labels
}

func hasLabel(ctx context.Context, q Querier, threadID, label string) bool {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM thread_labels WHERE thread_id = $1 AND label = $2)`,
		threadID, label).Scan(&exists); err != nil {
		slog.Warn("hasLabel: query failed", "thread_id", threadID, "label", label, "error", err)
		return false
	}
	return exists
}

func bulkAddLabel(ctx context.Context, q Querier, threadIDs []string, orgID, label string) error {
	_, err := q.Exec(ctx,
		`INSERT INTO thread_labels (thread_id, org_id, label)
		 SELECT unnest($1::uuid[]), $2, $3
		 ON CONFLICT DO NOTHING`,
		threadIDs, orgID, label)
	return err
}

func bulkRemoveLabel(ctx context.Context, q Querier, threadIDs []string, label string) error {
	_, err := q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = ANY($1::uuid[]) AND label = $2`,
		threadIDs, label)
	return err
}
