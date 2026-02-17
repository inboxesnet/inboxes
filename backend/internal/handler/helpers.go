package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func readJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// resolveFromDisplay looks up a display name for an email address.
// Checks aliases table first, then users table, falls back to bare address.
// Returns "Display Name <address>" or just "address" if no name found.
func resolveFromDisplay(ctx context.Context, db *pgxpool.Pool, orgID, address string) string {
	var name string
	err := db.QueryRow(ctx,
		"SELECT name FROM aliases WHERE org_id = $1 AND address = $2 AND name != ''",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address)
	}

	err = db.QueryRow(ctx,
		"SELECT name FROM users WHERE org_id = $1 AND email = $2 AND name != '' AND status = 'active'",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address)
	}

	return address
}
