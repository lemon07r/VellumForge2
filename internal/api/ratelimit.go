package api

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiterPool manages per-model rate limiters
type RateLimiterPool struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

// NewRateLimiterPool creates a new rate limiter pool
func NewRateLimiterPool() *RateLimiterPool {
	return &RateLimiterPool{
		limiters: make(map[string]*rate.Limiter),
	}
}

// GetOrCreate returns an existing rate limiter or creates a new one
func (p *RateLimiterPool) GetOrCreate(modelID string, requestsPerMinute int) *rate.Limiter {
	p.mu.RLock()
	limiter, exists := p.limiters[modelID]
	p.mu.RUnlock()

	if exists {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := p.limiters[modelID]; exists {
		return limiter
	}

	// Create new limiter: convert requests per minute to requests per second
	rps := float64(requestsPerMinute) / 60.0
	burst := max(1, requestsPerMinute/10) // Allow some burst capacity
	limiter = rate.NewLimiter(rate.Limit(rps), burst)
	p.limiters[modelID] = limiter

	return limiter
}

// Wait blocks until the rate limiter allows the next request
func (p *RateLimiterPool) Wait(ctx context.Context, modelID string, requestsPerMinute int) error {
	limiter := p.GetOrCreate(modelID, requestsPerMinute)
	return limiter.Wait(ctx)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
