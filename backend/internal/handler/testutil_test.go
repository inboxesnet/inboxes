package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
)

// withClaims injects auth claims into a request's context for testing.
func withClaims(r *http.Request, userID, orgID, role string) *http.Request {
	claims := &middleware.Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, claims)
	return r.WithContext(ctx)
}

// newChiRouteContext returns a context with a chi URL param set.
func newChiRouteContext(key, value string) context.Context {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
}
