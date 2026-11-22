package queue

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces a minimum interval between Resend API calls.
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	lastCall time.Time
}

// NewRateLimiter creates a rate limiter with the given minimum interval between calls.
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{interval: interval}
}

// Wait blocks until the minimum interval has elapsed since the last call.
// Returns an error if the context is cancelled while waiting.
// Each caller reserves its own slot under the lock, so concurrent callers
// leave one after the other and not all at the same time.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	now := time.Now()
	slot := rl.lastCall.Add(rl.interval)
	if slot.Before(now) {
		slot = now
	}
	rl.lastCall = slot
	rl.mu.Unlock()

	wait := slot.Sub(now)
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
