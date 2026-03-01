package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/service"
)

func TestIsResendErr_ResendError(t *testing.T) {
	t.Parallel()
	resendErr := &service.ResendError{StatusCode: 429, Body: "rate limited"}
	var target *service.ResendError
	if !isResendErr(resendErr, &target) {
		t.Error("expected true for *service.ResendError")
	}
	if target.StatusCode != 429 {
		t.Errorf("StatusCode: got %d, want 429", target.StatusCode)
	}
}

func TestIsResendErr_WrappedResendError(t *testing.T) {
	t.Parallel()
	resendErr := &service.ResendError{StatusCode: 500, Body: "internal"}
	wrapped := errors.Join(errors.New("context"), resendErr)
	var target *service.ResendError
	if !isResendErr(wrapped, &target) {
		t.Error("expected true for wrapped *service.ResendError")
	}
}

func TestIsResendErr_NonResendError(t *testing.T) {
	t.Parallel()
	generic := errors.New("something went wrong")
	var target *service.ResendError
	if isResendErr(generic, &target) {
		t.Error("expected false for generic error")
	}
}

func TestIsResendErr_NilError(t *testing.T) {
	t.Parallel()
	var target *service.ResendError
	if isResendErr(nil, &target) {
		t.Error("expected false for nil error")
	}
}

func TestNewDomainHeartbeat_DefaultInterval(t *testing.T) {
	t.Parallel()
	dh := NewDomainHeartbeat(nil, nil, nil, 0)
	if dh.Interval != 6*time.Hour {
		t.Errorf("Interval: got %v, want %v", dh.Interval, 6*time.Hour)
	}
}

func TestNewDomainHeartbeat_CustomInterval(t *testing.T) {
	t.Parallel()
	dh := NewDomainHeartbeat(nil, nil, nil, 2*time.Hour)
	if dh.Interval != 2*time.Hour {
		t.Errorf("Interval: got %v, want %v", dh.Interval, 2*time.Hour)
	}
}
