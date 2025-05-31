package database

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// BenchmarkSuite provides comprehensive database performance benchmarking
type BenchmarkSuite struct {
	db      *OptimizedDatabase
	config  *BenchmarkConfig
	results *BenchmarkResults
	queries map[string]BenchmarkQuery
}

// BenchmarkConfig defines benchmark parameters
type BenchmarkConfig struct {
	Duration        time.Duration `json:"duration"`
	Concurrency     int           `json:"concurrency"`
	WarmupDuration  time.Duration `json:"warmup_duration"`
	SampleSize      int           `json:"sample_size"`
	TargetLatency   time.Duration `json:"target_latency"`
	EnableProfiling bool          `json:"enable_profiling"`
	TestDataSize    int           `json:"test_data_size"`
}

// BenchmarkQuery defines a query pattern for benchmarking
type BenchmarkQuery struct {
	Name            string        `json:"name"`
	SQL             string        `json:"sql"`
	Args            []interface{} `json:"args"`
	Weight          float64       `json:"weight"`
	Critical        bool          `json:"critical"`
	ExpectedLatency time.Duration `json:"expected_latency"`
}

// BenchmarkResults stores comprehensive benchmark results
type BenchmarkResults struct {
	StartTime       time.Time                  `json:"start_time"`
	EndTime         time.Time                  `json:"end_time"`
	Duration        time.Duration              `json:"duration"`
	TotalQueries    int64                      `json:"total_queries"`
	QueriesPerSec   float64                    `json:"queries_per_sec"`
	AvgLatency      time.Duration              `json:"avg_latency"`
	P50Latency      time.Duration              `json:"p50_latency"`
	P95Latency      time.Duration              `json:"p95_latency"`
	P99Latency      time.Duration              `json:"p99_latency"`
	MaxLatency      time.Duration              `json:"max_latency"`
	MinLatency      time.Duration              `json:"min_latency"`
	ErrorRate       float64                    `json:"error_rate"`
	CacheHitRate    float64                    `json:"cache_hit_rate"`
	TargetMet       bool                       `json:"target_met"`
	QueryResults    map[string]*QueryBenchmark `json:"query_results"`
	ResourceUsage   *ResourceUsage             `json:"resource_usage"`
	Recommendations []string                   `json:"recommendations"`
}

// QueryBenchmark stores results for individual query types
type QueryBenchmark struct {
	Name            string          `json:"name"`
	TotalExecutions int64           `json:"total_executions"`
	SuccessCount    int64           `json:"success_count"`
	ErrorCount      int64           `json:"error_count"`
	AvgLatency      time.Duration   `json:"avg_latency"`
	P95Latency      time.Duration   `json:"p95_latency"`
	P99Latency      time.Duration   `json:"p99_latency"`
	MaxLatency      time.Duration   `json:"max_latency"`
	MinLatency      time.Duration   `json:"min_latency"`
	Throughput      float64         `json:"throughput"`
	ErrorRate       float64         `json:"error_rate"`
	TargetMet       bool            `json:"target_met"`
	Latencies       []time.Duration `json:"-"` // Not serialized due to size
}

// ResourceUsage tracks system resource consumption during benchmarks
type ResourceUsage struct {
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsage     float64 `json:"memory_usage"`
	ConnectionsUsed int     `json:"connections_used"`
	CacheMemory     int64   `json:"cache_memory"`
	IOOperations    int64   `json:"io_operations"`
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(db *OptimizedDatabase, config *BenchmarkConfig) *BenchmarkSuite {
	if config == nil {
		config = &BenchmarkConfig{
			Duration:        1 * time.Minute,
			Concurrency:     50,
			WarmupDuration:  10 * time.Second,
			SampleSize:      1000,
			TargetLatency:   1 * time.Millisecond,
			EnableProfiling: true,
			TestDataSize:    10000,
		}
	}

	return &BenchmarkSuite{
		db:     db,
		config: config,
		results: &BenchmarkResults{
			QueryResults: make(map[string]*QueryBenchmark),
		},
		queries: make(map[string]BenchmarkQuery),
	}
}

// AddQuery adds a query pattern to benchmark
func (bs *BenchmarkSuite) AddQuery(query BenchmarkQuery) {
	bs.queries[query.Name] = query
}

// SetupDefaultQueries configures standard trading platform queries
func (bs *BenchmarkSuite) SetupDefaultQueries() {
	queries := []BenchmarkQuery{
		{
			Name:            "get_order_by_id",
			SQL:             "SELECT * FROM orders WHERE id = ? AND user_id = ?",
			Args:            []interface{}{1, 1},
			Weight:          0.3,
			Critical:        true,
			ExpectedLatency: 500 * time.Microsecond,
		},
		{
			Name:            "get_user_active_orders",
			SQL:             "SELECT * FROM orders WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 50",
			Args:            []interface{}{1},
			Weight:          0.2,
			Critical:        true,
			ExpectedLatency: 1 * time.Millisecond,
		},
		{
			Name:            "get_recent_trades",
			SQL:             "SELECT * FROM trades WHERE symbol = ? AND created_at > ? ORDER BY created_at DESC LIMIT 100",
			Args:            []interface{}{"BTCUSD", time.Now().Add(-1 * time.Hour)},
			Weight:          0.15,
			Critical:        true,
			ExpectedLatency: 800 * time.Microsecond,
		},
		{
			Name:            "get_user_balance",
			SQL:             "SELECT currency, balance FROM user_balances WHERE user_id = ?",
			Args:            []interface{}{1},
			Weight:          0.1,
			Critical:        true,
			ExpectedLatency: 300 * time.Microsecond,
		},
		{
			Name:            "get_market_data",
			SQL:             "SELECT symbol, price, volume FROM market_data WHERE symbol IN (?, ?, ?) AND updated_at > ?",
			Args:            []interface{}{"BTCUSD", "ETHUSD", "ADAUSD", time.Now().Add(-5 * time.Minute)},
			Weight:          0.1,
			Critical:        false,
			ExpectedLatency: 2 * time.Millisecond,
		},
		{
			Name:            "get_user_trade_history",
			SQL:             "SELECT * FROM trades WHERE user_id = ? AND created_at > ? ORDER BY created_at DESC LIMIT 200",
			Args:            []interface{}{1, time.Now().Add(-24 * time.Hour)},
			Weight:          0.08,
			Critical:        false,
			ExpectedLatency: 3 * time.Millisecond,
		},
		{
			Name:            "get_order_book",
			SQL:             "SELECT price, quantity, side FROM orders WHERE symbol = ? AND status = 'active' ORDER BY price",
			Args:            []interface{}{"BTCUSD"},
			Weight:          0.05,
			Critical:        true,
			ExpectedLatency: 1500 * time.Microsecond,
		},
		{
			Name:            "insert_new_order",
			SQL:             "INSERT INTO orders (user_id, symbol, side, type, quantity, price, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			Args:            []interface{}{1, "BTCUSD", "buy", "limit", 0.1, 50000, "active", time.Now()},
			Weight:          0.02,
			Critical:        true,
			ExpectedLatency: 2 * time.Millisecond,
		},
	}

	for _, query := range queries {
		bs.AddQuery(query)
	}
}

// RunBenchmark executes the complete benchmark suite
func (bs *BenchmarkSuite) RunBenchmark(ctx context.Context) (*BenchmarkResults, error) {
	fmt.Println("🚀 Starting Database Performance Benchmark")
	fmt.Printf("Target: < %v average latency\n", bs.config.TargetLatency)
	fmt.Printf("Duration: %v, Concurrency: %d\n", bs.config.Duration, bs.config.Concurrency)

	// Setup test data
	if err := bs.setupTestData(ctx); err != nil {
		return nil, fmt.Errorf("failed to setup test data: %w", err)
	}

	// Warmup phase
	fmt.Println("🔥 Warming up...")
	if err := bs.warmup(ctx); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}

	// Initialize results
	bs.results.StartTime = time.Now()
	for name := range bs.queries {
		bs.results.QueryResults[name] = &QueryBenchmark{
			Name:       name,
			Latencies:  make([]time.Duration, 0, bs.config.SampleSize),
			MinLatency: time.Hour, // Initialize to large value
		}
	}

	// Run benchmark
	fmt.Println("⚡ Running benchmark...")
	if err := bs.runBenchmarkLoop(ctx); err != nil {
		return nil, fmt.Errorf("benchmark failed: %w", err)
	}

	// Calculate results
	bs.calculateResults()

	// Generate recommendations
	bs.generateRecommendations()

	fmt.Println("✅ Benchmark completed!")
	bs.printResults()

	return bs.results, nil
}

func (bs *BenchmarkSuite) setupTestData(ctx context.Context) error {
	fmt.Println("📊 Setting up test data...")

	// Create test tables if they don't exist
	queries := []string{
		`CREATE TABLE IF NOT EXISTS benchmark_users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) UNIQUE,
			email VARCHAR(100),
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS benchmark_orders (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES benchmark_users(id),
			symbol VARCHAR(20),
			side VARCHAR(10),
			type VARCHAR(20),
			quantity DECIMAL(18,8),
			price DECIMAL(18,8),
			status VARCHAR(20),
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS benchmark_trades (
			id SERIAL PRIMARY KEY,
			user_id INTEGER,
			symbol VARCHAR(20),
			side VARCHAR(10),
			quantity DECIMAL(18,8),
			price DECIMAL(18,8),
			created_at TIMESTAMP DEFAULT NOW()
		)`,
	}

	// Use masterDB from OptimizedDatabase directly
	masterDB := bs.db.masterDB
	for _, query := range queries {
		if err := masterDB.WithContext(ctx).Exec(query).Error; err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Insert test data
	return bs.insertTestData(ctx)
}

func (bs *BenchmarkSuite) insertTestData(ctx context.Context) error {
	masterDB := bs.db.masterDB

	// Insert users
	for i := 1; i <= bs.config.TestDataSize/100; i++ {
		err := masterDB.WithContext(ctx).Exec(
			"INSERT INTO benchmark_users (username, email) VALUES (?, ?) ON CONFLICT (username) DO NOTHING",
			fmt.Sprintf("user%d", i),
			fmt.Sprintf("user%d@example.com", i),
		).Error
		if err != nil {
			return err
		}
	}

	// Insert orders
	for i := 1; i <= bs.config.TestDataSize; i++ {
		userID := (i % (bs.config.TestDataSize / 100)) + 1
		err := masterDB.WithContext(ctx).Exec(
			"INSERT INTO benchmark_orders (user_id, symbol, side, type, quantity, price, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
			userID, "BTCUSD", "buy", "limit", 0.1, 50000.0, "active",
		).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func (bs *BenchmarkSuite) warmup(ctx context.Context) error {
	warmupCtx, cancel := context.WithTimeout(ctx, bs.config.WarmupDuration)
	defer cancel()

	// Execute each query type a few times to warm up caches
	for _, query := range bs.queries {
		for i := 0; i < 10; i++ {
			_, _ = bs.db.ExecuteQuery(warmupCtx, query.SQL, query.Args...)
		}
	}

	return nil
}

func (bs *BenchmarkSuite) runBenchmarkLoop(ctx context.Context) error {
	benchCtx, cancel := context.WithTimeout(ctx, bs.config.Duration)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan *QueryExecution, 1000)

	// Start result collector
	go bs.collectResults(results)

	// Start worker goroutines
	for i := 0; i < bs.config.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bs.worker(benchCtx, results)
		}()
	}

	// Wait for all workers to complete
	wg.Wait()
	close(results)

	// Wait a bit for collector to finish
	time.Sleep(100 * time.Millisecond)

	return nil
}

type QueryExecution struct {
	QueryName string
	Duration  time.Duration
	Error     error
	Timestamp time.Time
}

func (bs *BenchmarkSuite) worker(ctx context.Context, results chan<- *QueryExecution) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Select random query based on weights
			query := bs.selectWeightedQuery()

			start := time.Now()
			_, err := bs.db.ExecuteQuery(ctx, query.SQL, query.Args...)
			duration := time.Since(start)

			results <- &QueryExecution{
				QueryName: query.Name,
				Duration:  duration,
				Error:     err,
				Timestamp: time.Now(),
			}
		}
	}
}

func (bs *BenchmarkSuite) selectWeightedQuery() BenchmarkQuery {
	// Simple weighted selection
	totalWeight := 0.0
	for _, query := range bs.queries {
		totalWeight += query.Weight
	}

	// Generate random value
	r := float64(time.Now().UnixNano()%1000) / 1000.0 * totalWeight

	// Select query
	current := 0.0
	for _, query := range bs.queries {
		current += query.Weight
		if r <= current {
			return query
		}
	}

	// Fallback to first query
	for _, query := range bs.queries {
		return query
	}
	panic("no queries configured")
}

func (bs *BenchmarkSuite) collectResults(results <-chan *QueryExecution) {
	for execution := range results {
		queryResult := bs.results.QueryResults[execution.QueryName]

		queryResult.TotalExecutions++
		if execution.Error != nil {
			queryResult.ErrorCount++
		} else {
			queryResult.SuccessCount++
			queryResult.Latencies = append(queryResult.Latencies, execution.Duration)

			// Update min/max
			if execution.Duration < queryResult.MinLatency {
				queryResult.MinLatency = execution.Duration
			}
			if execution.Duration > queryResult.MaxLatency {
				queryResult.MaxLatency = execution.Duration
			}
		}
	}
}

func (bs *BenchmarkSuite) calculateResults() {
	bs.results.EndTime = time.Now()
	bs.results.Duration = bs.results.EndTime.Sub(bs.results.StartTime)

	var allLatencies []time.Duration
	totalQueries := int64(0)
	totalErrors := int64(0)

	// Calculate per-query statistics
	for _, queryResult := range bs.results.QueryResults {
		if len(queryResult.Latencies) == 0 {
			continue
		}

		// Sort latencies for percentile calculations
		sort.Slice(queryResult.Latencies, func(i, j int) bool {
			return queryResult.Latencies[i] < queryResult.Latencies[j]
		})

		// Calculate percentiles
		queryResult.P95Latency = queryResult.Latencies[int(float64(len(queryResult.Latencies))*0.95)]
		queryResult.P99Latency = queryResult.Latencies[int(float64(len(queryResult.Latencies))*0.99)]

		// Calculate average
		var sum time.Duration
		for _, lat := range queryResult.Latencies {
			sum += lat
			allLatencies = append(allLatencies, lat)
		}
		queryResult.AvgLatency = sum / time.Duration(len(queryResult.Latencies))

		// Calculate throughput and error rate
		queryResult.Throughput = float64(queryResult.TotalExecutions) / bs.results.Duration.Seconds()
		queryResult.ErrorRate = float64(queryResult.ErrorCount) / float64(queryResult.TotalExecutions)

		// Check if target met
		query := bs.queries[queryResult.Name]
		queryResult.TargetMet = queryResult.AvgLatency <= query.ExpectedLatency

		totalQueries += queryResult.TotalExecutions
		totalErrors += queryResult.ErrorCount
	}

	// Calculate overall statistics
	bs.results.TotalQueries = totalQueries
	bs.results.QueriesPerSec = float64(totalQueries) / bs.results.Duration.Seconds()
	bs.results.ErrorRate = float64(totalErrors) / float64(totalQueries)

	if len(allLatencies) > 0 {
		sort.Slice(allLatencies, func(i, j int) bool {
			return allLatencies[i] < allLatencies[j]
		})

		var sum time.Duration
		for _, lat := range allLatencies {
			sum += lat
		}
		bs.results.AvgLatency = sum / time.Duration(len(allLatencies))
		bs.results.P50Latency = allLatencies[len(allLatencies)/2]
		bs.results.P95Latency = allLatencies[int(float64(len(allLatencies))*0.95)]
		bs.results.P99Latency = allLatencies[int(float64(len(allLatencies))*0.99)]
		bs.results.MaxLatency = allLatencies[len(allLatencies)-1]
		bs.results.MinLatency = allLatencies[0]

		// Check if overall target met
		bs.results.TargetMet = bs.results.AvgLatency <= bs.config.TargetLatency
	}

	// Get cache hit rate from monitoring
	if bs.db.GetMonitoring() != nil {
		metrics := bs.db.GetMonitoring().GetCurrentMetrics()
		if metrics != nil && metrics.CacheMetrics != nil {
			bs.results.CacheHitRate = metrics.CacheMetrics.HitRate
		}
	}
}

func (bs *BenchmarkSuite) generateRecommendations() {
	var recommendations []string

	// Check if target latency was met
	if !bs.results.TargetMet {
		recommendations = append(recommendations,
			fmt.Sprintf("❌ Target latency of %v not met (actual: %v)", bs.config.TargetLatency, bs.results.AvgLatency))

		// Specific recommendations based on performance
		if bs.results.AvgLatency > 5*time.Millisecond {
			recommendations = append(recommendations, "🔧 Consider adding more database indexes")
			recommendations = append(recommendations, "🔧 Implement connection pooling optimization")
		}

		if bs.results.CacheHitRate < 0.8 {
			recommendations = append(recommendations, "🔧 Increase cache TTL or size")
			recommendations = append(recommendations, "🔧 Optimize cache key strategies")
		}
	} else {
		recommendations = append(recommendations, "✅ Target latency achieved!")
	}

	// Check individual query performance
	for _, queryResult := range bs.results.QueryResults {
		if !queryResult.TargetMet {
			query := bs.queries[queryResult.Name]
			if query.Critical {
				recommendations = append(recommendations,
					fmt.Sprintf("❌ Critical query '%s' exceeds target (%v vs %v)",
						queryResult.Name, queryResult.AvgLatency, query.ExpectedLatency))
			}
		}
	}

	// Performance recommendations
	if bs.results.P99Latency > 10*bs.results.AvgLatency {
		recommendations = append(recommendations, "⚠️  High latency variance detected - investigate outliers")
	}

	if bs.results.ErrorRate > 0.01 {
		recommendations = append(recommendations, "⚠️  Error rate above 1% - check database connections")
	}

	bs.results.Recommendations = recommendations
}

func (bs *BenchmarkSuite) printResults() {
	fmt.Println("\n📈 BENCHMARK RESULTS")
	fmt.Println("==================")
	fmt.Printf("Duration: %v\n", bs.results.Duration)
	fmt.Printf("Total Queries: %d\n", bs.results.TotalQueries)
	fmt.Printf("Queries/sec: %.2f\n", bs.results.QueriesPerSec)
	fmt.Printf("Error Rate: %.2f%%\n", bs.results.ErrorRate*100)
	fmt.Printf("Cache Hit Rate: %.2f%%\n", bs.results.CacheHitRate*100)

	fmt.Println("\n⏱️  LATENCY STATISTICS")
	fmt.Printf("Average: %v\n", bs.results.AvgLatency)
	fmt.Printf("P50: %v\n", bs.results.P50Latency)
	fmt.Printf("P95: %v\n", bs.results.P95Latency)
	fmt.Printf("P99: %v\n", bs.results.P99Latency)
	fmt.Printf("Max: %v\n", bs.results.MaxLatency)
	fmt.Printf("Min: %v\n", bs.results.MinLatency)

	fmt.Printf("\n🎯 TARGET MET: %v (Target: %v, Actual: %v)\n",
		bs.results.TargetMet, bs.config.TargetLatency, bs.results.AvgLatency)

	fmt.Println("\n📊 QUERY BREAKDOWN")
	for name, result := range bs.results.QueryResults {
		status := "✅"
		if !result.TargetMet {
			status = "❌"
		}
		fmt.Printf("%s %s: %v avg (P95: %v, count: %d)\n",
			status, name, result.AvgLatency, result.P95Latency, result.TotalExecutions)
	}

	fmt.Println("\n💡 RECOMMENDATIONS")
	for _, rec := range bs.results.Recommendations {
		fmt.Printf("   %s\n", rec)
	}
}
