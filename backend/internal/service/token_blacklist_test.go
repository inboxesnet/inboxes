package service

import (
	"context"
	"testing"
	"time"
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
