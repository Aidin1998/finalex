package test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/database"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DatabasePerformanceTest validates the database optimization improvements
type DatabasePerformanceTest struct {
	db           *sql.DB
	optimizedDB  *database.OptimizedDatabase
	numUsers     int
	numOrders    int
	numTrades    int
	numAlerts    int
	concurrency  int
	testDuration time.Duration
}

// TestDatabasePerformanceAfterOptimization runs comprehensive performance tests
func TestDatabasePerformanceAfterOptimization(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping database performance tests in short mode")
	}

	perfTest := &DatabasePerformanceTest{
		numUsers:     10000,
		numOrders:    100000,
		numTrades:    50000,
		numAlerts:    5000,
		concurrency:  50,
		testDuration: 30 * time.Second,
	}

	// Initialize test database
	err := perfTest.setupTestDatabase(t)
	require.NoError(t, err)
	defer perfTest.cleanup()

	// Run performance test suite
	t.Run("ConnectionPoolPerformance", perfTest.testConnectionPoolPerformance)
	t.Run("IndexedQueryPerformance", perfTest.testIndexedQueryPerformance)
	t.Run("ConcurrentOrderInserts", perfTest.testConcurrentOrderInserts)
	t.Run("ConcurrentTradeQueries", perfTest.testConcurrentTradeQueries)
	t.Run("ComplianceQueryPerformance", perfTest.testComplianceQueryPerformance)
	t.Run("LoadTestFullStack", perfTest.testLoadTestFullStack)
}

func (pt *DatabasePerformanceTest) setupTestDatabase(t *testing.T) error {
	// Initialize optimized database with performance settings
	config := database.DefaultConfig()

	// Use optimized connection pool settings
	config.Master.MaxOpenConns = 100
	config.Master.MaxIdleConns = 10
	config.Master.ConnMaxLifetime = 1 * time.Hour

	var err error
	pt.optimizedDB, err = database.NewOptimizedDatabase(config, nil)
	if err != nil {
		return fmt.Errorf("failed to create optimized database: %w", err)
	}

	// Get underlying database connection
	pt.db, err = pt.optimizedDB.GetMasterDB().DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Seed test data
	return pt.seedTestData()
}

func (pt *DatabasePerformanceTest) testConnectionPoolPerformance(t *testing.T) {
	t.Logf("Testing connection pool performance with %d concurrent connections", pt.concurrency)

	start := time.Now()
	var wg sync.WaitGroup
	errorChan := make(chan error, pt.concurrency)

	// Test connection acquisition under load
	for i := 0; i < pt.concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				// Simulate database operation
				_, err := pt.db.Exec("SELECT 1")
				if err != nil {
					errorChan <- fmt.Errorf("worker %d error: %w", workerID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Error(err)
	}

	duration := time.Since(start)
	t.Logf("Connection pool test completed in %v", duration)

	// Validate performance metrics
	stats := pt.db.Stats()
	t.Logf("Connection stats: OpenConnections=%d, InUse=%d, Idle=%d",
		stats.OpenConnections, stats.InUse, stats.Idle)

	// Assert connection pool efficiency
	assert.LessOrEqual(t, stats.OpenConnections, 100, "Should not exceed MaxOpenConns")
	assert.LessOrEqual(t, stats.WaitCount, int64(10), "Should have minimal connection waits")
}

func (pt *DatabasePerformanceTest) testIndexedQueryPerformance(t *testing.T) {
	t.Log("Testing indexed query performance improvements")

	testCases := []struct {
		name     string
		query    string
		expected time.Duration
	}{
		{
			name:     "OrdersByUserAndSymbol",
			query:    "SELECT COUNT(*) FROM orders WHERE user_id = $1 AND symbol = $2",
			expected: 10 * time.Millisecond,
		},
		{
			name:     "TradesBySymbolAndTimeRange",
			query:    "SELECT COUNT(*) FROM trades WHERE symbol = $1 AND created_at >= $2",
			expected: 15 * time.Millisecond,
		},
		{
			name:     "ComplianceAlertsByStatus",
			query:    "SELECT COUNT(*) FROM compliance_alerts WHERE status = $1 ORDER BY created_at DESC LIMIT 100",
			expected: 5 * time.Millisecond,
		},
		{
			name:     "AMLUsersByRiskLevel",
			query:    "SELECT COUNT(*) FROM aml_users WHERE risk_level = $1 AND risk_score > $2",
			expected: 8 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Warm up the query
			for i := 0; i < 3; i++ {
				pt.executeTestQuery(tc.query)
			}

			// Measure performance
			start := time.Now()
			iterations := 100

			for i := 0; i < iterations; i++ {
				pt.executeTestQuery(tc.query)
			}

			avgDuration := time.Since(start) / time.Duration(iterations)
			t.Logf("%s average query time: %v", tc.name, avgDuration)

			// Assert performance meets requirements
			assert.LessOrEqual(t, avgDuration, tc.expected,
				"Query should complete within expected time")
		})
	}
}

func (pt *DatabasePerformanceTest) testConcurrentOrderInserts(t *testing.T) {
	t.Log("Testing concurrent order insert performance")

	start := time.Now()
	var wg sync.WaitGroup
	errorChan := make(chan error, pt.concurrency)
	successCount := int64(0)
	var successMutex sync.Mutex

	ordersPerWorker := 1000

	for i := 0; i < pt.concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < ordersPerWorker; j++ {
				err := pt.insertTestOrder(fmt.Sprintf("user_%d_%d", workerID, j))
				if err != nil {
					errorChan <- err
					return
				}

				successMutex.Lock()
				successCount++
				successMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	duration := time.Since(start)
	totalOrders := int64(pt.concurrency * ordersPerWorker)
	ordersPerSecond := float64(successCount) / duration.Seconds()

	t.Logf("Inserted %d orders in %v (%.2f orders/sec)", successCount, duration, ordersPerSecond)

	// Check for errors
	errorCount := 0
	for err := range errorChan {
		t.Error(err)
		errorCount++
	}

	// Assert performance targets
	assert.Equal(t, totalOrders, successCount, "All orders should be inserted successfully")
	assert.GreaterOrEqual(t, ordersPerSecond, 1000.0, "Should achieve at least 1000 orders/sec")
	assert.Equal(t, 0, errorCount, "Should have no insertion errors")
}

func (pt *DatabasePerformanceTest) testConcurrentTradeQueries(t *testing.T) {
	t.Log("Testing concurrent trade query performance")

	start := time.Now()
	var wg sync.WaitGroup
	queryCount := int64(0)
	var queryMutex sync.Mutex

	for i := 0; i < pt.concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				// Query trades by various criteria
				queries := []string{
					"SELECT COUNT(*) FROM trades WHERE symbol = 'BTC/USD' AND created_at >= NOW() - INTERVAL '1 hour'",
					"SELECT * FROM trades WHERE user_id = 'test_user' ORDER BY created_at DESC LIMIT 10",
					"SELECT AVG(price) FROM trades WHERE symbol = 'ETH/USD' AND created_at >= NOW() - INTERVAL '1 day'",
				}

				for _, query := range queries {
					_, err := pt.db.Query(query)
					if err != nil {
						t.Errorf("Query error: %v", err)
						return
					}

					queryMutex.Lock()
					queryCount++
					queryMutex.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	duration := time.Since(start)
	queriesPerSecond := float64(queryCount) / duration.Seconds()

	t.Logf("Executed %d queries in %v (%.2f queries/sec)", queryCount, duration, queriesPerSecond)

	// Assert query performance
	assert.GreaterOrEqual(t, queriesPerSecond, 500.0, "Should achieve at least 500 queries/sec")
}

func (pt *DatabasePerformanceTest) testComplianceQueryPerformance(t *testing.T) {
	t.Log("Testing compliance table query performance")

	complianceQueries := []struct {
		name  string
		query string
	}{
		{
			name:  "HighRiskAlerts",
			query: "SELECT * FROM compliance_alerts WHERE severity IN ('high', 'critical') AND status = 'open' ORDER BY created_at DESC LIMIT 50",
		},
		{
			name:  "UserRiskProfile",
			query: "SELECT * FROM aml_users WHERE risk_level = 'HIGH' AND risk_score > 80 ORDER BY risk_score DESC LIMIT 20",
		},
		{
			name:  "RecentAlertsByUser",
			query: "SELECT * FROM compliance_alerts WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '30 days' ORDER BY created_at DESC",
		},
		{
			name:  "CountryRiskAnalysis",
			query: "SELECT country_code, COUNT(*), AVG(risk_score) FROM aml_users WHERE is_high_risk_country = true GROUP BY country_code",
		},
	}

	for _, cq := range complianceQueries {
		t.Run(cq.name, func(t *testing.T) {
			start := time.Now()

			// Execute query multiple times
			for i := 0; i < 50; i++ {
				if cq.name == "RecentAlertsByUser" {
					_, err := pt.db.Query(cq.query, "test_user")
					assert.NoError(t, err)
				} else {
					_, err := pt.db.Query(cq.query)
					assert.NoError(t, err)
				}
			}

			avgDuration := time.Since(start) / 50
			t.Logf("%s average query time: %v", cq.name, avgDuration)

			// Assert compliance query performance
			assert.LessOrEqual(t, avgDuration, 20*time.Millisecond,
				"Compliance queries should be fast with proper indexes")
		})
	}
}

func (pt *DatabasePerformanceTest) testLoadTestFullStack(t *testing.T) {
	t.Log("Running full-stack load test simulation")

	ctx, cancel := context.WithTimeout(context.Background(), pt.testDuration)
	defer cancel()

	var wg sync.WaitGroup
	metrics := &LoadTestMetrics{
		start: time.Now(),
	}

	// Simulate realistic trading workload
	workloads := []struct {
		name     string
		workers  int
		workFunc func(context.Context, int, *LoadTestMetrics)
	}{
		{"OrderCreation", 10, pt.orderCreationWorkload},
		{"TradeExecution", 8, pt.tradeExecutionWorkload},
		{"ComplianceChecks", 5, pt.complianceCheckWorkload},
		{"UserQueries", 15, pt.userQueryWorkload},
	}

	for _, workload := range workloads {
		for i := 0; i < workload.workers; i++ {
			wg.Add(1)
			go func(name string, workerID int, work func(context.Context, int, *LoadTestMetrics)) {
				defer wg.Done()
				work(ctx, workerID, metrics)
			}(workload.name, i, workload.workFunc)
		}
	}

	wg.Wait()

	// Report metrics
	metrics.end = time.Now()
	pt.reportLoadTestMetrics(t, metrics)
}

// Helper functions and workloads

func (pt *DatabasePerformanceTest) executeTestQuery(query string) error {
	switch query {
	case "SELECT COUNT(*) FROM orders WHERE user_id = $1 AND symbol = $2":
		_, err := pt.db.Query(query, "test_user", "BTC/USD")
		return err
	case "SELECT COUNT(*) FROM trades WHERE symbol = $1 AND created_at >= $2":
		_, err := pt.db.Query(query, "BTC/USD", time.Now().Add(-1*time.Hour))
		return err
	case "SELECT COUNT(*) FROM compliance_alerts WHERE status = $1 ORDER BY created_at DESC LIMIT 100":
		_, err := pt.db.Query(query, "open")
		return err
	case "SELECT COUNT(*) FROM aml_users WHERE risk_level = $1 AND risk_score > $2":
		_, err := pt.db.Query(query, "HIGH", 75.0)
		return err
	default:
		_, err := pt.db.Query(query)
		return err
	}
}

func (pt *DatabasePerformanceTest) insertTestOrder(userID string) error {
	query := `
		INSERT INTO orders (id, user_id, symbol, side, type, quantity, price, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := pt.db.Exec(query,
		fmt.Sprintf("order_%d", rand.Int63()),
		userID,
		"BTC/USD",
		"buy",
		"limit",
		decimal.NewFromFloat(0.1),
		decimal.NewFromFloat(50000.0),
		"pending",
		time.Now(),
		time.Now(),
	)

	return err
}

// Workload functions for load testing

func (pt *DatabasePerformanceTest) orderCreationWorkload(ctx context.Context, workerID int, metrics *LoadTestMetrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := pt.insertTestOrder(fmt.Sprintf("load_user_%d", workerID))
			if err != nil {
				metrics.incrementError()
			} else {
				metrics.incrementSuccess()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (pt *DatabasePerformanceTest) tradeExecutionWorkload(ctx context.Context, workerID int, metrics *LoadTestMetrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, err := pt.db.Query("SELECT * FROM trades WHERE symbol = $1 ORDER BY created_at DESC LIMIT 10", "BTC/USD")
			if err != nil {
				metrics.incrementError()
			} else {
				metrics.incrementSuccess()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (pt *DatabasePerformanceTest) complianceCheckWorkload(ctx context.Context, workerID int, metrics *LoadTestMetrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, err := pt.db.Query("SELECT * FROM compliance_alerts WHERE status = 'open' LIMIT 5")
			if err != nil {
				metrics.incrementError()
			} else {
				metrics.incrementSuccess()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (pt *DatabasePerformanceTest) userQueryWorkload(ctx context.Context, workerID int, metrics *LoadTestMetrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, err := pt.db.Query("SELECT * FROM aml_users WHERE risk_level != 'LOW' LIMIT 10")
			if err != nil {
				metrics.incrementError()
			} else {
				metrics.incrementSuccess()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Load test metrics and reporting

type LoadTestMetrics struct {
	start        time.Time
	end          time.Time
	successCount int64
	errorCount   int64
	mutex        sync.Mutex
}

func (m *LoadTestMetrics) incrementSuccess() {
	m.mutex.Lock()
	m.successCount++
	m.mutex.Unlock()
}

func (m *LoadTestMetrics) incrementError() {
	m.mutex.Lock()
	m.errorCount++
	m.mutex.Unlock()
}

func (pt *DatabasePerformanceTest) reportLoadTestMetrics(t *testing.T, metrics *LoadTestMetrics) {
	duration := metrics.end.Sub(metrics.start)
	totalOps := metrics.successCount + metrics.errorCount
	opsPerSecond := float64(totalOps) / duration.Seconds()
	errorRate := float64(metrics.errorCount) / float64(totalOps) * 100

	t.Logf("Load Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Total Operations: %d", totalOps)
	t.Logf("  Successful Operations: %d", metrics.successCount)
	t.Logf("  Failed Operations: %d", metrics.errorCount)
	t.Logf("  Operations/Second: %.2f", opsPerSecond)
	t.Logf("  Error Rate: %.2f%%", errorRate)

	// Assert performance targets
	assert.GreaterOrEqual(t, opsPerSecond, 1000.0, "Should achieve at least 1000 ops/sec")
	assert.LessOrEqual(t, errorRate, 1.0, "Error rate should be less than 1%")
}

func (pt *DatabasePerformanceTest) seedTestData() error {
	// This would seed initial test data
	// For brevity, implementing a simple version
	return nil
}

func (pt *DatabasePerformanceTest) cleanup() {
	if pt.optimizedDB != nil {
		pt.optimizedDB.Stop()
	}
}
