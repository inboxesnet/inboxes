package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var resendBaseURL = "https://api.resend.com"

// Global rate limiter: one Resend API call at a time, 600ms between calls.
var resendMu sync.Mutex
var resendLastCall time.Time

type ResendService struct {
	encSvc    *EncryptionService
	pool      *pgxpool.Pool
	systemKey string
}

func NewResendService(encSvc *EncryptionService, pool *pgxpool.Pool, systemKey string) *ResendService {
	return &ResendService{encSvc: encSvc, pool: pool, systemKey: systemKey}
}

func (s *ResendService) Fetch(ctx context.Context, orgID, method, path string, body interface{}) ([]byte, error) {
	var encrypted, iv, tag string
	err := s.pool.QueryRow(ctx,
		"SELECT resend_api_key_encrypted, resend_api_key_iv, resend_api_key_tag FROM orgs WHERE id = $1", orgID,
	).Scan(&encrypted, &iv, &tag)
	if err != nil {
		return nil, fmt.Errorf("resend: fetch api key for org %s: %w", orgID, err)
	}
	apiKey, err := s.encSvc.Decrypt(encrypted, iv, tag)
	if err != nil {
		return nil, fmt.Errorf("resend: decrypt api key for org %s: %w", orgID, err)
	}
	return doRequest(apiKey, method, resendBaseURL+path, body)
}

func (s *ResendService) SystemFetch(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	if s.systemKey == "" {
		return nil, fmt.Errorf("resend: system api key not configured")
	}
	return doRequest(s.systemKey, method, resendBaseURL+path, body)
}

// ResendDirectFetch makes a request with a raw API key (used during onboarding before key is stored).
func ResendDirectFetch(apiKey, method, path string, body interface{}) ([]byte, error) {
	return doRequest(apiKey, method, resendBaseURL+path, body)
}

func doRequest(apiKey, method, url string, body interface{}) ([]byte, error) {
	// Extract path for logging (strip base URL, never log full URL with key params)
	path := strings.TrimPrefix(url, resendBaseURL)

	// Global rate limit: wait until 600ms since last call
	resendMu.Lock()
	if elapsed := time.Since(resendLastCall); elapsed < 600*time.Millisecond {
		time.Sleep(600*time.Millisecond - elapsed)
	}
	resendLastCall = time.Now()
	resendMu.Unlock()

	slog.Info("resend: request", "method", method, "path", path)

	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("resend: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("resend: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("resend: request failed", "method", method, "path", path, "error", err)
		return nil, fmt.Errorf("resend: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("resend: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("resend: error response", "method", method, "path", path, "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("resend: %d: %s", resp.StatusCode, string(respBody))
	}

	// Truncate response body for success logging (max 500 chars)
	logBody := string(respBody)
	if len(logBody) > 500 {
		logBody = logBody[:500] + "..."
	}
	slog.Info("resend: response", "method", method, "path", path, "status", resp.StatusCode, "body", logBody)

	return respBody, nil
}
