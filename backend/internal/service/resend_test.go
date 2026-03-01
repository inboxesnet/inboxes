package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoRequest_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization: got %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: got %q, want %q", got, "application/json")
		}
		if r.Method != "GET" {
			t.Errorf("Method: got %q, want GET", r.Method)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	data, err := DoRequest("test-key", "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if !strings.Contains(string(data), `"id":"123"`) {
		t.Errorf("DoRequest body: got %q, want containing '\"id\":\"123\"'", string(data))
	}
}

func TestDoRequest_4xxError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"validation error"}`))
	}))
	defer srv.Close()

	_, err := DoRequest("test-key", "POST", srv.URL+"/test", nil)
	if err == nil {
		t.Fatal("DoRequest(422): expected error, got nil")
	}
	var resendErr *ResendError
	if !errors.As(err, &resendErr) {
		t.Fatalf("DoRequest(422): expected *ResendError, got %T", err)
	}
	if resendErr.StatusCode != 422 {
		t.Errorf("DoRequest(422): StatusCode = %d, want 422", resendErr.StatusCode)
	}
	if resendErr.IsRetryable() {
		t.Error("DoRequest(422): 422 should not be retryable")
	}
}

func TestDoRequest_5xxError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	_, err := DoRequest("test-key", "GET", srv.URL+"/test", nil)
	if err == nil {
		t.Fatal("DoRequest(500): expected error, got nil")
	}
	var resendErr *ResendError
	if !errors.As(err, &resendErr) {
		t.Fatalf("DoRequest(500): expected *ResendError, got %T", err)
	}
	if resendErr.StatusCode != 500 {
		t.Errorf("DoRequest(500): StatusCode = %d, want 500", resendErr.StatusCode)
	}
	if !resendErr.IsRetryable() {
		t.Error("DoRequest(500): 500 should be retryable")
	}
}

func TestDoRequest_429Error(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	_, err := DoRequest("test-key", "GET", srv.URL+"/test", nil)
	if err == nil {
		t.Fatal("DoRequest(429): expected error, got nil")
	}
	var resendErr *ResendError
	if !errors.As(err, &resendErr) {
		t.Fatalf("DoRequest(429): expected *ResendError, got %T", err)
	}
	if !resendErr.IsRetryable() {
		t.Error("DoRequest(429): 429 should be retryable")
	}
}

func TestDoRequest_NilBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("NilBody: got body %q, want empty", string(body))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	data, err := DoRequest("test-key", "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("DoRequest(nil body): %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("DoRequest(nil body): got %q, want %q", string(data), "{}")
	}
}

func TestDoRequest_MarshalBody(t *testing.T) {
	t.Parallel()
	type testBody struct {
		From    string `json:"from"`
		Subject string `json:"subject"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got testBody
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("MarshalBody: unmarshal request: %v", err)
		}
		if got.From != "test@example.com" {
			t.Errorf("MarshalBody from: got %q, want %q", got.From, "test@example.com")
		}
		if got.Subject != "Hello" {
			t.Errorf("MarshalBody subject: got %q, want %q", got.Subject, "Hello")
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"456"}`))
	}))
	defer srv.Close()

	_, err := DoRequest("test-key", "POST", srv.URL+"/test", testBody{From: "test@example.com", Subject: "Hello"})
	if err != nil {
		t.Fatalf("DoRequest(struct body): %v", err)
	}
}

func TestResendDirectFetch_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"domains":[]}`))
	}))
	defer srv.Close()

	data, err := DoRequest("test-key", "GET", srv.URL+"/domains", nil)
	if err != nil {
		t.Fatalf("ResendDirectFetch: %v", err)
	}
	if !strings.Contains(string(data), "domains") {
		t.Errorf("ResendDirectFetch: got %q, want containing 'domains'", string(data))
	}
}

func init() {
	// Suppress log output during tests
	_ = fmt.Sprintf("")
}
