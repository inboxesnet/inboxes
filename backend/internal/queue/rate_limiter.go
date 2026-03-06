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
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	elapsed := time.Since(rl.lastCall)
	wait := rl.interval - elapsed
	if wait <= 0 {
		rl.lastCall = time.Now()
		rl.mu.Unlock()
		return nil
	}
	rl.mu.Unlock()

	select {
	case <-time.After(wait):
	case <-ctx.Done():
		return ctx.Err()
	}

	rl.mu.Lock()
	rl.lastCall = time.Now()
	rl.mu.Unlock()
	return nil
}
