package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/inboxes/backend/internal/service"
)

// --- calcBackoff ---

func TestCalcBackoff_Retry0(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(0); got != 5*time.Second {
		t.Errorf("calcBackoff(0) = %v, want 5s", got)
	}
}

func TestCalcBackoff_Retry1(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(1); got != 10*time.Second {
		t.Errorf("calcBackoff(1) = %v, want 10s", got)
	}
}

func TestCalcBackoff_Retry2(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(2); got != 20*time.Second {
		t.Errorf("calcBackoff(2) = %v, want 20s", got)
	}
}

func TestCalcBackoff_Retry4(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(4); got != 80*time.Second {
		t.Errorf("calcBackoff(4) = %v, want 80s", got)
	}
}

func TestCalcBackoff_Retry10_Capped(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(10); got != 300*time.Second {
		t.Errorf("calcBackoff(10) = %v, want 300s (capped)", got)
	}
}

func TestCalcBackoff_Retry20_StillCapped(t *testing.T) {
	t.Parallel()
	if got := calcBackoff(20); got != 300*time.Second {
		t.Errorf("calcBackoff(20) = %v, want 300s (capped)", got)
	}
}

// --- shouldSkipJob ---

func TestShouldSkipJob_Completed(t *testing.T) {
	t.Parallel()
	if !shouldSkipJob("completed", 0, 3) {
		t.Error("expected true for completed status")
	}
}

func TestShouldSkipJob_FailedMaxRetries(t *testing.T) {
	t.Parallel()
	if !shouldSkipJob("failed", 3, 3) {
		t.Error("expected true for failed + retryCount >= maxRetries")
	}
}

func TestShouldSkipJob_FailedUnderMaxRetries(t *testing.T) {
	t.Parallel()
	if shouldSkipJob("failed", 1, 3) {
		t.Error("expected false for failed + retryCount < maxRetries")
	}
}

func TestShouldSkipJob_Pending(t *testing.T) {
	t.Parallel()
	if shouldSkipJob("pending", 0, 3) {
		t.Error("expected false for pending status")
	}
}

func TestShouldSkipJob_Running(t *testing.T) {
	t.Parallel()
	if shouldSkipJob("running", 0, 3) {
		t.Error("expected false for running status")
	}
}

// --- isRetryableFailure ---

func TestIsRetryableFailure_ResendError422(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 422, Body: "validation error"}
	if isRetryableFailure(err) {
		t.Error("expected 422 to be non-retryable")
	}
}

func TestIsRetryableFailure_ResendError429(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 429, Body: "rate limited"}
	if !isRetryableFailure(err) {
		t.Error("expected 429 to be retryable")
	}
}

func TestIsRetryableFailure_ResendError500(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 500, Body: "server error"}
	if !isRetryableFailure(err) {
		t.Error("expected 500 to be retryable")
	}
}

func TestIsRetryableFailure_GenericError(t *testing.T) {
	t.Parallel()
	if !isRetryableFailure(errors.New("something broke")) {
		t.Error("expected generic errors to be retryable")
	}
}

func TestIsRetryableFailure_NilError(t *testing.T) {
	t.Parallel()
	if !isRetryableFailure(nil) {
		t.Error("expected nil to be retryable")
	}
}

// --- shouldPermanentlyFail ---

func TestShouldPermanentlyFail_NotRetryable(t *testing.T) {
	t.Parallel()
	if !shouldPermanentlyFail(false, 1, 5) {
		t.Error("expected true when not retryable")
	}
}

func TestShouldPermanentlyFail_RetryableUnderMax(t *testing.T) {
	t.Parallel()
	if shouldPermanentlyFail(true, 2, 5) {
		t.Error("expected false when retryable and under max")
	}
}

func TestShouldPermanentlyFail_RetryableAtMax(t *testing.T) {
	t.Parallel()
	if !shouldPermanentlyFail(true, 5, 5) {
		t.Error("expected true when retryable but at max")
	}
}

func TestShouldPermanentlyFail_RetryableOverMax(t *testing.T) {
	t.Parallel()
	if !shouldPermanentlyFail(true, 6, 5) {
		t.Error("expected true when retryable but over max")
	}
}

// --- isDomainFailure ---

func TestIsDomainFailure_NilError(t *testing.T) {
	t.Parallel()
	if isDomainFailure(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsDomainFailure_ResendError403(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 403, Body: "invalid api key"}
	if !isDomainFailure(err) {
		t.Error("expected true for 403")
	}
}

func TestIsDomainFailure_ResendError422_DomainBody(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 422, Body: `{"code":"invalid_from_address"}`}
	if !isDomainFailure(err) {
		t.Error("expected true for 422 with invalid_from_address")
	}
}

func TestIsDomainFailure_ResendError422_OtherBody(t *testing.T) {
	t.Parallel()
	err := &service.ResendError{StatusCode: 422, Body: `{"code":"validation_error"}`}
	if isDomainFailure(err) {
		t.Error("expected false for 422 with generic validation")
	}
}

func TestIsDomainFailure_GenericError(t *testing.T) {
	t.Parallel()
	if isDomainFailure(errors.New("network timeout")) {
		t.Error("expected false for generic error")
	}
}

func TestIsDomainFailure_WrappedResendError(t *testing.T) {
	t.Parallel()
	inner := &service.ResendError{StatusCode: 403, Body: "forbidden"}
	wrapped := fmt.Errorf("send failed: %w", inner)
	if !isDomainFailure(wrapped) {
		t.Error("expected true for wrapped ResendError 403")
	}
}
