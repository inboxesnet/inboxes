package worker

import (
	"testing"
	"time"
)

func TestNewGracePeriodWorker_DefaultInterval(t *testing.T) {
	t.Parallel()
	w := NewGracePeriodWorker(nil, nil, 0)
	if w.Interval != 1*time.Hour {
		t.Errorf("Interval: got %v, want %v", w.Interval, 1*time.Hour)
	}
}

func TestNewGracePeriodWorker_CustomInterval(t *testing.T) {
	t.Parallel()
	w := NewGracePeriodWorker(nil, nil, 30*time.Minute)
	if w.Interval != 30*time.Minute {
		t.Errorf("Interval: got %v, want %v", w.Interval, 30*time.Minute)
	}
}

func TestNewGracePeriodWorker_ZeroInterval(t *testing.T) {
	t.Parallel()
	w := NewGracePeriodWorker(nil, nil, 0)
	if w.Interval != 1*time.Hour {
		t.Errorf("Interval: got %v, want %v (zero should default to 1h)", w.Interval, 1*time.Hour)
	}
}

func TestNewGracePeriodWorker_NegativeInterval(t *testing.T) {
	t.Parallel()
	w := NewGracePeriodWorker(nil, nil, -5*time.Minute)
	if w.Interval != 1*time.Hour {
		t.Errorf("Interval: got %v, want %v (negative should default to 1h)", w.Interval, 1*time.Hour)
	}
}
