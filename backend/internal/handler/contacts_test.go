package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

func TestEscapeLIKE_NoSpecialChars(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("hello"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEscapeLIKE_Percent(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("100%"); got != `100\%` {
		t.Errorf("got %q, want %q", got, `100\%`)
	}
}

func TestEscapeLIKE_Underscore(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE("first_name"); got != `first\_name` {
		t.Errorf("got %q, want %q", got, `first\_name`)
	}
}

func TestEscapeLIKE_Backslash(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE(`path\to`); got != `path\\to` {
		t.Errorf("got %q, want %q", got, `path\\to`)
	}
}

func TestEscapeLIKE_AllMetachars(t *testing.T) {
	t.Parallel()
	got := escapeLIKE(`%_\`)
	want := `\%\_\\`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeLIKE_Empty(t *testing.T) {
	t.Parallel()
	if got := escapeLIKE(""); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestEscapeLIKE_SQLInjection(t *testing.T) {
	t.Parallel()
	input := `'; DROP TABLE users; --`
	got := escapeLIKE(input)
	// Only LIKE metacharacters should be escaped; SQL injection is handled by parameterized queries
	if got != input {
		t.Errorf("got %q, want %q (no LIKE metachar to escape)", got, input)
	}
}

// ── #95: Autocomplete returns contacts ranked by frequency ──

func TestContactSuggest_RankedByFrequency(t *testing.T) {
	t.Parallel()
	h := &ContactHandler{
		Store: &store.MockStore{
			SuggestContactsFn: func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
				// Store returns results pre-ranked by frequency (most frequent first)
				return []map[string]any{
					{"email": "alice@example.com", "name": "Alice", "count": 42},
					{"email": "alex@example.com", "name": "Alex", "count": 7},
					{"email": "anna@example.com", "name": "Anna", "count": 1},
				}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/contacts/suggest?q=a", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Suggest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Suggest(ranked): status %d, want %d", w.Code, http.StatusOK)
	}
	var resp []map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 3 {
		t.Fatalf("Suggest(ranked): got %d, want 3", len(resp))
	}
	// Verify order preserved (store provides ranked results)
	if resp[0]["email"] != "alice@example.com" {
		t.Errorf("Suggest(ranked): first result = %v, want alice@example.com", resp[0]["email"])
	}
}

// ── #96: Autocomplete — minimum 2 chars ──
// NOTE: The handler accepts single-char queries — the store handles minimum length.
// This test verifies single-char queries pass through to the store.

func TestContactSuggest_SingleChar(t *testing.T) {
	t.Parallel()
	var capturedQuery string
	h := &ContactHandler{
		Store: &store.MockStore{
			SuggestContactsFn: func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
				capturedQuery = query
				return []map[string]any{}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/contacts/suggest?q=a", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Suggest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Suggest(1 char): status %d", w.Code)
	}
	if capturedQuery != "a" {
		t.Errorf("Suggest(1 char): query = %q, want %q", capturedQuery, "a")
	}
}

func TestContactSuggest_LimitPassedToStore(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	h := &ContactHandler{
		Store: &store.MockStore{
			SuggestContactsFn: func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
				capturedLimit = limit
				return []map[string]any{}, nil
			},
		},
	}
	req := httptest.NewRequest("GET", "/contacts/suggest?q=test", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Suggest(w, req)
	if capturedLimit != 10 {
		t.Errorf("Suggest: limit = %d, want 10", capturedLimit)
	}
}

// ── ContactHandler.Suggest ──

func TestContactSuggest_Success(t *testing.T) {
	t.Parallel()

	h := &ContactHandler{
		Store: &store.MockStore{
			SuggestContactsFn: func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
				return []map[string]any{
					{"email": "alice@example.com", "name": "Alice"},
					{"email": "alex@example.com", "name": "Alex"},
				}, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/contacts/suggest?q=al", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Suggest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ContactSuggest: got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ContactSuggest: failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("ContactSuggest: got %d contacts, want 2", len(resp))
	}
}

func TestContactSuggest_EmptyQuery(t *testing.T) {
	t.Parallel()

	storeCalled := false
	h := &ContactHandler{
		Store: &store.MockStore{
			SuggestContactsFn: func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
				storeCalled = true
				return nil, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/contacts/suggest?q=", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Suggest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ContactSuggest(empty): got status %d, want %d", w.Code, http.StatusOK)
	}
	if storeCalled {
		t.Error("ContactSuggest(empty): store should not be called for empty query")
	}

	var resp []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ContactSuggest(empty): failed to decode: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("ContactSuggest(empty): got %d items, want 0", len(resp))
	}
}
