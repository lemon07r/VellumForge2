package api

import (
	"context"
	"log/slog"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiterPool manages per-model rate limiters
type RateLimiterPool struct {
	limiters map[string]*rate.Limiter
	rates    map[string]int // Track original rates for consistency check
	mu       sync.RWMutex
}

// NewRateLimiterPool creates a new rate limiter pool
func NewRateLimiterPool() *RateLimiterPool {
	return &RateLimiterPool{
		limiters: make(map[string]*rate.Limiter),
		rates:    make(map[string]int),
	}
}

// GetOrCreate returns an existing rate limiter or creates a new one
// If a limiter exists with a different rate, it logs a warning and keeps the existing one
func (p *RateLimiterPool) GetOrCreate(modelID string, requestsPerMinute int) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()

	if limiter, exists := p.limiters[modelID]; exists {
		// Check if rate has changed
		if existingRate, ok := p.rates[modelID]; ok && existingRate != requestsPerMinute {
			expectedRPS := float64(requestsPerMinute) / 60.0
			existingRPS := float64(existingRate) / 60.0
			slog.Warn("Rate limiter already exists with different rate, using existing rate",
				"model_id", modelID,
				"existing_rpm", existingRate,
				"existing_rps", existingRPS,
				"requested_rpm", requestsPerMinute,
				"requested_rps", expectedRPS)
		}
		return limiter
	}

	// Create new limiter: convert requests per minute to requests per second
	rps := float64(requestsPerMinute) / 60.0
	burst := max(5, requestsPerMinute/5) // Allow 20% burst capacity (increased from 10%)
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	p.limiters[modelID] = limiter
	p.rates[modelID] = requestsPerMinute

	slog.Debug("Created rate limiter",
		"model_id", modelID,
		"rpm", requestsPerMinute,
		"rps", rps,
		"burst", burst)

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
