package queue

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OrgLimiterMap manages per-org rate limiters for Resend API calls.
// It satisfies the service.Waiter interface (Wait + WaitForOrg).
type OrgLimiterMap struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	pool     *pgxpool.Pool

	// globalLimiter is used for non-org calls (e.g., SyncService via Waiter interface)
	globalLimiter *RateLimiter
}

// NewOrgLimiterMap creates a per-org limiter map with the given default RPS.
func NewOrgLimiterMap(pool *pgxpool.Pool, defaultRPS int) *OrgLimiterMap {
	return &OrgLimiterMap{
		limiters:      make(map[string]*RateLimiter),
		pool:          pool,
		globalLimiter: NewRateLimiter(rpsToInterval(defaultRPS)),
	}
}

// Wait implements service.Waiter — uses the global limiter.
func (m *OrgLimiterMap) Wait(ctx context.Context) error {
	return m.globalLimiter.Wait(ctx)
}

// WaitForOrg blocks until the per-org rate limit interval has elapsed.
func (m *OrgLimiterMap) WaitForOrg(ctx context.Context, orgID string) error {
	limiter := m.getOrCreate(ctx, orgID)
	return limiter.Wait(ctx)
}

// UpdateOrgRPS dynamically updates the rate limit for an org.
func (m *OrgLimiterMap) UpdateOrgRPS(orgID string, rps int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limiters[orgID] = NewRateLimiter(rpsToInterval(rps))
}

func (m *OrgLimiterMap) getOrCreate(ctx context.Context, orgID string) *RateLimiter {
	m.mu.RLock()
	if rl, ok := m.limiters[orgID]; ok {
		m.mu.RUnlock()
		return rl
	}
	m.mu.RUnlock()

	// Fetch from DB
	var rps int
	err := m.pool.QueryRow(ctx,
		"SELECT resend_rps FROM orgs WHERE id = $1", orgID,
	).Scan(&rps)
	if err != nil || rps <= 0 {
		rps = 2
	}

	rl := NewRateLimiter(rpsToInterval(rps))

	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock
	if existing, ok := m.limiters[orgID]; ok {
		return existing
	}
	m.limiters[orgID] = rl
	return rl
}

// rpsToInterval converts requests-per-second to a minimum interval with 15% safety margin.
func rpsToInterval(rps int) time.Duration {
	if rps <= 0 {
		rps = 2
	}
	return time.Duration(float64(time.Second) / float64(rps) * 1.15)
}
