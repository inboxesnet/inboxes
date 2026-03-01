package queue

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// warnIfErr logs a warning if err is non-nil. Use for non-critical lookups that have a fallback.
func warnIfErr(err error, msg string, args ...any) {
	if err != nil {
		slog.Warn(msg, append(args, "error", err)...)
	}
}

// querier abstracts *pgxpool.Pool and pgx.Tx for label operations.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func addLabelQ(ctx context.Context, q querier, threadID, orgID, label string) error {
	_, err := q.Exec(ctx,
		`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		threadID, orgID, label)
	return err
}

func removeLabelQ(ctx context.Context, q querier, threadID, label string) error {
	_, err := q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = $1 AND label = $2`,
		threadID, label)
	return err
}

func hasLabelQ(ctx context.Context, q querier, threadID, label string) bool {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM thread_labels WHERE thread_id = $1 AND label = $2)`,
		threadID, label).Scan(&exists); err != nil {
		slog.Warn("hasLabelQ: query failed", "thread_id", threadID, "label", label, "error", err)
		return false
	}
	return exists
}
