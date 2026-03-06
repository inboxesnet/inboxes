package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_FirstCallImmediate(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(50 * time.Millisecond)
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("first call took %v, expected near-instant", elapsed)
	}
}

func TestRateLimiter_SecondCallWaits(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(50 * time.Millisecond)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 1: %v", err)
	}
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 2: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("second call took %v, expected >= 40ms", elapsed)
	}
}

func TestRateLimiter_AfterIntervalNoWait(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(10 * time.Millisecond)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 1: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 2: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("call after interval took %v, expected near-instant", elapsed)
	}
}

func TestRateLimiter_ContextCancelled(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(5 * time.Second)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 1: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := rl.Wait(ctx)
	if err != context.Canceled {
		t.Errorf("Wait: got %v, want context.Canceled", err)
	}
}

func TestRateLimiter_ConcurrentSafety(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(1 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rl.Wait(context.Background())
		}()
	}
	wg.Wait()
}

func TestRateLimiter_ZeroInterval(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(0)
	for i := 0; i < 5; i++ {
		start := time.Now()
		if err := rl.Wait(context.Background()); err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Millisecond {
			t.Errorf("call %d took %v, expected near-instant", i, elapsed)
		}
	}
}

func TestRateLimiter_ShortInterval(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(1 * time.Millisecond)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 1: %v", err)
	}
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 2: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 20*time.Millisecond {
		t.Errorf("short interval wait took %v, expected < 20ms", elapsed)
	}
}

func TestRateLimiter_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(5 * time.Second)
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait 1: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := rl.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Wait: got %v, want context.DeadlineExceeded", err)
	}
}
