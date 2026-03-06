package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type LabelHandler struct {
	Store store.Store
}

var systemLabels = map[string]bool{
	"inbox": true, "sent": true, "trash": true, "spam": true,
	"starred": true, "archive": true, "drafts": true,
}

// isReservedLabel returns true for system labels and internal "alias:" prefixed labels.
func isReservedLabel(label string) bool {
	return systemLabels[label] || strings.HasPrefix(label, "alias:")
}

func (h *LabelHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	labels, err := h.Store.ListOrgLabels(r.Context(), claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list labels")
		return
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
	if isReservedLabel(req.Name) {
		writeError(w, http.StatusBadRequest, "cannot use system label name")
		return
	}

	id, err := h.Store.CreateOrgLabel(r.Context(), claims.OrgID, req.Name)
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
	if isReservedLabel(req.Name) {
		writeError(w, http.StatusBadRequest, "cannot use system label name")
		return
	}

	var renameErr error
	err := h.Store.WithTx(r.Context(), func(tx store.Store) error {
		_, renameErr = tx.RenameOrgLabel(r.Context(), labelID, claims.OrgID, req.Name)
		return renameErr
	})
	if renameErr != nil {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename label")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": labelID, "name": req.Name})
}

func (h *LabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	labelID := chi.URLParam(r, "id")

	var deleteErr error
	err := h.Store.WithTx(r.Context(), func(tx store.Store) error {
		_, deleteErr = tx.DeleteOrgLabel(r.Context(), labelID, claims.OrgID)
		return deleteErr
	})
	if deleteErr != nil {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
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
