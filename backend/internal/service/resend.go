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

var httpClient = &http.Client{Timeout: 30 * time.Second}

// ResendBaseURL returns the base URL used for Resend API requests.
func ResendBaseURL() string { return resendBaseURL }

// ResendError wraps Resend API errors with status code for retry decisions.
type ResendError struct {
	StatusCode int
	Body       string
}

func (e *ResendError) Error() string {
	return fmt.Sprintf("resend: %d: %s", e.StatusCode, e.Body)
}

// IsRetryable returns true for rate limits (429), concurrency conflicts (409),
// and server errors (5xx).
func (e *ResendError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode == 409 || e.StatusCode >= 500
}

// IsDomainError returns true when the error indicates the domain or API key
// is no longer valid — the domain should be marked disconnected.
// Matches: 403 (invalid_api_key), 422 with domain-specific error codes.
func (e *ResendError) IsDomainError() bool {
	if e.StatusCode == 403 {
		return true
	}
	if e.StatusCode == 422 {
		body := strings.ToLower(e.Body)
		return strings.Contains(body, "invalid_from_address") ||
			strings.Contains(body, "invalid_access") ||
			strings.Contains(body, "not_found")
	}
	return false
}

type orgKeyCacheEntry struct {
	key      string
	cachedAt time.Time
}

type ResendService struct {
	encSvc         *EncryptionService
	pool           *pgxpool.Pool
	systemKey      string
	systemFromAddr string

	mu              sync.Mutex
	cachedDBKey     string
	dbKeyCacheValid bool

	cachedFromAddr string
	cachedFromName string
	fromCacheValid bool

	orgKeyMu    sync.RWMutex
	orgKeyCache map[string]orgKeyCacheEntry
}

func NewResendService(encSvc *EncryptionService, pool *pgxpool.Pool, systemKey, systemFromAddr string) *ResendService {
	return &ResendService{
		encSvc:         encSvc,
		pool:           pool,
		systemKey:      systemKey,
		systemFromAddr: systemFromAddr,
		orgKeyCache:    make(map[string]orgKeyCacheEntry),
	}
}

func (s *ResendService) Fetch(ctx context.Context, orgID, method, path string, body interface{}) ([]byte, error) {
	apiKey, err := s.GetOrgAPIKey(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return DoRequest(apiKey, method, resendBaseURL+path, body)
}

// GetOrgAPIKey fetches and decrypts the Resend API key for an org.
// Results are cached in memory for 5 minutes to avoid repeated DB + decrypt calls.
func (s *ResendService) GetOrgAPIKey(ctx context.Context, orgID string) (string, error) {
	const cacheTTL = 5 * time.Minute

	// Fast path: read lock
	s.orgKeyMu.RLock()
	if entry, ok := s.orgKeyCache[orgID]; ok && time.Since(entry.cachedAt) < cacheTTL {
		s.orgKeyMu.RUnlock()
		return entry.key, nil
	}
	s.orgKeyMu.RUnlock()

	// Slow path: write lock, double-check
	s.orgKeyMu.Lock()
	defer s.orgKeyMu.Unlock()

	if entry, ok := s.orgKeyCache[orgID]; ok && time.Since(entry.cachedAt) < cacheTTL {
		return entry.key, nil
	}

	var encrypted, iv, tag string
	err := s.pool.QueryRow(ctx,
		"SELECT resend_api_key_encrypted, resend_api_key_iv, resend_api_key_tag FROM orgs WHERE id = $1", orgID,
	).Scan(&encrypted, &iv, &tag)
	if err != nil {
		return "", fmt.Errorf("resend: fetch api key for org %s: %w", orgID, err)
	}
	apiKey, err := s.encSvc.Decrypt(encrypted, iv, tag)
	if err != nil {
		return "", fmt.Errorf("resend: decrypt api key for org %s: %w", orgID, err)
	}

	s.orgKeyCache[orgID] = orgKeyCacheEntry{key: apiKey, cachedAt: time.Now()}
	return apiKey, nil
}

// InvalidateOrgKeyCache removes the cached API key for an org, forcing a fresh
// DB fetch + decrypt on the next call. Used after API key rotation.
func (s *ResendService) InvalidateOrgKeyCache(orgID string) {
	s.orgKeyMu.Lock()
	defer s.orgKeyMu.Unlock()
	delete(s.orgKeyCache, orgID)
}

// getSystemKey returns the system Resend API key, checking env var first then DB cache.
func (s *ResendService) getSystemKey(ctx context.Context) string {
	if s.systemKey != "" {
		return s.systemKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dbKeyCacheValid {
		return s.cachedDBKey
	}

	// Query from system_settings
	var encrypted, iv, tag string
	var isEncrypted bool
	err := s.pool.QueryRow(ctx,
		`SELECT value, iv, tag, encrypted FROM system_settings WHERE key = 'resend_system_api_key'`,
	).Scan(&encrypted, &iv, &tag, &isEncrypted)
	if err != nil {
		s.cachedDBKey = ""
		s.dbKeyCacheValid = true
		return ""
	}

	if isEncrypted && s.encSvc != nil {
		decrypted, err := s.encSvc.Decrypt(encrypted, iv, tag)
		if err != nil {
			slog.Error("resend: failed to decrypt system key from DB", "error", err)
			s.cachedDBKey = ""
			s.dbKeyCacheValid = true
			return ""
		}
		s.cachedDBKey = decrypted
	} else {
		s.cachedDBKey = encrypted
	}

	s.dbKeyCacheValid = true
	return s.cachedDBKey
}

// HasSystemKey returns whether a system Resend API key is available.
func (s *ResendService) HasSystemKey(ctx context.Context) bool {
	return s.getSystemKey(ctx) != ""
}

// InvalidateSystemKeyCache clears the DB key cache so the next call re-reads from DB.
func (s *ResendService) InvalidateSystemKeyCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedDBKey = ""
	s.dbKeyCacheValid = false
}

// GetSystemFromAddress reads the system_from_address from system_settings, with caching.
// Falls back to the SYSTEM_FROM_ADDRESS env var if DB has no value.
func (s *ResendService) GetSystemFromAddress(ctx context.Context) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fromCacheValid {
		if s.cachedFromAddr != "" {
			return s.cachedFromAddr
		}
		return s.systemFromAddr
	}

	s.loadFromCache(ctx)
	if s.cachedFromAddr != "" {
		return s.cachedFromAddr
	}
	return s.systemFromAddr
}

// GetSystemFromName reads the system_from_name from system_settings, with caching.
func (s *ResendService) GetSystemFromName(ctx context.Context) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fromCacheValid {
		return s.cachedFromName
	}

	s.loadFromCache(ctx)
	return s.cachedFromName
}

// loadFromCache populates cachedFromAddr and cachedFromName from DB. Must be called with mu held.
func (s *ResendService) loadFromCache(ctx context.Context) {
	s.fromCacheValid = true

	var addr string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = 'system_from_address'`,
	).Scan(&addr)
	if err == nil {
		s.cachedFromAddr = addr
	}

	var name string
	err = s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = 'system_from_name'`,
	).Scan(&name)
	if err == nil {
		s.cachedFromName = name
	}
}

// GetSystemFrom returns the formatted "Name <address>" string, or empty if unconfigured.
func (s *ResendService) GetSystemFrom(ctx context.Context) string {
	addr := s.GetSystemFromAddress(ctx)
	if addr == "" {
		return ""
	}
	name := s.GetSystemFromName(ctx)
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

// InvalidateFromCache resets the from-address cache so the next call re-reads from DB.
func (s *ResendService) InvalidateFromCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedFromAddr = ""
	s.cachedFromName = ""
	s.fromCacheValid = false
}

func (s *ResendService) SystemFetch(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	key := s.getSystemKey(ctx)
	if key == "" {
		return nil, nil // graceful degradation
	}
	return DoRequest(key, method, resendBaseURL+path, body)
}

// ResendDirectFetch makes a request with a raw API key (used during onboarding before key is stored).
func ResendDirectFetch(apiKey, method, path string, body interface{}) ([]byte, error) {
	return DoRequest(apiKey, method, resendBaseURL+path, body)
}

// DoRequest makes an HTTP request to the Resend API. Rate limiting is handled
// externally by the queue's RateLimiter — this function does no throttling.
func DoRequest(apiKey, method, url string, body interface{}) ([]byte, error) {
	path := strings.TrimPrefix(url, resendBaseURL)

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

	resp, err := httpClient.Do(req)
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
		return nil, &ResendError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	logBody := string(respBody)
	if len(logBody) > 500 {
		logBody = logBody[:500] + "..."
	}
	slog.Info("resend: response", "method", method, "path", path, "status", resp.StatusCode, "body", logBody)

	return respBody, nil
}
