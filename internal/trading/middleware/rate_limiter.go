package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	// Per-user limits
	UserOrdersPerSecond   float64 `json:"user_orders_per_second" default:"10"`
	UserOrdersPerMinute   int     `json:"user_orders_per_minute" default:"300"`
	UserOrdersPerDay      int     `json:"user_orders_per_day" default:"100000"`
	UserRequestsPerSecond float64 `json:"user_requests_per_second" default:"20"`
	UserRequestsPerMinute int     `json:"user_requests_per_minute" default:"1200"`

	// Per-IP limits
	IPRequestsPerSecond float64 `json:"ip_requests_per_second" default:"50"`
	IPRequestsPerMinute int     `json:"ip_requests_per_minute" default:"3000"`

	// Global limits
	GlobalOrdersPerSecond   float64 `json:"global_orders_per_second" default:"1000"`
	GlobalRequestsPerSecond float64 `json:"global_requests_per_second" default:"10000"`

	// Burst limits
	UserOrderBurst     int `json:"user_order_burst" default:"20"`
	UserRequestBurst   int `json:"user_request_burst" default:"50"`
	IPRequestBurst     int `json:"ip_request_burst" default:"100"`
	GlobalOrderBurst   int `json:"global_order_burst" default:"2000"`
	GlobalRequestBurst int `json:"global_request_burst" default:"20000"`

	// Window periods for counting limits
	MinuteWindow time.Duration `json:"minute_window" default:"1m"`
	DayWindow    time.Duration `json:"day_window" default:"24h"`

	// Rate limiter cleanup interval
	CleanupInterval time.Duration `json:"cleanup_interval" default:"5m"`
	LimiterTTL      time.Duration `json:"limiter_ttl" default:"1h"`
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		UserOrdersPerSecond:     10,
		UserOrdersPerMinute:     300,
		UserOrdersPerDay:        100000,
		UserRequestsPerSecond:   20,
		UserRequestsPerMinute:   1200,
		IPRequestsPerSecond:     50,
		IPRequestsPerMinute:     3000,
		GlobalOrdersPerSecond:   1000,
		GlobalRequestsPerSecond: 10000,
		UserOrderBurst:          20,
		UserRequestBurst:        50,
		IPRequestBurst:          100,
		GlobalOrderBurst:        2000,
		GlobalRequestBurst:      20000,
		MinuteWindow:            time.Minute,
		DayWindow:               24 * time.Hour,
		CleanupInterval:         5 * time.Minute,
		LimiterTTL:              time.Hour,
	}
}

// UserLimiter tracks rate limits for a specific user
type UserLimiter struct {
	OrderLimiter   *rate.Limiter
	RequestLimiter *rate.Limiter
	OrderCount     *WindowCounter
	LastAccess     time.Time
	mutex          sync.RWMutex
}

// IPLimiter tracks rate limits for a specific IP
type IPLimiter struct {
	RequestLimiter *rate.Limiter
	RequestCount   *WindowCounter
	LastAccess     time.Time
	mutex          sync.RWMutex
}

// WindowCounter tracks counts within time windows
type WindowCounter struct {
	Counts     map[int64]int
	WindowSize time.Duration
	mutex      sync.RWMutex
}

// NewWindowCounter creates a new window counter
func NewWindowCounter(windowSize time.Duration) *WindowCounter {
	return &WindowCounter{
		Counts:     make(map[int64]int),
		WindowSize: windowSize,
	}
}

// Increment increments the counter for the current window
func (wc *WindowCounter) Increment() {
	wc.mutex.Lock()
	defer wc.mutex.Unlock()

	now := time.Now()
	window := now.Truncate(wc.WindowSize).Unix()
	wc.Counts[window]++

	// Clean old windows
	cutoff := now.Add(-wc.WindowSize * 2).Truncate(wc.WindowSize).Unix()
	for w := range wc.Counts {
		if w < cutoff {
			delete(wc.Counts, w)
		}
	}
}

// Count returns the total count within the window period
func (wc *WindowCounter) Count(period time.Duration) int {
	wc.mutex.RLock()
	defer wc.mutex.RUnlock()

	now := time.Now()
	cutoff := now.Add(-period).Truncate(wc.WindowSize).Unix()

	total := 0
	for window, count := range wc.Counts {
		if window >= cutoff {
			total += count
		}
	}
	return total
}

// RateLimiter implements comprehensive rate limiting
type RateLimiter struct {
	config        *RateLimitConfig
	logger        *zap.Logger
	userLimiters  map[string]*UserLimiter
	ipLimiters    map[string]*IPLimiter
	globalOrder   *rate.Limiter
	globalRequest *rate.Limiter
	mutex         sync.RWMutex
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimitConfig, logger *zap.Logger) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	rl := &RateLimiter{
		config:        config,
		logger:        logger,
		userLimiters:  make(map[string]*UserLimiter),
		ipLimiters:    make(map[string]*IPLimiter),
		globalOrder:   rate.NewLimiter(rate.Limit(config.GlobalOrdersPerSecond), config.GlobalOrderBurst),
		globalRequest: rate.NewLimiter(rate.Limit(config.GlobalRequestsPerSecond), config.GlobalRequestBurst),
		stopChan:      make(chan struct{}),
	}

	// Start cleanup goroutine
	rl.wg.Add(1)
	go rl.cleanupLoop()

	return rl
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
	rl.wg.Wait()
}

// cleanupLoop periodically cleans up old limiters
func (rl *RateLimiter) cleanupLoop() {
	defer rl.wg.Done()

	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopChan:
			return
		}
	}
}

// cleanup removes old unused limiters
func (rl *RateLimiter) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.config.LimiterTTL)

	// Clean user limiters
	for userID, limiter := range rl.userLimiters {
		limiter.mutex.RLock()
		lastAccess := limiter.LastAccess
		limiter.mutex.RUnlock()

		if lastAccess.Before(cutoff) {
			delete(rl.userLimiters, userID)
		}
	}

	// Clean IP limiters
	for ip, limiter := range rl.ipLimiters {
		limiter.mutex.RLock()
		lastAccess := limiter.LastAccess
		limiter.mutex.RUnlock()

		if lastAccess.Before(cutoff) {
			delete(rl.ipLimiters, ip)
		}
	}

	rl.logger.Debug("Rate limiter cleanup completed",
		zap.Int("active_user_limiters", len(rl.userLimiters)),
		zap.Int("active_ip_limiters", len(rl.ipLimiters)),
	)
}

// getUserLimiter gets or creates a user limiter
func (rl *RateLimiter) getUserLimiter(userID string) *UserLimiter {
	rl.mutex.RLock()
	limiter, exists := rl.userLimiters[userID]
	rl.mutex.RUnlock()

	if exists {
		limiter.mutex.Lock()
		limiter.LastAccess = time.Now()
		limiter.mutex.Unlock()
		return limiter
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.userLimiters[userID]; exists {
		limiter.mutex.Lock()
		limiter.LastAccess = time.Now()
		limiter.mutex.Unlock()
		return limiter
	}

	// Create new user limiter
	limiter = &UserLimiter{
		OrderLimiter:   rate.NewLimiter(rate.Limit(rl.config.UserOrdersPerSecond), rl.config.UserOrderBurst),
		RequestLimiter: rate.NewLimiter(rate.Limit(rl.config.UserRequestsPerSecond), rl.config.UserRequestBurst),
		OrderCount:     NewWindowCounter(time.Minute),
		LastAccess:     time.Now(),
	}
	rl.userLimiters[userID] = limiter

	return limiter
}

// getIPLimiter gets or creates an IP limiter
func (rl *RateLimiter) getIPLimiter(ip string) *IPLimiter {
	rl.mutex.RLock()
	limiter, exists := rl.ipLimiters[ip]
	rl.mutex.RUnlock()

	if exists {
		limiter.mutex.Lock()
		limiter.LastAccess = time.Now()
		limiter.mutex.Unlock()
		return limiter
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.ipLimiters[ip]; exists {
		limiter.mutex.Lock()
		limiter.LastAccess = time.Now()
		limiter.mutex.Unlock()
		return limiter
	}

	// Create new IP limiter
	limiter = &IPLimiter{
		RequestLimiter: rate.NewLimiter(rate.Limit(rl.config.IPRequestsPerSecond), rl.config.IPRequestBurst),
		RequestCount:   NewWindowCounter(time.Minute),
		LastAccess:     time.Now(),
	}
	rl.ipLimiters[ip] = limiter

	return limiter
}

// CheckOrderLimit checks if an order can be placed
func (rl *RateLimiter) CheckOrderLimit(ctx context.Context, userID, ip string) error {
	// Check global order limit
	if !rl.globalOrder.Allow() {
		return fmt.Errorf("global order rate limit exceeded")
	}

	// Check user order limits
	userLimiter := rl.getUserLimiter(userID)

	// Check per-second limit
	if !userLimiter.OrderLimiter.Allow() {
		return fmt.Errorf("user order rate limit exceeded (per second)")
	}

	// Check per-minute limit
	userLimiter.OrderCount.Increment()
	if userLimiter.OrderCount.Count(rl.config.MinuteWindow) > rl.config.UserOrdersPerMinute {
		return fmt.Errorf("user order rate limit exceeded (per minute)")
	}

	// Check per-day limit
	if userLimiter.OrderCount.Count(rl.config.DayWindow) > rl.config.UserOrdersPerDay {
		return fmt.Errorf("user order rate limit exceeded (per day)")
	}

	return nil
}

// CheckRequestLimit checks if a request can be processed
func (rl *RateLimiter) CheckRequestLimit(ctx context.Context, userID, ip string) error {
	// Check global request limit
	if !rl.globalRequest.Allow() {
		return fmt.Errorf("global request rate limit exceeded")
	}

	// Check IP limits if provided
	if ip != "" {
		ipLimiter := rl.getIPLimiter(ip)
		if !ipLimiter.RequestLimiter.Allow() {
			return fmt.Errorf("IP request rate limit exceeded")
		}

		ipLimiter.RequestCount.Increment()
		if ipLimiter.RequestCount.Count(rl.config.MinuteWindow) > rl.config.IPRequestsPerMinute {
			return fmt.Errorf("IP request rate limit exceeded (per minute)")
		}
	}

	// Check user limits if provided
	if userID != "" {
		userLimiter := rl.getUserLimiter(userID)
		if !userLimiter.RequestLimiter.Allow() {
			return fmt.Errorf("user request rate limit exceeded")
		}
	}

	return nil
}

// GetUserStats returns rate limit statistics for a user
func (rl *RateLimiter) GetUserStats(userID string) map[string]interface{} {
	rl.mutex.RLock()
	limiter, exists := rl.userLimiters[userID]
	rl.mutex.RUnlock()

	if !exists {
		return map[string]interface{}{
			"orders_per_minute": 0,
			"orders_per_day":    0,
			"order_tokens":      rl.config.UserOrderBurst,
			"request_tokens":    rl.config.UserRequestBurst,
		}
	}

	limiter.mutex.RLock()
	defer limiter.mutex.RUnlock()

	return map[string]interface{}{
		"orders_per_minute": limiter.OrderCount.Count(rl.config.MinuteWindow),
		"orders_per_day":    limiter.OrderCount.Count(rl.config.DayWindow),
		"order_tokens":      limiter.OrderLimiter.Tokens(),
		"request_tokens":    limiter.RequestLimiter.Tokens(),
	}
}

// Middleware functions

// OrderRateLimitMiddleware creates middleware for order endpoints
func (rl *RateLimiter) OrderRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		ip := c.ClientIP()

		if err := rl.CheckOrderLimit(c.Request.Context(), userID, ip); err != nil {
			rl.logger.Warn("Order rate limit exceeded",
				zap.String("user_id", userID),
				zap.String("ip", ip),
				zap.Error(err),
			)

			c.Header("X-RateLimit-Limit", strconv.Itoa(int(rl.config.UserOrdersPerSecond)))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Second).Unix(), 10))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    "RATE_LIMIT_EXCEEDED",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequestRateLimitMiddleware creates middleware for general endpoints
func (rl *RateLimiter) RequestRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		ip := c.ClientIP()

		if err := rl.CheckRequestLimit(c.Request.Context(), userID, ip); err != nil {
			rl.logger.Warn("Request rate limit exceeded",
				zap.String("user_id", userID),
				zap.String("ip", ip),
				zap.Error(err),
			)

			c.Header("X-RateLimit-Limit", strconv.Itoa(int(rl.config.UserRequestsPerSecond)))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Second).Unix(), 10))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    "RATE_LIMIT_EXCEEDED",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		if userID != "" {
			stats := rl.GetUserStats(userID)
			if tokens, ok := stats["request_tokens"].(float64); ok {
				c.Header("X-RateLimit-Remaining", strconv.FormatInt(int64(tokens), 10))
			}
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(int(rl.config.UserRequestsPerSecond)))

		c.Next()
	}
}
