package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitByIP returns middleware that rate-limits requests by client IP.
// limit is the max number of requests, windowSecs is the time window in seconds.
func RateLimitByIP(rdb *redis.Client, limit int, windowSecs int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			key := fmt.Sprintf("rl:%s:%s", r.URL.Path, ip)

			allowed, retryAfter := checkRateLimit(r.Context(), rdb, key, limit, windowSecs)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByUser returns middleware that rate-limits requests by authenticated user ID.
// limit is the max number of requests, windowSecs is the time window in seconds.
func RateLimitByUser(rdb *redis.Client, limit int, windowSecs int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetCurrentUser(r.Context())
			if claims == nil || claims.UserID == "" {
				next.ServeHTTP(w, r)
				return
			}
			key := fmt.Sprintf("rl:user:%s:%s", r.URL.Path, claims.UserID)

			allowed, retryAfter := checkRateLimit(r.Context(), rdb, key, limit, windowSecs)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByBodyField returns middleware that rate-limits requests by a JSON
// body field value (e.g. "email"). This is used alongside RateLimitByIP to
// prevent abuse targeting a specific identity.
func RateLimitByBodyField(rdb *redis.Client, fieldName string, limit int, windowSecs int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			var body map[string]interface{}
			if json.Unmarshal(bodyBytes, &body) != nil {
				next.ServeHTTP(w, r)
				return
			}

			val, ok := body[fieldName].(string)
			if !ok || val == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := fmt.Sprintf("rl:body:%s:%s", r.URL.Path, strings.ToLower(val))
			allowed, retryAfter := checkRateLimit(r.Context(), rdb, key, limit, windowSecs)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// memoryCounter tracks per-key request counts for in-memory rate limiting.
type memoryCounter struct {
	count     atomic.Int64
	expiresAt atomic.Int64
}

// memoryCounters is the in-memory rate limit fallback store.
var memoryCounters sync.Map // key → *memoryCounter

// checkMemoryRateLimit enforces per-process rate limits when Redis is unavailable.
func checkMemoryRateLimit(key string, limit, windowSecs int) (bool, int) {
	now := time.Now().Unix()
	val, _ := memoryCounters.LoadOrStore(key, &memoryCounter{})
	counter := val.(*memoryCounter)

	// Reset counter if window has expired
	if counter.expiresAt.Load() <= now {
		counter.count.Store(1)
		counter.expiresAt.Store(now + int64(windowSecs))
		return true, 0
	}

	count := counter.count.Add(1)
	if int(count) > limit {
		retryAfter := int(counter.expiresAt.Load() - now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}
	return true, 0
}

// checkRateLimit uses Redis INCR + EXPIRE to enforce a sliding window.
// Falls back to in-memory counters if Redis is unavailable.
// Returns (allowed, retryAfterSecs).
func checkRateLimit(ctx context.Context, rdb *redis.Client, key string, limit, windowSecs int) (bool, int) {
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// Redis is down — fall back to in-memory rate limiting
		return checkMemoryRateLimit(key, limit, windowSecs)
	}

	if count == 1 {
		rdb.Expire(ctx, key, time.Duration(windowSecs)*time.Second)
	}

	if int(count) > limit {
		ttl, _ := rdb.TTL(ctx, key).Result()
		return false, int(ttl.Seconds())
	}

	return true, 0
}
