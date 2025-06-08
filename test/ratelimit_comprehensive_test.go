package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/finalex/pkg/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Test configuration
var (
	testRedisAddr     = "localhost:6379"
	testRedisPassword = ""
	testRedisDB       = 15 // Use separate DB for tests
)

func setupTestRedis(t *testing.T) *RedisClient {
	// Try to connect to existing Redis instance
	client := NewRedisClient(testRedisAddr, testRedisPassword, testRedisDB)

	ctx := context.Background()
	if err := client.Client.Ping(ctx).Err(); err != nil {
		// Start Redis container if no local instance
		req := testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		}

		redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		require.NoError(t, err)

		host, err := redisContainer.Host(ctx)
		require.NoError(t, err)

		port, err := redisContainer.MappedPort(ctx, "6379")
		require.NoError(t, err)

		testRedisAddr = fmt.Sprintf("%s:%s", host, port.Port())
		client = NewRedisClient(testRedisAddr, "", 0)

		t.Cleanup(func() {
			redisContainer.Terminate(ctx)
		})
	}

	// Clean test database
	client.Client.FlushDB(ctx)

	return client
}

func setupTestConfig() *ConfigManager {
	cm := &ConfigManager{configs: make(map[string]*RateLimitConfig)}

	// Basic rate limit config
	cm.SetConfig("test:basic", &RateLimitConfig{
		Name:    "basic_test",
		Type:    LimiterTokenBucket,
		Limit:   10,
		Window:  time.Minute,
		Burst:   20,
		Enabled: true,
	})

	// Sliding window config
	cm.SetConfig("test:sliding", &RateLimitConfig{
		Name:    "sliding_test",
		Type:    LimiterSlidingWindow,
		Limit:   5,
		Window:  time.Minute,
		Enabled: true,
	})

	// Leaky bucket config
	cm.SetConfig("test:leaky", &RateLimitConfig{
		Name:    "leaky_test",
		Type:    "leaky_bucket",
		Limit:   3,
		Window:  time.Minute,
		Burst:   5,
		Enabled: true,
	})

	// IP-based config
	cm.SetConfig("ip:127.0.0.1", &RateLimitConfig{
		Name:    "ip_test",
		Type:    LimiterSlidingWindow,
		Limit:   100,
		Window:  time.Minute,
		Enabled: true,
	})

	// User tier configs
	cm.SetConfig("tier:basic:user123", &RateLimitConfig{
		Name:    "tier_basic",
		Type:    LimiterTokenBucket,
		Limit:   60,
		Window:  time.Minute,
		Burst:   120,
		Enabled: true,
	})

	return cm
}

func TestTokenBucketAlgorithm(t *testing.T) {
	redis := setupTestRedis(t)
	ctx := context.Background()

	tests := []struct {
		name            string
		capacity        int
		refillRate      float64
		requests        []int
		expectedResults []bool
		waitBetween     time.Duration
	}{
		{
			name:            "allow_within_burst",
			capacity:        10,
			refillRate:      1.0, // 1 token per second
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, true, true},
		},
		{
			name:            "deny_exceeding_burst",
			capacity:        3,
			refillRate:      1.0,
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, false, false},
		},
		{
			name:            "refill_over_time",
			capacity:        2,
			refillRate:      2.0, // 2 tokens per second
			requests:        []int{2, 2},
			expectedResults: []bool{true, true},
			waitBetween:     time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("test:token_bucket:%s", tt.name)

			for i, requestCount := range tt.requests {
				allowed, _, err := redis.TakeTokenBucket(ctx, key, tt.capacity, tt.refillRate, requestCount)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResults[i], allowed, "Request %d should be %v", i, tt.expectedResults[i])

				if tt.waitBetween > 0 && i < len(tt.requests)-1 {
					time.Sleep(tt.waitBetween)
				}
			}
		})
	}
}

func TestSlidingWindowAlgorithm(t *testing.T) {
	redis := setupTestRedis(t)
	ctx := context.Background()

	tests := []struct {
		name            string
		window          time.Duration
		limit           int
		requests        []int
		expectedResults []bool
		waitBetween     time.Duration
	}{
		{
			name:            "allow_within_limit",
			window:          time.Minute,
			limit:           5,
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, true, true},
		},
		{
			name:            "deny_exceeding_limit",
			window:          time.Minute,
			limit:           3,
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, false, false},
		},
		{
			name:            "window_reset",
			window:          time.Second,
			limit:           2,
			requests:        []int{2, 2},
			expectedResults: []bool{true, true},
			waitBetween:     1100 * time.Millisecond, // Wait for window to reset
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("test:sliding_window:%s", tt.name)

			for i, requestCount := range tt.requests {
				allowed, _, err := redis.TakeSlidingWindow(ctx, key, tt.window, tt.limit, requestCount)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResults[i], allowed, "Request %d should be %v", i, tt.expectedResults[i])

				if tt.waitBetween > 0 && i < len(tt.requests)-1 {
					time.Sleep(tt.waitBetween)
				}
			}
		})
	}
}

func TestLeakyBucketAlgorithm(t *testing.T) {
	redis := setupTestRedis(t)
	ctx := context.Background()

	tests := []struct {
		name            string
		capacity        int
		leakRate        int
		window          time.Duration
		requests        []int
		expectedResults []bool
		waitBetween     time.Duration
	}{
		{
			name:            "allow_within_capacity",
			capacity:        5,
			leakRate:        1,
			window:          time.Minute,
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, true, true},
		},
		{
			name:            "deny_exceeding_capacity",
			capacity:        3,
			leakRate:        1,
			window:          time.Minute,
			requests:        []int{1, 1, 1, 1, 1},
			expectedResults: []bool{true, true, true, false, false},
		},
		{
			name:            "leak_over_time",
			capacity:        2,
			leakRate:        2,
			window:          time.Second,
			requests:        []int{2, 2},
			expectedResults: []bool{true, true},
			waitBetween:     600 * time.Millisecond, // Wait for leak
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("test:leaky_bucket:%s", tt.name)

			for i, requestCount := range tt.requests {
				allowed, _, err := redis.TakeLeakyBucket(ctx, key, tt.capacity, tt.leakRate, tt.window, requestCount)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResults[i], allowed, "Request %d should be %v", i, tt.expectedResults[i])

				if tt.waitBetween > 0 && i < len(tt.requests)-1 {
					time.Sleep(tt.waitBetween)
				}
			}
		})
	}
}

func TestEnhancedRateLimiterMiddleware(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(redis, config, "allow")
	defer limiter.Close()

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with rate limiting
	rateLimitedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter, headers, err := limiter.Check(r.Context(), r)

		// Set headers
		for k, v := range headers {
			w.Header().Set(k, v)
		}

		if err != nil && limiter.failureMode == "deny" {
			http.Error(w, "Rate limit service unavailable", http.StatusServiceUnavailable)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		handler.ServeHTTP(w, r)
	})

	t.Run("basic_rate_limiting", func(t *testing.T) {
		// Create requests that should trigger rate limiting
		for i := 0; i < 15; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User-ID", "user123")
			req.Header.Set("X-User-Tier", string(models.TierBasic))
			req.RemoteAddr = "127.0.0.1:12345"

			w := httptest.NewRecorder()
			rateLimitedHandler.ServeHTTP(w, req)

			if i < 10 {
				assert.Equal(t, http.StatusOK, w.Code, "Request %d should be allowed", i)
			} else {
				assert.Equal(t, http.StatusTooManyRequests, w.Code, "Request %d should be rate limited", i)
			}
		}
	})
}

func TestConcurrentRateLimiting(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(redis, config, "allow")
	defer limiter.Close()

	const numGoroutines = 10
	const requestsPerGoroutine = 20

	var wg sync.WaitGroup
	allowedCount := make(chan int, numGoroutines)

	// Launch concurrent requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			allowed := 0
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-User-ID", fmt.Sprintf("user%d", goroutineID))
				req.RemoteAddr = "127.0.0.1:12345"

				isAllowed, _, _, err := limiter.Check(context.Background(), req)
				if err == nil && isAllowed {
					allowed++
				}

				time.Sleep(10 * time.Millisecond) // Small delay
			}

			allowedCount <- allowed
		}(i)
	}

	wg.Wait()
	close(allowedCount)

	// Count total allowed requests
	totalAllowed := 0
	for count := range allowedCount {
		totalAllowed += count
	}

	t.Logf("Total allowed requests: %d out of %d", totalAllowed, numGoroutines*requestsPerGoroutine)

	// Should allow some requests but enforce limits
	assert.Greater(t, totalAllowed, 0, "Should allow some requests")
	assert.Less(t, totalAllowed, numGoroutines*requestsPerGoroutine, "Should enforce rate limits")
}

func TestFailSafeMechanism(t *testing.T) {
	// Create a rate limiter with invalid Redis connection
	invalidRedis := &RedisClient{
		Client: redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		}),
	}

	config := setupTestConfig()

	t.Run("allow_on_failure", func(t *testing.T) {
		limiter := NewEnhancedRateLimiter(invalidRedis, config, "allow")
		defer limiter.Close()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-ID", "user123")
		req.RemoteAddr = "127.0.0.1:12345"

		allowed, _, _, err := limiter.Check(context.Background(), req)

		// Should allow request even with Redis failure
		assert.True(t, allowed, "Should allow requests when Redis fails and failure mode is 'allow'")
		assert.Error(t, err, "Should return an error indicating Redis failure")
	})

	t.Run("deny_on_failure", func(t *testing.T) {
		limiter := NewEnhancedRateLimiter(invalidRedis, config, "deny")
		defer limiter.Close()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-ID", "user123")
		req.RemoteAddr = "127.0.0.1:12345"

		allowed, _, _, err := limiter.Check(context.Background(), req)

		// Should deny request when Redis fails and failure mode is 'deny'
		assert.False(t, allowed, "Should deny requests when Redis fails and failure mode is 'deny'")
		assert.Error(t, err, "Should return an error indicating Redis failure")
	})
}

func TestLocalCacheFailover(t *testing.T) {
	// Test that local cache works when Redis is unavailable
	invalidRedis := &RedisClient{
		Client: redis.NewClient(&redis.Options{
			Addr: "invalid:6379",
		}),
	}

	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(invalidRedis, config, "allow")
	defer limiter.Close()

	// Make multiple requests to test local cache
	key := "test:local_cache"
	rateLimitConfig := &RateLimitConfig{
		Type:   LimiterSlidingWindow,
		Limit:  3,
		Window: time.Minute,
	}

	allowedCount := 0
	for i := 0; i < 10; i++ {
		allowed, _, _, _ := limiter.checkLocalCache(key, rateLimitConfig, "test")
		if allowed {
			allowedCount++
		}
	}

	assert.Equal(t, 3, allowedCount, "Local cache should enforce rate limit of 3")
}

func TestBruteForceProtection(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()

	// Add brute force protection config
	config.SetConfig("endpoint:/auth/login", &RateLimitConfig{
		Name:    "login_protection",
		Type:    LimiterSlidingWindow,
		Limit:   3,
		Window:  time.Minute,
		Enabled: true,
	})

	limiter := NewEnhancedRateLimiter(redis, config, "deny")
	defer limiter.Close()

	// Simulate brute force attack
	allowedAttempts := 0
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.100:12345"

		allowed, _, _, _ := limiter.Check(context.Background(), req)
		if allowed {
			allowedAttempts++
		}
	}

	assert.LessOrEqual(t, allowedAttempts, 3, "Should limit login attempts to prevent brute force")
}

func TestBurstTrafficHandling(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()

	// Configure for burst traffic
	config.SetConfig("global:burst", &RateLimitConfig{
		Name:    "burst_test",
		Type:    LimiterTokenBucket,
		Limit:   10, // 10 requests per minute
		Window:  time.Minute,
		Burst:   50, // But allow bursts of 50
		Enabled: true,
	})

	limiter := NewEnhancedRateLimiter(redis, config, "deny")
	defer limiter.Close()

	// Send burst of requests
	allowedInBurst := 0
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.Header.Set("X-User-ID", "burst_user")
		req.RemoteAddr = "127.0.0.1:12345"

		// Override key extraction for this test
		key := "global:burst"
		rateLimitConfig := config.GetConfig(key)
		allowed, _, _, _ := limiter.checkRedis(context.Background(), key, rateLimitConfig)

		if allowed {
			allowedInBurst++
		}
	}

	t.Logf("Allowed %d requests in burst", allowedInBurst)
	assert.Greater(t, allowedInBurst, 10, "Should allow more than base limit during burst")
	assert.LessOrEqual(t, allowedInBurst, 50, "Should not exceed burst limit")
}

func TestAbuseScenarios(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()

	// Configure strict limits for abuse testing
	config.SetConfig("abuse:user", &RateLimitConfig{
		Name:    "abuse_protection",
		Type:    LimiterSlidingWindow,
		Limit:   5,
		Window:  time.Minute,
		Enabled: true,
	})

	limiter := NewEnhancedRateLimiter(redis, config, "deny")
	defer limiter.Close()

	scenarios := []struct {
		name        string
		requests    int
		userAgent   string
		shouldBlock bool
	}{
		{
			name:        "normal_user",
			requests:    3,
			userAgent:   "Mozilla/5.0 (normal browser)",
			shouldBlock: false,
		},
		{
			name:        "suspicious_bot",
			requests:    20,
			userAgent:   "Bot/1.0",
			shouldBlock: true,
		},
		{
			name:        "script_kiddie",
			requests:    100,
			userAgent:   "curl/7.0",
			shouldBlock: true,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			blockedCount := 0

			for i := 0; i < scenario.requests; i++ {
				req := httptest.NewRequest("GET", "/api/sensitive", nil)
				req.Header.Set("User-Agent", scenario.userAgent)
				req.Header.Set("X-User-ID", fmt.Sprintf("%s_user", scenario.name))
				req.RemoteAddr = fmt.Sprintf("10.0.0.%d:12345", i%255)

				allowed, _, _, _ := limiter.Check(context.Background(), req)
				if !allowed {
					blockedCount++
				}
			}

			if scenario.shouldBlock {
				assert.Greater(t, blockedCount, 0, "Should block abusive requests")
			} else {
				assert.Equal(t, 0, blockedCount, "Should not block normal requests")
			}
		})
	}
}

func TestMetricsCollection(t *testing.T) {
	redis := setupTestRedis(t)
	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(redis, config, "allow")
	defer limiter.Close()

	// Make some requests to generate metrics
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-ID", "metrics_user")
		req.RemoteAddr = "127.0.0.1:12345"

		limiter.Check(context.Background(), req)
	}

	// Check that metrics are being collected
	metrics := limiter.metrics.GetKeyMetrics("user:metrics_user")
	assert.NotNil(t, metrics, "Should collect metrics for rate limited keys")
	if metrics != nil {
		assert.Greater(t, metrics.Requests, int64(0), "Should track request count")
	}
}

func BenchmarkRateLimiting(b *testing.B) {
	redis := setupTestRedis(&testing.T{})
	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(redis, config, "allow")
	defer limiter.Close()

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-ID", "bench_user")
	req.RemoteAddr = "127.0.0.1:12345"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Check(context.Background(), req)
		}
	})
}

func BenchmarkConcurrentRateLimiting(b *testing.B) {
	redis := setupTestRedis(&testing.T{})
	config := setupTestConfig()
	limiter := NewEnhancedRateLimiter(redis, config, "allow")
	defer limiter.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		userID := 0
		for pb.Next() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User-ID", fmt.Sprintf("bench_user_%d", userID%1000))
			req.RemoteAddr = "127.0.0.1:12345"

			limiter.Check(context.Background(), req)
			userID++
		}
	})
}
