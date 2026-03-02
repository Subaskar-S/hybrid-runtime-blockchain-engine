package mcp

import (
	"sync"
	"time"
)

// RateLimiter implements per-tool rate limiting
type RateLimiter struct {
	limit    int
	window   time.Duration
	counters map[string]*counter
	mu       sync.RWMutex
}

// counter tracks requests for a specific tool
type counter struct {
	count      int
	windowStart time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*counter),
	}
}

// Allow checks if a request is allowed under the rate limit
func (rl *RateLimiter) Allow(tool string) bool {
	rl.mu.Lock()
	c, exists := rl.counters[tool]
	if !exists {
		c = &counter{
			count:      0,
			windowStart: time.Now(),
		}
		rl.counters[tool] = c
	}
	rl.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	
	// Reset counter if window has passed
	if now.Sub(c.windowStart) >= rl.window {
		c.count = 0
		c.windowStart = now
	}

	// Check if limit exceeded
	if c.count >= rl.limit {
		return false
	}

	// Increment counter
	c.count++
	return true
}

// Reset resets the rate limiter (useful for testing)
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.counters = make(map[string]*counter)
}
