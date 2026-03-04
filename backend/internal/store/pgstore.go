package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore implements Store using a PostgreSQL connection pool.
type PgStore struct {
	pool *pgxpool.Pool
	q    Querier // pool (default) or tx (inside WithTx)
}

// NewPgStore creates a PgStore backed by the given pool.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool, q: pool}
}

// Pool returns the underlying connection pool.
func (s *PgStore) Pool() *pgxpool.Pool { return s.pool }

// Q returns the current querier (pool or tx).
func (s *PgStore) Q() Querier { return s.q }

// WithTx executes fn within a database transaction.
// If fn returns nil, the tx is committed; otherwise it is rolled back.
func (s *PgStore) WithTx(ctx context.Context, fn func(Store) error) error {
	return s.WithTxOpts(ctx, pgx.TxOptions{}, fn)
}

// WithTxOpts executes fn within a transaction with the given options.
func (s *PgStore) WithTxOpts(ctx context.Context, opts pgx.TxOptions, fn func(Store) error) error {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txStore := &PgStore{pool: s.pool, q: tx}
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// scanMaps collects all rows into []map[string]any using pgx.RowToMap.
// Post-processes UUID values: pgx returns [16]byte for uuid columns which
// JSON-marshals as a number array. This converts them to proper UUID strings.
func scanMaps(rows pgx.Rows) ([]map[string]any, error) {
	result, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = []map[string]any{}
	}
	for _, m := range result {
		for k, v := range m {
			if b, ok := v.([16]byte); ok {
				m[k] = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
			}
		}
	}
	return result, nil
}

// warnIfErr logs a warning if err is non-nil. Used for non-critical lookups.
func warnIfErr(err error, msg string, args ...any) {
	if err != nil {
		slog.Warn(msg, append(args, "error", err)...)
	}
}

// Ensure PgStore compiles as a Querier (it delegates to its pool/tx).
var _ Querier = (*pgxpool.Pool)(nil)
