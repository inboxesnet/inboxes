package service

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRevokeToken_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	err := b.RevokeToken(context.Background(), "jti", time.Now().Add(time.Hour))
	if err == nil {
		t.Error("expected error for nil receiver")
	}
}

func TestRevokeToken_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	err := b.RevokeToken(context.Background(), "jti", time.Now().Add(time.Hour))
	if err == nil {
		t.Error("expected error for nil redis")
	}
}

func TestRevokeAllForUser_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	err := b.RevokeAllForUser(context.Background(), "user1")
	if err == nil {
		t.Error("expected error for nil receiver")
	}
}

func TestRevokeAllForUser_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	err := b.RevokeAllForUser(context.Background(), "user1")
	if err == nil {
		t.Error("expected error for nil redis")
	}
}

func TestIsRevoked_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	result := b.IsRevoked(context.Background(), "jti", "user1", time.Now())
	if result {
		t.Error("expected false (fail-open) for nil receiver")
	}
}

func TestIsRevoked_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	result := b.IsRevoked(context.Background(), "jti", "user1", time.Now())
	if result {
		t.Error("expected false (fail-open) for nil redis")
	}
}

func TestRegisterSession_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	// Should not panic
	b.RegisterSession(context.Background(), "user1", "jti")
}

func TestRegisterSession_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	// Should not panic
	b.RegisterSession(context.Background(), "user1", "jti")
}

func TestListSessions_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	sessions := b.ListSessions(context.Background(), "user1")
	if sessions != nil {
		t.Errorf("expected nil for nil receiver, got %v", sessions)
	}
}

func TestListSessions_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	sessions := b.ListSessions(context.Background(), "user1")
	if sessions != nil {
		t.Errorf("expected nil for nil redis, got %v", sessions)
	}
}

func TestRevokeSession_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	err := b.RevokeSession(context.Background(), "user1", "jti")
	if err == nil {
		t.Error("expected error for nil receiver")
	}
}

func TestRevokeSession_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	err := b.RevokeSession(context.Background(), "user1", "jti")
	if err == nil {
		t.Error("expected error for nil redis")
	}
}

func TestClearSessions_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	// Should not panic
	b.ClearSessions(context.Background(), "user1")
}

func TestClearSessions_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	// Should not panic
	b.ClearSessions(context.Background(), "user1")
}

func TestMaxSessionsConstant(t *testing.T) {
	t.Parallel()
	if maxSessions != 5 {
		t.Errorf("maxSessions: got %d, want 5", maxSessions)
	}
}

func TestKeyFormat_RevokeToken(t *testing.T) {
	t.Parallel()
	// Verify the key prefix used in RevokeToken
	// We can't test actual Redis ops without integration, but validate the
	// nil-receiver error message format
	var b *TokenBlacklist
	err := b.RevokeToken(context.Background(), "test-jti", time.Now().Add(time.Hour))
	if err == nil || err.Error() != "redis client not available" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKeyFormat_RevokeAllForUser(t *testing.T) {
	t.Parallel()
	var b *TokenBlacklist
	err := b.RevokeAllForUser(context.Background(), "test-user")
	if err == nil || err.Error() != "redis client not available" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlacklistActive(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{strconv.FormatInt(now.Add(time.Minute).Unix(), 10), false},
		{strconv.FormatInt(now.Add(-time.Minute).Unix(), 10), true},
		{"garbage", true},
	}
	for _, c := range cases {
		if got := blacklistActive(c.val, now); got != c.want {
			t.Errorf("blacklistActive(%q): got %v, want %v", c.val, got, c.want)
		}
	}
}

func TestAcquireRenewal_NilRedis(t *testing.T) {
	t.Parallel()
	b := &TokenBlacklist{rdb: nil}
	if !b.AcquireRenewal(context.Background(), "jti") {
		t.Error("without Redis the renewal must proceed")
	}
}

func TestSessionTime(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	if got := sessionTime(float64(now.Unix())); !got.Equal(now) {
		t.Errorf("seconds score: got %v, want %v", got, now)
	}
	if got := sessionTime(float64(now.UnixMilli())); !got.Equal(now) {
		t.Errorf("milliseconds score: got %v, want %v", got, now)
	}
}

// redisForTest returns a client for TEST_REDIS_URL, or skips the test.
func redisForTest(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func TestRegisterSession_EvictsOldestBeyondLimit(t *testing.T) {
	rdb := redisForTest(t)
	ctx := context.Background()
	b := NewTokenBlacklist(rdb)
	user := "test-user-" + uuid.NewString()
	t.Cleanup(func() { rdb.Del(ctx, "sessions:"+user) })

	var jtis []string
	for i := 0; i < maxSessions+2; i++ {
		jti := uuid.NewString()
		jtis = append(jtis, jti)
		t.Cleanup(func() { rdb.Del(ctx, "token:bl:"+jti) })
		b.RegisterSession(ctx, user, jti)
		time.Sleep(2 * time.Millisecond)
	}
	if got := len(b.ListSessions(ctx, user)); got != maxSessions {
		t.Fatalf("session count: got %d, want %d", got, maxSessions)
	}
	for i, jti := range jtis {
		revoked := b.IsRevoked(ctx, jti, user, time.Now())
		if want := i < 2; revoked != want {
			t.Errorf("session %d: revoked = %v, want %v", i, revoked, want)
		}
	}
}

func TestReplaceSession_KeepsOneSlot(t *testing.T) {
	rdb := redisForTest(t)
	ctx := context.Background()
	b := NewTokenBlacklist(rdb)
	user := "test-user-" + uuid.NewString()
	t.Cleanup(func() { rdb.Del(ctx, "sessions:"+user) })

	other := uuid.NewString()
	b.RegisterSession(ctx, user, other)
	cur := uuid.NewString()
	b.RegisterSession(ctx, user, cur)
	// Many renewals of one device must not evict the other device.
	for i := 0; i < maxSessions*2; i++ {
		next := uuid.NewString()
		b.ReplaceSession(ctx, user, cur, next)
		if b.IsRevoked(ctx, cur, user, time.Now()) {
			t.Fatal("ReplaceSession must not blacklist the old token")
		}
		cur = next
	}
	sessions := b.ListSessions(ctx, user)
	if len(sessions) != 2 {
		t.Fatalf("session count: got %d, want 2", len(sessions))
	}
	if b.IsRevoked(ctx, other, user, time.Now()) {
		t.Error("the other device lost its session")
	}
}

func TestAcquireRenewal_OneWinner(t *testing.T) {
	rdb := redisForTest(t)
	ctx := context.Background()
	b := NewTokenBlacklist(rdb)
	jti := uuid.NewString()
	t.Cleanup(func() { rdb.Del(ctx, "token:renew:"+jti) })

	if !b.AcquireRenewal(ctx, jti) {
		t.Fatal("first caller must win")
	}
	if b.AcquireRenewal(ctx, jti) {
		t.Error("second caller must not win")
	}
}

func TestCheck_RetireHasGraceAndStatus(t *testing.T) {
	rdb := redisForTest(t)
	ctx := context.Background()
	b := NewTokenBlacklist(rdb)
	user := "test-user-" + uuid.NewString()
	oldJTI, newJTI := uuid.NewString(), uuid.NewString()
	statusKey := "user:status:" + user
	t.Cleanup(func() { rdb.Del(ctx, "token:bl:"+oldJTI, statusKey) })

	res := b.Check(ctx, TokenCheck{JTI: newJTI, UserID: user, IssuedAt: time.Now(), RetireJTI: oldJTI, StatusKey: statusKey})
	if res.Revoked || res.StatusCached {
		t.Fatalf("first check: got %+v", res)
	}
	// Inside the grace window the old token still works.
	if b.IsRevoked(ctx, oldJTI, user, time.Now()) {
		t.Error("old token revoked inside the grace window")
	}
	// After the deadline it does not.
	rdb.Set(ctx, "token:bl:"+oldJTI, time.Now().Add(-time.Second).Unix(), time.Minute)
	if !b.IsRevoked(ctx, oldJTI, user, time.Now()) {
		t.Error("old token not revoked after the grace window")
	}
	// A second retire must not move the deadline.
	b.Check(ctx, TokenCheck{JTI: newJTI, UserID: user, IssuedAt: time.Now(), RetireJTI: oldJTI})
	if !b.IsRevoked(ctx, oldJTI, user, time.Now()) {
		t.Error("a second retire extended the grace window")
	}

	rdb.Set(ctx, statusKey, "active", time.Minute)
	res = b.Check(ctx, TokenCheck{JTI: newJTI, UserID: user, IssuedAt: time.Now(), StatusKey: statusKey})
	if !res.StatusCached || res.Status != "active" {
		t.Errorf("status read: got %+v", res)
	}
}
