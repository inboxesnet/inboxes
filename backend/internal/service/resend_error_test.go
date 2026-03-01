package service

import (
	"strings"
	"testing"
)

// --- IsRetryable ---

func TestResendError_IsRetryable_429(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 429, Body: "rate limited"}
	if !e.IsRetryable() {
		t.Error("429 should be retryable")
	}
}

func TestResendError_IsRetryable_409(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 409, Body: "conflict"}
	if !e.IsRetryable() {
		t.Error("409 should be retryable")
	}
}

func TestResendError_IsRetryable_500(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 500, Body: "server error"}
	if !e.IsRetryable() {
		t.Error("500 should be retryable")
	}
}

func TestResendError_IsRetryable_502(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 502, Body: "bad gateway"}
	if !e.IsRetryable() {
		t.Error("502 should be retryable")
	}
}

func TestResendError_IsRetryable_503(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 503, Body: "unavailable"}
	if !e.IsRetryable() {
		t.Error("503 should be retryable")
	}
}

func TestResendError_IsRetryable_400_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 400, Body: "bad request"}
	if e.IsRetryable() {
		t.Error("400 should NOT be retryable")
	}
}

func TestResendError_IsRetryable_401_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 401, Body: "unauthorized"}
	if e.IsRetryable() {
		t.Error("401 should NOT be retryable")
	}
}

func TestResendError_IsRetryable_403_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 403, Body: "forbidden"}
	if e.IsRetryable() {
		t.Error("403 should NOT be retryable")
	}
}

func TestResendError_IsRetryable_404_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 404, Body: "not found"}
	if e.IsRetryable() {
		t.Error("404 should NOT be retryable")
	}
}

func TestResendError_IsRetryable_422_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: "validation"}
	if e.IsRetryable() {
		t.Error("422 should NOT be retryable")
	}
}

// --- IsDomainError ---

func TestResendError_IsDomainError_403(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 403, Body: "invalid api key"}
	if !e.IsDomainError() {
		t.Error("403 should be a domain error")
	}
}

func TestResendError_IsDomainError_422_InvalidFromAddress(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: `{"code":"invalid_from_address"}`}
	if !e.IsDomainError() {
		t.Error("422 with invalid_from_address should be a domain error")
	}
}

func TestResendError_IsDomainError_422_InvalidAccess(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: `{"code":"invalid_access"}`}
	if !e.IsDomainError() {
		t.Error("422 with invalid_access should be a domain error")
	}
}

func TestResendError_IsDomainError_422_NotFound(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: `{"code":"not_found"}`}
	if !e.IsDomainError() {
		t.Error("422 with not_found should be a domain error")
	}
}

func TestResendError_IsDomainError_422_Other(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: `{"code":"validation_error"}`}
	if e.IsDomainError() {
		t.Error("422 with generic validation should NOT be a domain error")
	}
}

func TestResendError_IsDomainError_400_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 400, Body: "bad request"}
	if e.IsDomainError() {
		t.Error("400 should NOT be a domain error")
	}
}

func TestResendError_IsDomainError_500_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 500, Body: "server error"}
	if e.IsDomainError() {
		t.Error("500 should NOT be a domain error")
	}
}

func TestResendError_IsDomainError_429_No(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 429, Body: "rate limited"}
	if e.IsDomainError() {
		t.Error("429 should NOT be a domain error")
	}
}

// --- Error() ---

func TestResendError_Error_Format(t *testing.T) {
	t.Parallel()
	e := &ResendError{StatusCode: 422, Body: "validation error"}
	got := e.Error()
	if !strings.Contains(got, "422") {
		t.Errorf("Error() should contain status code, got %q", got)
	}
	if !strings.Contains(got, "validation error") {
		t.Errorf("Error() should contain body, got %q", got)
	}
	if got != "resend: 422: validation error" {
		t.Errorf("Error() = %q, want %q", got, "resend: 422: validation error")
	}
}
