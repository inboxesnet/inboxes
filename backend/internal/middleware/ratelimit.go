package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
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

// checkRateLimit uses Redis INCR + EXPIRE to enforce a sliding window.
// Returns (allowed, retryAfterSecs).
func checkRateLimit(ctx context.Context, rdb *redis.Client, key string, limit, windowSecs int) (bool, int) {
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// If Redis is down, allow the request
		return true, 0
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
