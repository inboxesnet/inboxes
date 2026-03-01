package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenBlacklist provides JWT revocation via Redis.
// Individual tokens can be revoked by JTI, or all tokens for a user
// can be revoked at once (e.g. on password change or account disable).
type TokenBlacklist struct {
	rdb *redis.Client
}

func NewTokenBlacklist(rdb *redis.Client) *TokenBlacklist {
	return &TokenBlacklist{rdb: rdb}
}

// RevokeToken blacklists a single token by its JTI until the token's expiry.
// Returns an error if Redis is unavailable.
func (b *TokenBlacklist) RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error {
	if b == nil || b.rdb == nil {
		return fmt.Errorf("redis client not available")
	}
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return nil // already expired
	}
	if err := b.rdb.Set(ctx, "token:bl:"+jti, "1", remaining).Err(); err != nil {
		slog.Error("token_blacklist: failed to revoke token", "jti", jti, "error", err)
		return err
	}
	return nil
}

// RevokeAllForUser invalidates all tokens issued before now for the given user.
// Tokens are checked against this timestamp; any token with IssuedAt before
// the stored value is considered revoked. TTL is 7 days (max token lifetime).
// Returns an error if Redis is unavailable.
func (b *TokenBlacklist) RevokeAllForUser(ctx context.Context, userID string) error {
	if b == nil || b.rdb == nil {
		return fmt.Errorf("redis client not available")
	}
	if err := b.rdb.Set(ctx, "token:rv:"+userID, time.Now().Unix(), 7*24*time.Hour).Err(); err != nil {
		slog.Error("token_blacklist: failed to revoke all for user", "user_id", userID, "error", err)
		return err
	}
	// Push-based WS disconnect — immediately close all WebSocket connections for this user
	if err := b.rdb.Publish(ctx, "ws:disconnect", userID).Err(); err != nil {
		slog.Warn("token_blacklist: failed to publish ws:disconnect", "user_id", userID, "error", err)
	}
	return nil
}

// IsRevoked checks whether a token has been revoked, either individually or
// via a user-wide revocation. Fails open: if Redis is unavailable, returns false.
func (b *TokenBlacklist) IsRevoked(ctx context.Context, jti, userID string, issuedAt time.Time) bool {
	if b == nil || b.rdb == nil {
		return false
	}
	// Check individual token revocation
	if jti != "" {
		exists, err := b.rdb.Exists(ctx, "token:bl:"+jti).Result()
		if err != nil {
			slog.Error("token_blacklist: redis error on jti check, failing open", "error", err)
			return false
		}
		if exists > 0 {
			return true
		}
	}

	// Check user-wide revocation
	val, err := b.rdb.Get(ctx, "token:rv:"+userID).Int64()
	if err != nil {
		// Key doesn't exist or Redis error — not revoked
		return false
	}
	revokedAt := time.Unix(val, 0)
	return issuedAt.Before(revokedAt)
}

const maxSessions = 5

// Session represents an active user session tracked in Redis.
type Session struct {
	JTI       string    `json:"jti"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterSession adds a JTI to the user's session set and evicts oldest sessions
// beyond the max limit. Fails silently if Redis is unavailable.
func (b *TokenBlacklist) RegisterSession(ctx context.Context, userID, jti string) {
	if b == nil || b.rdb == nil {
		return
	}
	key := "sessions:" + userID
	b.rdb.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().Unix()), Member: jti})
	b.rdb.Expire(ctx, key, 7*24*time.Hour)

	count, _ := b.rdb.ZCard(ctx, key).Result()
	if count > maxSessions {
		oldest, _ := b.rdb.ZRange(ctx, key, 0, count-maxSessions-1).Result()
		for _, oldJTI := range oldest {
			b.rdb.ZRem(ctx, key, oldJTI)
			b.rdb.Set(ctx, "token:bl:"+oldJTI, "1", 7*24*time.Hour)
		}
	}
}

// ListSessions returns all active sessions for a user.
func (b *TokenBlacklist) ListSessions(ctx context.Context, userID string) []Session {
	if b == nil || b.rdb == nil {
		return nil
	}
	members, _ := b.rdb.ZRangeWithScores(ctx, "sessions:"+userID, 0, -1).Result()
	sessions := make([]Session, 0, len(members))
	for _, m := range members {
		sessions = append(sessions, Session{
			JTI:       m.Member.(string),
			CreatedAt: time.Unix(int64(m.Score), 0),
		})
	}
	return sessions
}

// RevokeSession removes a specific session and blacklists the token.
func (b *TokenBlacklist) RevokeSession(ctx context.Context, userID, jti string) error {
	if b == nil || b.rdb == nil {
		return fmt.Errorf("redis client not available")
	}
	b.rdb.ZRem(ctx, "sessions:"+userID, jti)
	return b.rdb.Set(ctx, "token:bl:"+jti, "1", 7*24*time.Hour).Err()
}

// ClearSessions removes all tracked sessions for a user.
func (b *TokenBlacklist) ClearSessions(ctx context.Context, userID string) {
	if b == nil || b.rdb == nil {
		return
	}
	b.rdb.Del(ctx, "sessions:"+userID)
}
