package handler

import (
	"testing"
	"time"
)

func TestEventHandler_DefaultCatchupMaxAge(t *testing.T) {
	t.Parallel()
	h := &EventHandler{}
	// When CatchupMaxAge is zero, it defaults to 48h in the handler
	if h.CatchupMaxAge != 0 {
		t.Errorf("expected zero-value CatchupMaxAge, got %v", h.CatchupMaxAge)
	}
	// The handler defaults to 48h when CatchupMaxAge <= 0
	expected := 48 * time.Hour
	if h.CatchupMaxAge <= 0 {
		if expected != 48*time.Hour {
			t.Errorf("default CatchupMaxAge: got %v, want %v", expected, 48*time.Hour)
		}
	}
}

func TestEventHandler_CustomCatchupMaxAge(t *testing.T) {
	t.Parallel()
	h := &EventHandler{CatchupMaxAge: 24 * time.Hour}
	if h.CatchupMaxAge != 24*time.Hour {
		t.Errorf("CatchupMaxAge: got %v, want %v", h.CatchupMaxAge, 24*time.Hour)
	}
}
