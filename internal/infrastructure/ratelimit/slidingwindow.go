// slidingwindow.go: Sliding window algorithm implementation
// Production-ready: all stubs removed, correct logic for Allow, Take, AllowN, TakeN, Peek implemented.
package ratelimit

import (
	"sync"
	"time"
)

// SlidingWindow implements a thread-safe, in-memory sliding window rate limiter.
type SlidingWindow struct {
	limit    int           // max requests per window
	window   time.Duration // window size
	requests []int64       // timestamps (unix nanos)
	mu       sync.Mutex    // protects state
}

// NewSlidingWindow creates a new sliding window limiter.
func NewSlidingWindow(limit int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		limit:    limit,
		window:   window,
		requests: make([]int64, 0, limit+1),
	}
}

// Allow checks if a request is allowed (does not record).
func (sw *SlidingWindow) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	return len(sw.requests) < sw.limit
}

// Take records a request and returns true if allowed, false if rate limited.
func (sw *SlidingWindow) Take() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	if len(sw.requests) < sw.limit {
		sw.requests = append(sw.requests, now)
		return true
	}
	return false
}

// cleanup removes timestamps outside the window.
func (sw *SlidingWindow) cleanup(now int64) {
	cutoff := now - sw.window.Nanoseconds()
	idx := 0
	for idx < len(sw.requests) && sw.requests[idx] < cutoff {
		idx++
	}
	if idx > 0 {
		sw.requests = sw.requests[idx:]
	}
}

// SetLimit dynamically updates the request limit.
func (sw *SlidingWindow) SetLimit(limit int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.limit = limit
	if len(sw.requests) > limit {
		sw.requests = sw.requests[len(sw.requests)-limit:]
	}
}

// SetWindow dynamically updates the window size.
func (sw *SlidingWindow) SetWindow(window time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.window = window
}

// Remaining returns the number of requests left in the window.
func (sw *SlidingWindow) Remaining() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	return sw.limit - len(sw.requests)
}

// Reset clears the window.
func (sw *SlidingWindow) Reset() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.requests = sw.requests[:0]
}

// Remove broken Allow and Peek interface stubs and implement correct logic for multi-request Allow and Peek.
// Allow checks if n requests are allowed in the current window (for interface compatibility)
func (sw *SlidingWindow) AllowN(n int) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	return len(sw.requests)+n <= sw.limit
}

// TakeN records n requests and returns true if allowed, false if rate limited.
func (sw *SlidingWindow) TakeN(n int) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	if len(sw.requests)+n <= sw.limit {
		for i := 0; i < n; i++ {
			sw.requests = append(sw.requests, now)
		}
		return true
	}
	return false
}

// Peek returns remaining requests and reset time (when the window will allow a new request)
func (sw *SlidingWindow) Peek() (remaining int, reset time.Time) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	now := time.Now().UnixNano()
	sw.cleanup(now)
	remaining = sw.limit - len(sw.requests)
	reset = time.Now()
	if len(sw.requests) > 0 {
		oldest := sw.requests[0]
		reset = time.Unix(0, oldest+sw.window.Nanoseconds())
	}
	return
}
