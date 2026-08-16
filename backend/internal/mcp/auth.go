package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenIdentity is the resolved identity behind a bearer token.
type TokenIdentity struct {
	TokenID string
	UserID  string
	OrgID   string
	Role    string
}

// HashToken returns the hex SHA-256 of a raw token. Tokens are high-entropy
// random strings, so a fast hash is sufficient — no KDF needed.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewToken generates a random token with the given prefix, e.g. "inbx_k".
func NewToken(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// LookupToken resolves a raw bearer token to a live identity. The role and
// status come from the users table on every call, so a role change or a
// disabled account takes effect immediately.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, raw string) (*TokenIdentity, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty token")
	}
	hash := HashToken(raw)

	var id TokenIdentity
	var expiresAt, revokedAt *time.Time
	var status string
	err := pool.QueryRow(ctx,
		`SELECT t.id, t.user_id, t.org_id, t.expires_at, t.revoked_at, u.role, u.status
		 FROM agent_tokens t
		 JOIN users u ON u.id = t.user_id
		 JOIN orgs o ON o.id = t.org_id AND o.deleted_at IS NULL
		 WHERE t.token_hash = $1`,
		hash,
	).Scan(&id.TokenID, &id.UserID, &id.OrgID, &expiresAt, &revokedAt, &id.Role, &status)
	if err != nil {
		return nil, fmt.Errorf("token not found")
	}
	if revokedAt != nil {
		return nil, fmt.Errorf("token revoked")
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token expired")
	}
	if status != "active" {
		return nil, fmt.Errorf("user not active")
	}

	// Best-effort usage stamp; failure never blocks the request.
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool.Exec(bctx, "UPDATE agent_tokens SET last_used_at = now() WHERE id = $1", id.TokenID)
	}()

	return &id, nil
}
