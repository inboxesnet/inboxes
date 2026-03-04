//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/inboxes/backend/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

// seedOrg creates a test org + admin user, returns orgID, userID.
func seedOrg(t *testing.T, name, email, password string) (orgID, userID string) {
	t.Helper()
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	orgID, userID, err = testStore.CreateOrgAndAdmin(ctx, name, email, "Test Admin", string(hash), true, false)
	if err != nil {
		t.Fatal(err)
	}
	return orgID, userID
}

// seedDomain creates a test domain, returns domainID.
func seedDomain(t *testing.T, orgID, domainName string) string {
	t.Helper()
	ctx := context.Background()
	domainID, err := testStore.InsertDomain(ctx, orgID, domainName, "resend-"+domainName, "active", json.RawMessage("[]"))
	if err != nil {
		t.Fatal(err)
	}
	return domainID
}

// seedAlias creates an alias and returns its ID.
func seedAlias(t *testing.T, orgID, domainID, address, name string) string {
	t.Helper()
	ctx := context.Background()
	aliasID, err := testStore.CreateAlias(ctx, orgID, domainID, address, name)
	if err != nil {
		t.Fatal(err)
	}
	return aliasID
}

// seedThread creates a thread and returns its ID.
func seedThread(t *testing.T, orgID, userID, domainID, subject string) string {
	t.Helper()
	ctx := context.Background()
	participants, _ := json.Marshal([]string{"test@example.com"})
	threadID, err := testStore.CreateThread(ctx, orgID, userID, domainID, subject, participants, "Test snippet")
	if err != nil {
		t.Fatal(err)
	}
	// Add "inbox" label so it appears in thread listings
	testStore.AddLabel(ctx, threadID, orgID, "inbox")
	return threadID
}

// seedEmail inserts a test email into a thread.
func seedEmail(t *testing.T, orgID, userID, domainID, threadID, direction, from, subject string) string {
	t.Helper()
	ctx := context.Background()
	toJSON, _ := json.Marshal([]string{"to@example.com"})
	emptyJSON := json.RawMessage("[]")
	emailID, err := testStore.InsertEmail(ctx, threadID, userID, orgID, domainID, direction, from, toJSON, emptyJSON, emptyJSON, subject, "<p>body</p>", "body", "delivered", "", emptyJSON)
	if err != nil {
		t.Fatal(err)
	}
	return emailID
}

// cleanupOrg deletes all data for an org (for test isolation).
func cleanupOrg(t *testing.T, orgID string) {
	t.Helper()
	ctx := context.Background()
	// Order matters due to FK constraints.
	// First delete tables that don't have org_id but reference org-owned rows.
	testPool.Exec(ctx, "DELETE FROM discovered_addresses WHERE domain_id IN (SELECT id FROM domains WHERE org_id = $1)", orgID)

	// Now delete tables that have org_id directly.
	tablesWithOrgID := []string{
		"events",
		"thread_labels",
		"email_bounces",
		"email_jobs",
		"sync_jobs",
		"drafts",
		"attachments",
		"alias_users",
		"emails",
		"threads",
		"aliases",
		"org_labels",
		"domains",
		"user_reassignments",
		"users",
	}
	for _, tbl := range tablesWithOrgID {
		testPool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE org_id = $1", tbl), orgID)
	}
	testPool.Exec(ctx, "DELETE FROM orgs WHERE id = $1", orgID)
}

// jsonBody creates a JSON string reader for HTTP requests.
func jsonBody(v any) *strings.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

// parseJSON decodes the response body into dst.
func parseJSON(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(dst); err != nil {
		t.Fatalf("parse response JSON: %v\nbody: %s", err, w.Body.String())
	}
}

// withChiParam adds a chi URL param to the request context.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// withClaims sets auth claims on the request context using the same key
// and type as the real middleware (middleware.UserContextKey / *middleware.Claims).
func withClaims(r *http.Request, userID, orgID, role string) *http.Request {
	claims := &middleware.Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, claims)
	return r.WithContext(ctx)
}
