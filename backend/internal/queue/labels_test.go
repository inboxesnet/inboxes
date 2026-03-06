package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- mock infrastructure ---

type execCall struct {
	sql  string
	args []interface{}
}

type mockQuerier struct {
	execCalls []execCall
	execErr   error
	row       *mockRow
}

func (m *mockQuerier) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, execCall{sql: sql, args: args})
	return pgconn.NewCommandTag(""), m.execErr
}

func (m *mockQuerier) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	m.execCalls = append(m.execCalls, execCall{sql: sql, args: args})
	return m.row
}

func (m *mockQuerier) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

type mockRow struct {
	val interface{}
	err error
}

func (r *mockRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if b, ok := dest[0].(*bool); ok {
			if v, ok := r.val.(bool); ok {
				*b = v
			}
		}
	}
	return nil
}

// --- addLabelQ tests ---

func TestAddLabelQ_Success(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{}
	err := addLabelQ(context.Background(), m, "thread-1", "org-1", "inbox")
	if err != nil {
		t.Fatalf("addLabelQ: %v", err)
	}
	if len(m.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(m.execCalls))
	}
	call := m.execCalls[0]
	if !strings.Contains(call.sql, "INSERT INTO thread_labels") {
		t.Errorf("SQL should contain INSERT INTO thread_labels, got %q", call.sql)
	}
	if !strings.Contains(call.sql, "ON CONFLICT DO NOTHING") {
		t.Errorf("SQL should contain ON CONFLICT DO NOTHING, got %q", call.sql)
	}
	if len(call.args) != 3 || call.args[0] != "thread-1" || call.args[1] != "org-1" || call.args[2] != "inbox" {
		t.Errorf("args: got %v, want [thread-1 org-1 inbox]", call.args)
	}
}

func TestAddLabelQ_ExecError(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{execErr: errors.New("db down")}
	err := addLabelQ(context.Background(), m, "t1", "o1", "inbox")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "db down" {
		t.Errorf("got %q, want %q", err.Error(), "db down")
	}
}

// --- removeLabelQ tests ---

func TestRemoveLabelQ_Success(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{}
	err := removeLabelQ(context.Background(), m, "thread-1", "trash")
	if err != nil {
		t.Fatalf("removeLabelQ: %v", err)
	}
	if len(m.execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(m.execCalls))
	}
	call := m.execCalls[0]
	if !strings.Contains(call.sql, "DELETE FROM thread_labels") {
		t.Errorf("SQL should contain DELETE FROM thread_labels, got %q", call.sql)
	}
	if !strings.Contains(call.sql, "WHERE") {
		t.Errorf("SQL should contain WHERE, got %q", call.sql)
	}
	if len(call.args) != 2 || call.args[0] != "thread-1" || call.args[1] != "trash" {
		t.Errorf("args: got %v, want [thread-1 trash]", call.args)
	}
}

func TestRemoveLabelQ_ExecError(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{execErr: errors.New("db down")}
	err := removeLabelQ(context.Background(), m, "t1", "inbox")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- hasLabelQ tests ---

func TestHasLabelQ_True(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{row: &mockRow{val: true}}
	if !hasLabelQ(context.Background(), m, "thread-1", "inbox") {
		t.Error("expected true, got false")
	}
	call := m.execCalls[0]
	if !strings.Contains(call.sql, "EXISTS") {
		t.Errorf("SQL should contain EXISTS, got %q", call.sql)
	}
}

func TestHasLabelQ_False(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{row: &mockRow{val: false}}
	if hasLabelQ(context.Background(), m, "thread-1", "inbox") {
		t.Error("expected false, got true")
	}
}

func TestHasLabelQ_ScanError(t *testing.T) {
	t.Parallel()
	m := &mockQuerier{row: &mockRow{err: errors.New("scan err")}}
	if hasLabelQ(context.Background(), m, "thread-1", "inbox") {
		t.Error("expected false on scan error, got true")
	}
}
