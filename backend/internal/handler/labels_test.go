package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inboxes/backend/internal/store"
)

func TestLabelList_Success(t *testing.T) {
	t.Parallel()

	h := &LabelHandler{
		Store: &store.MockStore{
			ListOrgLabelsFn: func(ctx context.Context, orgID string) ([]map[string]any, error) {
				return []map[string]any{
					{"id": "l1", "name": "custom1"},
					{"id": "l2", "name": "custom2"},
				}, nil
			},
		},
	}

	req := httptest.NewRequest("GET", "/labels", nil)
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("LabelList: got status %d, want %d", w.Code, http.StatusOK)
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("LabelList: failed to decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("LabelList: got %d labels, want 2", len(resp))
	}
}

func TestLabelCreate_Success(t *testing.T) {
	t.Parallel()

	h := &LabelHandler{
		Store: &store.MockStore{
			CreateOrgLabelFn: func(ctx context.Context, orgID, name string) (string, error) {
				return "l-new", nil
			},
		},
	}

	body := `{"name":"custom-label"}`
	req := httptest.NewRequest("POST", "/labels", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("LabelCreate: got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("LabelCreate: failed to decode response: %v", err)
	}
	if resp["id"] != "l-new" {
		t.Errorf("LabelCreate: got id %q, want %q", resp["id"], "l-new")
	}
	if resp["name"] != "custom-label" {
		t.Errorf("LabelCreate: got name %q, want %q", resp["name"], "custom-label")
	}
}

func TestLabelCreate_SystemName(t *testing.T) {
	t.Parallel()

	h := &LabelHandler{
		Store: &store.MockStore{},
	}

	for _, name := range []string{"inbox", "sent", "trash", "spam", "starred", "archive", "drafts"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest("POST", "/labels", strings.NewReader(body))
		req = withClaims(req, "user1", "org1", "admin")
		w := httptest.NewRecorder()

		h.Create(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("LabelCreate(%q): got status %d, want %d", name, w.Code, http.StatusBadRequest)
		}
		if !strings.Contains(w.Body.String(), "cannot use system label name") {
			t.Errorf("LabelCreate(%q): body = %q", name, w.Body.String())
		}
	}
}

func TestLabelRename_Success(t *testing.T) {
	t.Parallel()

	h := &LabelHandler{
		Store: &store.MockStore{
			RenameOrgLabelFn: func(ctx context.Context, labelID, orgID, newName string) (string, error) {
				return "renamed", nil
			},
		},
	}

	body := `{"name":"renamed-label"}`
	req := httptest.NewRequest("PATCH", "/labels/l1", strings.NewReader(body))
	req = req.WithContext(newChiRouteContext("id", "l1"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()

	h.Rename(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("LabelRename: got status %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("LabelRename: failed to decode response: %v", err)
	}
	if resp["id"] != "l1" {
		t.Errorf("LabelRename: got id %q, want %q", resp["id"], "l1")
	}
	if resp["name"] != "renamed-label" {
		t.Errorf("LabelRename: got name %q, want %q", resp["name"], "renamed-label")
	}
}

func TestLabelCreate_TooLong(t *testing.T) {
	t.Parallel()
	h := &LabelHandler{
		Store: &store.MockStore{},
	}
	longName := strings.Repeat("a", 101)
	body := `{"name":"` + longName + `"}`
	req := httptest.NewRequest("POST", "/labels", strings.NewReader(body))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Create(too long): got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLabelDelete_Success(t *testing.T) {
	t.Parallel()
	h := &LabelHandler{
		Store: &store.MockStore{
			WithTxFn: func(ctx context.Context, fn func(store.Store) error) error {
				return fn(&store.MockStore{
					DeleteOrgLabelFn: func(ctx context.Context, labelID, orgID string) (string, error) {
						return "deleted-label", nil
					},
				})
			},
		},
	}
	req := httptest.NewRequest("DELETE", "/labels/l1", nil)
	req = req.WithContext(newChiRouteContext("id", "l1"))
	req = withClaims(req, "user1", "org1", "admin")
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Delete: got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}
