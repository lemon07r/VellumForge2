package api

import (
	"context"
	"log/slog"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiterPool manages per-model and per-provider rate limiters
type RateLimiterPool struct {
	limiters         map[string]*rate.Limiter
	rates            map[string]int // Track original rates for consistency check
	providerLimiters map[string]*rate.Limiter
	providerRates    map[string]int
	mu               sync.RWMutex
}

// NewRateLimiterPool creates a new rate limiter pool
func NewRateLimiterPool() *RateLimiterPool {
	return &RateLimiterPool{
		limiters:         make(map[string]*rate.Limiter),
		rates:            make(map[string]int),
		providerLimiters: make(map[string]*rate.Limiter),
		providerRates:    make(map[string]int),
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

// GetOrCreateProvider returns an existing provider rate limiter or creates a new one
// Uses more conservative burst capacity (10%) compared to model-level limiters (20%)
// to prevent burst overages when multiple models share the same provider
func (p *RateLimiterPool) GetOrCreateProvider(providerName string, requestsPerMinute int) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()

	if limiter, exists := p.providerLimiters[providerName]; exists {
		// Check if rate has changed
		if existingRate, ok := p.providerRates[providerName]; ok && existingRate != requestsPerMinute {
			expectedRPS := float64(requestsPerMinute) / 60.0
			existingRPS := float64(existingRate) / 60.0
			slog.Warn("Provider rate limiter already exists with different rate, using existing rate",
				"provider", providerName,
				"existing_rpm", existingRate,
				"existing_rps", existingRPS,
				"requested_rpm", requestsPerMinute,
				"requested_rps", expectedRPS)
		}
		return limiter
	}

	// Create new limiter: convert requests per minute to requests per second
	rps := float64(requestsPerMinute) / 60.0
	// Use conservative 10% burst for provider-level limiters to prevent burst overages
	// when multiple models/workers share the same provider endpoint
	burst := max(3, requestsPerMinute/10)
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	p.providerLimiters[providerName] = limiter
	p.providerRates[providerName] = requestsPerMinute

	slog.Debug("Created provider rate limiter",
		"provider", providerName,
		"rpm", requestsPerMinute,
		"rps", rps,
		"burst", burst)

	return limiter
}

// Wait blocks until the rate limiter allows the next request
// If providerName is not empty and providerRPM > 0, uses provider-level rate limiting
func (p *RateLimiterPool) Wait(ctx context.Context, modelID string, requestsPerMinute int, providerName string, providerRPM int) error {
	// Use provider-level rate limiting if configured
	if providerName != "" && providerRPM > 0 {
		limiter := p.GetOrCreateProvider(providerName, providerRPM)
		return limiter.Wait(ctx)
	}

	// Fall back to model-level rate limiting
	limiter := p.GetOrCreate(modelID, requestsPerMinute)
	return limiter.Wait(ctx)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
