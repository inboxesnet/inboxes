package handler

import (
	"net/http"
	"strings"

	"github.com/inboxes/backend/internal/middleware"
	"github.com/inboxes/backend/internal/store"
)

type ContactHandler struct {
	Store store.Store
}

func (h *ContactHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	if query == "" {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	var aliasLabels []string
	if claims.Role != "admin" {
		aliasAddrs, _ := h.Store.GetUserAliasAddresses(r.Context(), claims.UserID)
		aliasLabels = make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			aliasLabels[i] = "alias:" + addr
		}
	}

	suggestions, err := h.Store.SuggestContacts(r.Context(), claims.OrgID, query, 10, claims.Role, aliasLabels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search contacts")
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

