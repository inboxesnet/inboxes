package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// In-memory fallback caches used when Redis is unavailable.
// These are package-level so all TokenBlacklist instances share them.
var (
	revokedTokens sync.Map // JTI → true
	revokedUsers  sync.Map // userID → int64 (unix timestamp)
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

	// Always write to in-memory cache as fallback
	revokedTokens.Store(jti, true)
	go func() {
		time.Sleep(remaining)
		revokedTokens.Delete(jti)
	}()

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

	now := time.Now().Unix()

	// Always write to in-memory cache as fallback
	revokedUsers.Store(userID, now)
	go func() {
		time.Sleep(7 * 24 * time.Hour)
		// Only clean up if the value hasn't been updated since
		if val, ok := revokedUsers.Load(userID); ok && val.(int64) == now {
			revokedUsers.Delete(userID)
		}
	}()

	if err := b.rdb.Set(ctx, "token:rv:"+userID, now, 7*24*time.Hour).Err(); err != nil {
		slog.Error("token_blacklist: failed to revoke all for user", "user_id", userID, "error", err)
		return err
	}
	// Push-based WS disconnect — immediately close all WebSocket connections for this user
	if err := b.rdb.Publish(ctx, "ws:disconnect", userID).Err(); err != nil {
		slog.Warn("token_blacklist: failed to publish ws:disconnect", "user_id", userID, "error", err)
	}
	return nil
}

// checkMemoryRevocation checks the in-memory fallback caches for revocation status.
func checkMemoryRevocation(jti, userID string, issuedAt time.Time) bool {
	// Check individual token revocation
	if jti != "" {
		if _, ok := revokedTokens.Load(jti); ok {
			return true
		}
	}
	// Check user-wide revocation
	if val, ok := revokedUsers.Load(userID); ok {
		revokedAt := time.Unix(val.(int64), 0)
		return issuedAt.Before(revokedAt)
	}
	return false
}

// IsRevoked checks whether a token has been revoked, either individually or
// via a user-wide revocation. Falls back to in-memory caches if Redis is unavailable.
func (b *TokenBlacklist) IsRevoked(ctx context.Context, jti, userID string, issuedAt time.Time) bool {
	return b.Check(ctx, TokenCheck{JTI: jti, UserID: userID, IssuedAt: issuedAt}).Revoked
}

// retireGrace is the time a superseded token stays valid after the client
// first uses its replacement. Requests already in flight carry the old token.
const retireGrace = 60 * time.Second

// renewLockTTL limits renewal to one new token per old token in this window.
const renewLockTTL = 60 * time.Second

// sessionTTL is the maximum token lifetime.
const sessionTTL = 7 * 24 * time.Hour

// TokenCheck is the input of Check.
type TokenCheck struct {
	JTI      string
	UserID   string
	IssuedAt time.Time
	// RetireJTI is a superseded token. Check blacklists it after retireGrace.
	RetireJTI string
	// StatusKey is an optional extra key to read in the same round trip.
	StatusKey string
}

// TokenCheckResult is the output of Check.
type TokenCheckResult struct {
	Revoked      bool
	Status       string
	StatusCached bool
}

// blacklistActive reports whether a token:bl value revokes the token now.
// "1" revokes at once. Any other value is the Unix time at which the
// revocation starts.
func blacklistActive(val string, now time.Time) bool {
	if val == "1" {
		return true
	}
	at, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return true
	}
	return now.Unix() >= at
}

// Check does all per-request token reads in one Redis round trip. It falls
// back to the in-memory caches if Redis is unavailable.
func (b *TokenBlacklist) Check(ctx context.Context, c TokenCheck) TokenCheckResult {
	if b == nil || b.rdb == nil {
		return TokenCheckResult{Revoked: checkMemoryRevocation(c.JTI, c.UserID, c.IssuedAt)}
	}
	now := time.Now()

	pipe := b.rdb.Pipeline()
	var blCmd, stCmd *redis.StringCmd
	if c.JTI != "" {
		blCmd = pipe.Get(ctx, "token:bl:"+c.JTI)
	}
	rvCmd := pipe.Get(ctx, "token:rv:"+c.UserID)
	if c.StatusKey != "" {
		stCmd = pipe.Get(ctx, c.StatusKey)
	}
	if c.RetireJTI != "" {
		// NX keeps an earlier deadline and an immediate revocation.
		pipe.SetNX(ctx, "token:bl:"+c.RetireJTI, now.Add(retireGrace).Unix(), sessionTTL)
	}
	// Exec returns the first command error, which includes redis.Nil for a
	// missing key. The code below reads each command result instead.
	_, _ = pipe.Exec(ctx)

	var res TokenCheckResult
	if stCmd != nil {
		if val, err := stCmd.Result(); err == nil {
			res.Status = val
			res.StatusCached = true
		}
	}

	if blCmd != nil {
		val, err := blCmd.Result()
		if err != nil && err != redis.Nil {
			slog.Error("token_blacklist: redis error on jti check, falling back to memory", "error", err)
			res.Revoked = checkMemoryRevocation(c.JTI, c.UserID, c.IssuedAt)
			return res
		}
		if err == nil && blacklistActive(val, now) {
			res.Revoked = true
			return res
		}
	}

	val, err := rvCmd.Int64()
	if err != nil {
		if err == redis.Nil {
			return res
		}
		slog.Error("token_blacklist: redis error on user revocation check, falling back to memory", "error", err)
		res.Revoked = checkMemoryRevocation(c.JTI, c.UserID, c.IssuedAt)
		return res
	}
	res.Revoked = c.IssuedAt.Before(time.Unix(val, 0))
	return res
}

const maxSessions = 5

// Session represents an active user session tracked in Redis.
type Session struct {
	JTI       string    `json:"jti"`
	CreatedAt time.Time `json:"created_at"`
}

// sessionScript registers a session and enforces the session limit in one
// atomic step. With an old JTI it replaces that session, so one device keeps
// one slot. It blacklists each evicted session.
// KEYS[1] = session set. ARGV = new JTI, score, limit, TTL seconds, old JTI.
var sessionScript = redis.NewScript(`
if ARGV[5] ~= '' then
  redis.call('ZREM', KEYS[1], ARGV[5])
end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[4])
local extra = redis.call('ZCARD', KEYS[1]) - tonumber(ARGV[3])
if extra > 0 then
  local old = redis.call('ZRANGE', KEYS[1], 0, extra - 1)
  for _, jti in ipairs(old) do
    redis.call('ZREM', KEYS[1], jti)
    redis.call('SET', 'token:bl:' .. jti, '1', 'EX', ARGV[4])
  end
end
return 0
`)

func (b *TokenBlacklist) runSessionScript(ctx context.Context, userID, oldJTI, newJTI string) {
	if b == nil || b.rdb == nil {
		return
	}
	// A millisecond score keeps the newest session last when two sessions
	// start in the same second.
	err := sessionScript.Run(ctx, b.rdb, []string{"sessions:" + userID},
		newJTI, time.Now().UnixMilli(), maxSessions, int(sessionTTL.Seconds()), oldJTI,
	).Err()
	if err != nil {
		slog.Warn("token_blacklist: failed to register session", "user_id", userID, "error", err)
	}
}

// RegisterSession adds a JTI to the user's session set and evicts oldest sessions
// beyond the max limit. Fails silently if Redis is unavailable.
func (b *TokenBlacklist) RegisterSession(ctx context.Context, userID, jti string) {
	b.runSessionScript(ctx, userID, "", jti)
}

// ReplaceSession puts newJTI in the session slot of oldJTI after a renewal.
// It does not blacklist oldJTI. Check does that when the client first uses
// the new token.
func (b *TokenBlacklist) ReplaceSession(ctx context.Context, userID, oldJTI, newJTI string) {
	b.runSessionScript(ctx, userID, oldJTI, newJTI)
}

// AcquireRenewal reports whether the caller may renew the token with this
// JTI. Only one caller wins in each renewLockTTL window, so parallel requests
// with the same old token mint one new token. It returns false on a Redis
// error: the old token stays valid and a later request renews it.
func (b *TokenBlacklist) AcquireRenewal(ctx context.Context, jti string) bool {
	if b == nil || b.rdb == nil {
		return true
	}
	ok, err := b.rdb.SetNX(ctx, "token:renew:"+jti, "1", renewLockTTL).Result()
	if err != nil {
		slog.Warn("token_blacklist: renewal lock failed, skipping renewal", "error", err)
		return false
	}
	return ok
}

// RemoveSession removes one session from the user's session set. It does
// not blacklist the token.
func (b *TokenBlacklist) RemoveSession(ctx context.Context, userID, jti string) {
	if b == nil || b.rdb == nil {
		return
	}
	b.rdb.ZRem(ctx, "sessions:"+userID, jti)
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
			CreatedAt: sessionTime(m.Score),
		})
	}
	return sessions
}

// sessionTime converts a session score to a time. Scores written before the
// change to milliseconds are in seconds.
func sessionTime(score float64) time.Time {
	if score < 1e11 {
		return time.Unix(int64(score), 0)
	}
	return time.UnixMilli(int64(score))
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
