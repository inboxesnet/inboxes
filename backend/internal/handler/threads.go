package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

type ThreadHandler struct {
	Store store.Store
	Bus   *event.Bus
}

func (h *ThreadHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	domainID := r.URL.Query().Get("domain_id")
	label := r.URL.Query().Get("label")
	if label == "" {
		label = "inbox"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	ctx := r.Context()

	var aliasAddrs []string
	if claims.Role != "admin" {
		addrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if addrs == nil {
			addrs = []string{}
		}
		aliasAddrs = addrs
	}

	threads, total, err := h.Store.ListThreads(ctx, claims.OrgID, label, domainID, claims.Role, aliasAddrs, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch threads")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"threads": threads,
		"page":    page,
		"total":   total,
	})
}

func (h *ThreadHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	threadMap, err := h.Store.GetThread(ctx, threadID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	// Alias visibility check for non-admins
	if claims.Role != "admin" {
		aliasAddrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		aliasLabels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			aliasLabels[i] = "alias:" + addr
		}
		visible, err := h.Store.CheckThreadVisibility(ctx, threadID, aliasLabels)
		if err != nil {
			slog.Warn("threads: visibility check failed", "thread_id", threadID, "error", err)
		}
		if !visible {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
	}

	// Fetch emails
	emails, err := h.Store.GetThreadEmails(ctx, threadID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch emails")
		return
	}

	threadMap["emails"] = emails

	writeJSON(w, http.StatusOK, map[string]interface{}{"thread": threadMap})
}

func (h *ThreadHandler) BulkAction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	var req struct {
		ThreadIDs    []string `json:"thread_ids"`
		Action       string   `json:"action"`
		Label        string   `json:"label"`
		SelectAll    bool     `json:"select_all"`
		FilterLabel  string   `json:"filter_label"`
		FilterDomain string   `json:"filter_domain_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	ctx := r.Context()

	// Select-all mode: resolve thread IDs from filters
	if req.SelectAll {
		if req.FilterLabel == "" {
			req.FilterLabel = "inbox"
		}
		var aliasAddrs []string
		if claims.Role != "admin" {
			addrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
			if addrs == nil {
				addrs = []string{}
			}
			aliasAddrs = addrs
		}
		resolved, err := h.Store.ResolveFilteredThreadIDs(ctx, claims.OrgID, req.FilterLabel, req.FilterDomain, claims.Role, aliasAddrs)
		if err != nil {
			slog.Error("threads: resolve select-all IDs failed", "error", err)
			if err.Error() == "too many threads selected; please narrow your filter" {
				writeError(w, http.StatusBadRequest, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "failed to resolve threads")
			}
			return
		}
		req.ThreadIDs = resolved
	}

	if len(req.ThreadIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "updated", "affected": 0})
		return
	}

	// Alias visibility: non-admins can only act on threads they have access to
	if claims.Role != "admin" && !req.SelectAll {
		aliasAddrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		aliasLabels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			aliasLabels[i] = "alias:" + addr
		}
		var allowed []string
		for _, tid := range req.ThreadIDs {
			visible, _ := h.Store.CheckThreadVisibility(ctx, tid, aliasLabels)
			if visible {
				allowed = append(allowed, tid)
			}
		}
		req.ThreadIDs = allowed
		if len(req.ThreadIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{"message": "updated", "affected": 0})
			return
		}
	}

	var affected int64

	switch req.Action {
	case "archive":
		if err := h.Store.BulkRemoveLabel(ctx, req.ThreadIDs, "inbox"); err != nil {
			slog.Error("threads: bulk archive failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to archive threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "trash":
		if err := h.Store.WithTx(ctx, func(tx store.Store) error {
			if err := tx.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "trash"); err != nil {
				return err
			}
			return tx.SetTrashExpiry(ctx, req.ThreadIDs, claims.OrgID)
		}); err != nil {
			slog.Error("threads: bulk trash failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to trash threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "spam":
		if err := h.Store.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "spam"); err != nil {
			slog.Error("threads: bulk spam label failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as spam")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "read":
		n, err := h.Store.BulkUpdateUnread(ctx, req.ThreadIDs, claims.OrgID, 0)
		if err != nil {
			slog.Error("threads: bulk mark read failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as read")
			return
		}
		affected = n

	case "unread":
		n, err := h.Store.BulkUpdateUnread(ctx, req.ThreadIDs, claims.OrgID, 1)
		if err != nil {
			slog.Error("threads: bulk mark unread failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to mark threads as unread")
			return
		}
		affected = n

	case "move":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for move action")
			return
		}
		validSystemLabels := map[string]bool{"inbox": true, "sent": true, "archive": true, "trash": true, "spam": true}
		if !validSystemLabels[req.Label] {
			writeError(w, http.StatusBadRequest, "invalid label: "+req.Label)
			return
		}
		if err := h.Store.WithTx(ctx, func(tx store.Store) error {
			switch req.Label {
			case "inbox":
				if err := tx.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "inbox"); err != nil {
					return err
				}
				if err := tx.BulkRemoveLabel(ctx, req.ThreadIDs, "trash"); err != nil {
					return err
				}
				if err := tx.BulkRemoveLabel(ctx, req.ThreadIDs, "spam"); err != nil {
					return err
				}
				_, err := tx.Q().Exec(ctx,
					`UPDATE threads SET trash_expires_at = NULL, updated_at = now()
					 WHERE id = ANY($1::uuid[]) AND org_id = $2`, req.ThreadIDs, claims.OrgID)
				return err
			case "trash":
				if err := tx.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "trash"); err != nil {
					return err
				}
				return tx.SetTrashExpiry(ctx, req.ThreadIDs, claims.OrgID)
			case "spam":
				return tx.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "spam")
			case "archive":
				return tx.BulkRemoveLabel(ctx, req.ThreadIDs, "inbox")
			default:
				return tx.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, req.Label)
			}
		}); err != nil {
			slog.Error("threads: bulk move failed", "label", req.Label, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to move threads")
			return
		}
		affected = int64(len(req.ThreadIDs))

	case "label":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for label action")
			return
		}
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
		if err := h.Store.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, req.Label); err != nil {
			slog.Error("threads: bulk add label failed", "label", req.Label, "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "unlabel":
		if req.Label == "" {
			writeError(w, http.StatusBadRequest, "label is required for unlabel action")
			return
		}
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
		if err := h.Store.BulkRemoveLabel(ctx, req.ThreadIDs, req.Label); err != nil {
			slog.Error("threads: bulk remove label failed", "label", req.Label, "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "mute":
		if err := h.Store.BulkAddLabel(ctx, req.ThreadIDs, claims.OrgID, "muted"); err != nil {
			slog.Error("threads: bulk mute failed", "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "unmute":
		if err := h.Store.BulkRemoveLabel(ctx, req.ThreadIDs, "muted"); err != nil {
			slog.Error("threads: bulk unmute failed", "error", err)
		}
		affected = int64(len(req.ThreadIDs))

	case "delete":
		trashIDs, err := h.Store.FilterTrashThreadIDs(ctx, req.ThreadIDs)
		if err != nil {
			slog.Error("threads: bulk delete filter failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete threads")
			return
		}

		if len(trashIDs) > 0 {
			if err := h.Store.WithTx(ctx, func(tx store.Store) error {
				for _, tid := range trashIDs {
					if err := tx.RemoveAllLabels(ctx, tid); err != nil {
						return err
					}
				}
				n, err := tx.BulkSoftDelete(ctx, trashIDs, claims.OrgID)
				if err != nil {
					return err
				}
				affected = n
				return nil
			}); err != nil {
				slog.Error("threads: bulk delete failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to delete threads")
				return
			}
		}

	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadBulkAction,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		Payload: map[string]interface{}{
			"action":     req.Action,
			"thread_ids": req.ThreadIDs,
			"label":      req.Label,
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "updated",
		"affected": affected,
	})
}

func (h *ThreadHandler) Move(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())

	threadID := chi.URLParam(r, "id")

	var req struct {
		Label string `json:"label"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	ctx := r.Context()

	// Alias visibility check for non-admins
	if claims.Role != "admin" {
		aliasAddrs, _ := h.Store.GetUserAliasAddresses(ctx, claims.UserID)
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		aliasLabels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			aliasLabels[i] = "alias:" + addr
		}
		visible, err := h.Store.CheckThreadVisibility(ctx, threadID, aliasLabels)
		if err != nil {
			slog.Warn("threads: visibility check failed", "thread_id", threadID, "error", err)
		}
		if !visible {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
	}

	// Reject reserved custom labels early (system labels are handled in the switch)
	switch req.Label {
	case "inbox", "trash", "spam", "archive":
		// system labels OK
	default:
		if isReservedLabel(req.Label) {
			writeError(w, http.StatusBadRequest, "cannot use reserved label name")
			return
		}
	}

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		switch req.Label {
		case "inbox":
			if err := tx.AddLabel(ctx, threadID, claims.OrgID, "inbox"); err != nil {
				return err
			}
			if err := tx.RemoveLabel(ctx, threadID, "trash"); err != nil {
				return err
			}
			if err := tx.RemoveLabel(ctx, threadID, "spam"); err != nil {
				return err
			}
			_, err := tx.Q().Exec(ctx, "UPDATE threads SET trash_expires_at = NULL, updated_at = now() WHERE id = $1 AND org_id = $2",
				threadID, claims.OrgID)
			return err
		case "trash":
			if err := tx.AddLabel(ctx, threadID, claims.OrgID, "trash"); err != nil {
				return err
			}
			return tx.SetTrashExpiry(ctx, []string{threadID}, claims.OrgID)
		case "spam":
			return tx.AddLabel(ctx, threadID, claims.OrgID, "spam")
		case "archive":
			return tx.RemoveLabel(ctx, threadID, "inbox")
		default:
			return tx.AddLabel(ctx, threadID, claims.OrgID, req.Label)
		}
	}); err != nil {
		slog.Error("threads: move failed", "thread_id", threadID, "label", req.Label, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to move thread")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadMoved,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"to_label": req.Label,
			"thread":   h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "moved"})
}

func (h *ThreadHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	n, err := h.Store.UpdateThreadUnread(ctx, threadID, claims.OrgID, 0)
	if err != nil || n == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	// Mark all emails in thread as read
	if err := h.Store.MarkAllEmailsRead(ctx, threadID, claims.OrgID); err != nil {
		slog.Error("threads: mark emails as read failed", "thread_id", threadID, "error", err)
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadRead,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) MarkUnread(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	n, err := h.Store.UpdateThreadUnread(ctx, threadID, claims.OrgID, 1)
	if err != nil || n == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	// Mark latest email as unread
	if err := h.Store.MarkLatestEmailUnread(ctx, threadID, claims.OrgID); err != nil {
		slog.Error("threads: mark latest email as unread failed", "thread_id", threadID, "error", err)
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadUnread,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Star(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, err := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	// Accept optional { starred: bool } for idempotent set-value operations.
	// Falls back to toggle behavior if no body is provided.
	var req struct {
		Starred *bool `json:"starred"`
	}
	_ = readJSON(r, &req) // Ignore parse errors -- treat as toggle

	var wantStarred bool
	if req.Starred != nil {
		wantStarred = *req.Starred
	} else {
		wantStarred = !h.Store.HasLabel(ctx, threadID, "starred")
	}

	if wantStarred {
		h.Store.AddLabel(ctx, threadID, claims.OrgID, "starred")
	} else {
		h.Store.RemoveLabel(ctx, threadID, "starred")
	}

	evtType := event.ThreadStarred
	if !wantStarred {
		evtType = event.ThreadUnstarred
	}
	h.Bus.Publish(ctx, event.Event{
		EventType: evtType,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Mute(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, err := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	muted := h.Store.HasLabel(ctx, threadID, "muted")
	if muted {
		h.Store.RemoveLabel(ctx, threadID, "muted")
	} else {
		h.Store.AddLabel(ctx, threadID, claims.OrgID, "muted")
	}

	evtType := event.ThreadMuted
	if muted {
		evtType = event.ThreadUnmuted
	}
	h.Bus.Publish(ctx, event.Event{
		EventType: evtType,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Archive(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	if err := h.Store.RemoveLabel(ctx, threadID, "inbox"); err != nil {
		slog.Error("threads: archive removeLabel failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to archive thread")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadArchived,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Trash(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		if err := tx.AddLabel(ctx, threadID, claims.OrgID, "trash"); err != nil {
			return err
		}
		return tx.SetTrashExpiry(ctx, []string{threadID}, claims.OrgID)
	}); err != nil {
		slog.Error("threads: trash failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to trash thread")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadTrashed,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload: map[string]interface{}{
			"thread": h.fetchThreadSummary(ctx, threadID, claims.OrgID),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Spam(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	var req struct {
		Action string `json:"action"`
	}
	readJSON(r, &req)

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	evtType := event.ThreadSpammed
	payload := map[string]interface{}{}

	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		if req.Action == "not_spam" {
			if err := tx.RemoveLabel(ctx, threadID, "spam"); err != nil {
				return err
			}
			if err := tx.AddLabel(ctx, threadID, claims.OrgID, "inbox"); err != nil {
				return err
			}
			evtType = event.ThreadMoved
			payload["to_label"] = "inbox"
		} else {
			if err := tx.AddLabel(ctx, threadID, claims.OrgID, "spam"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.Error("threads: spam update failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update spam status")
		return
	}

	payload["thread"] = h.fetchThreadSummary(ctx, threadID, claims.OrgID)
	h.Bus.Publish(ctx, event.Event{
		EventType: evtType,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload:   payload,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *ThreadHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	ctx := r.Context()

	domainID, _ := h.Store.GetThreadDomainID(ctx, threadID, claims.OrgID)

	// Only allow deleting threads that have trash label
	if !h.Store.HasLabel(ctx, threadID, "trash") {
		writeError(w, http.StatusNotFound, "thread not found or not in trash")
		return
	}

	var deleted int64
	if err := h.Store.WithTx(ctx, func(tx store.Store) error {
		if err := tx.RemoveAllLabels(ctx, threadID); err != nil {
			return err
		}
		n, err := tx.SoftDeleteThread(ctx, threadID, claims.OrgID)
		if err != nil {
			return err
		}
		deleted = n
		return nil
	}); err != nil {
		slog.Error("threads: delete failed", "thread_id", threadID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	if deleted == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}

	h.Bus.Publish(ctx, event.Event{
		EventType: event.ThreadDeleted,
		OrgID:     claims.OrgID,
		UserID:    claims.UserID,
		DomainID:  domainID,
		ThreadID:  threadID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// fetchThreadSummary returns a thread map suitable for event payloads.
func (h *ThreadHandler) fetchThreadSummary(ctx context.Context, threadID, orgID string) map[string]interface{} {
	t, err := h.Store.FetchThreadSummary(ctx, threadID, orgID)
	if err != nil {
		return nil
	}
	return t
}

func (h *ThreadHandler) updateThread(w http.ResponseWriter, r *http.Request, query string) {
	claims := middleware.GetCurrentUser(r.Context())
	threadID := chi.URLParam(r, "id")
	tag, err := h.Store.Q().Exec(r.Context(), query, threadID, claims.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}
