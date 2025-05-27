package api

import (
	"sync"
	"time"
)

// RateLimiter struct to manage rate limiting
type RateLimiter struct {
	limit      int
	interval   time.Duration
	tokens     chan struct{}
	mu         sync.Mutex
}

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limit:    limit,
		interval: interval,
		tokens:   make(chan struct{}, limit),
	}

	go rl.fillTokens()
	return rl
}

// fillTokens fills the token bucket at the specified interval
func (rl *RateLimiter) fillTokens() {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		rl.mu.Lock()
		for i := 0; i < rl.limit; i++ {
			select {
			case rl.tokens <- struct{}{}:
			default:
				return
			}
		}
		rl.mu.Unlock()
	}
}

// Wait blocks until a token is available
func (rl *RateLimiter) Wait() {
	<-rl.tokens
}

// GetAvailableTokens returns the number of available tokens
func (rl *RateLimiter) GetAvailableTokens() int {
	return len(rl.tokens)
}