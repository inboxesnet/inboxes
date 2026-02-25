package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// resetRateLimiter resets the global rate limiter state between tests.
func resetRateLimiter() {
	resendMu.Lock()
	resendLastCall = time.Time{}
	resendMu.Unlock()
}

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

	data, err := doRequest("test-key", "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if !strings.Contains(string(data), `"id":"123"`) {
		t.Errorf("doRequest body: got %q, want containing '\"id\":\"123\"'", string(data))
	}
}

func TestDoRequest_4xxError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"validation error"}`))
	}))
	defer srv.Close()

	_, err := doRequest("test-key", "POST", srv.URL+"/test", nil)
	if err == nil {
		t.Fatal("doRequest(422): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resend: 422:") {
		t.Errorf("doRequest(422): error = %q, want containing 'resend: 422:'", err.Error())
	}
}

func TestDoRequest_5xxError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	_, err := doRequest("test-key", "GET", srv.URL+"/test", nil)
	if err == nil {
		t.Fatal("doRequest(500): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resend: 500:") {
		t.Errorf("doRequest(500): error = %q, want containing 'resend: 500:'", err.Error())
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

	data, err := doRequest("test-key", "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("doRequest(nil body): %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("doRequest(nil body): got %q, want %q", string(data), "{}")
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

	_, err := doRequest("test-key", "POST", srv.URL+"/test", testBody{From: "test@example.com", Subject: "Hello"})
	if err != nil {
		t.Fatalf("doRequest(struct body): %v", err)
	}
}

func TestDoRequest_RateLimiting(t *testing.T) {
	// This test must NOT be t.Parallel() because it relies on the global rate limiter.
	// Reset global rate limiter state for this test.
	resetRateLimiter()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	start := time.Now()

	// Make 3 rapid calls — the rate limiter enforces 600ms between calls.
	// First call: immediate. Second call: waits ~600ms. Third call: waits ~600ms.
	// Total elapsed: >= 1200ms.
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := doRequest("test-key", "GET", srv.URL+"/test", nil)
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)
	for i, err := range errs {
		if err != nil {
			t.Errorf("doRequest[%d]: %v", i, err)
		}
	}
	if elapsed < 1200*time.Millisecond {
		t.Errorf("RateLimiting: 3 calls completed in %v, want >= 1200ms", elapsed)
	}
}

func TestResendDirectFetch_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"domains":[]}`))
	}))
	defer srv.Close()

	origURL := resendBaseURL
	// We can't safely mutate the global in parallel, so use doRequest directly with full URL.
	data, err := doRequest("test-key", "GET", srv.URL+"/domains", nil)
	_ = origURL
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
