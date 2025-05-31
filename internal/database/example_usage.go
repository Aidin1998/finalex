package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pincex_unified/internal/database"
)

func main() {
	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\n🛑 Received shutdown signal")
		cancel()
	}()

	fmt.Println("🚀 Starting Database Optimization Demo")
	fmt.Println("====================================")

	// Initialize optimized database with production config
	config := database.DefaultConfig()
	
	// Customize for demo environment
	config.Cache.Redis.Addr = "localhost:6379"
	config.Monitoring.AlertThresholds.MaxQueryLatency = 1 * time.Millisecond
	config.QueryOptimizer.SlowQueryThreshold = 100 * time.Microsecond
	
	fmt.Println("📊 Initializing optimized database system...")
	
	optimizedDB, err := database.NewOptimizedDatabase(config)
	if err != nil {
		log.Fatalf("Failed to create optimized database: %v", err)
	}

	// Start the system
	fmt.Println("⚡ Starting database optimization components...")
	if err := optimizedDB.Start(); err != nil {
		log.Fatalf("Failed to start optimized database: %v", err)
	}
	defer optimizedDB.Stop()

	// Wait for system to initialize
	time.Sleep(2 * time.Second)

	// Check health status
	fmt.Println("\n🔍 Checking system health...")
	healthStatus := optimizedDB.GetHealthStatus()
	printHealthStatus(healthStatus)

	// Run performance benchmark
	fmt.Println("\n🏁 Running Performance Benchmark")
	if err := runBenchmark(ctx, optimizedDB); err != nil {
		log.Printf("Benchmark failed: %v", err)
	}

	// Demonstrate query optimization features
	fmt.Println("\n🔧 Demonstrating Query Optimization Features")
	demonstrateOptimizations(ctx, optimizedDB)

	// Show monitoring dashboard
	fmt.Println("\n📈 Monitoring Dashboard")
	showMonitoringDashboard(optimizedDB)

	// Show cache performance
	fmt.Println("\n💾 Cache Performance")
	showCachePerformance(ctx, optimizedDB)

	// Wait for user input or signal
	fmt.Println("\n✅ Demo complete! Press Ctrl+C to exit...")
	<-ctx.Done()
	
	fmt.Println("\n👋 Shutting down gracefully...")
}

func printHealthStatus(status map[string]interface{}) {
	fmt.Println("Health Status:")
	for component, details := range status {
		fmt.Printf("  %s: %+v\n", component, details)
	}
}

func runBenchmark(ctx context.Context, db *database.OptimizedDatabase) error {
	// Create benchmark suite
	benchConfig := &database.BenchmarkConfig{
		Duration:       30 * time.Second,
		Concurrency:    20,
		WarmupDuration: 5 * time.Second,
		TargetLatency:  1 * time.Millisecond,
		TestDataSize:   1000,
	}

	suite := database.NewBenchmarkSuite(db, benchConfig)
	suite.SetupDefaultQueries()

	// Run benchmark
	results, err := suite.RunBenchmark(ctx)
	if err != nil {
		return err
	}

	// Print summary
	fmt.Printf("\n🎯 BENCHMARK SUMMARY\n")
	fmt.Printf("Target Achieved: %v\n", results.TargetMet)
	fmt.Printf("Average Latency: %v\n", results.AvgLatency)
	fmt.Printf("Throughput: %.2f queries/sec\n", results.QueriesPerSec)
	fmt.Printf("Cache Hit Rate: %.2f%%\n", results.CacheHitRate*100)

	return nil
}

func demonstrateOptimizations(ctx context.Context, db *database.OptimizedDatabase) {
	// Test query caching
	fmt.Println("1. Testing Query Caching")
	testQuery := "SELECT COUNT(*) FROM orders WHERE status = 'active'"
	
	// First execution (cache miss)
	start := time.Now()
	result1, err := db.ExecuteQuery(ctx, testQuery)
	duration1 := time.Since(start)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Printf("   First execution (cache miss): %v\n", duration1)
	}

	// Second execution (cache hit)
	start = time.Now()
	result2, err := db.ExecuteQuery(ctx, testQuery)
	duration2 := time.Since(start)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Printf("   Second execution (cache hit): %v\n", duration2)
		if duration2 < duration1 {
			fmt.Printf("   ✅ Cache speedup: %.2fx faster\n", float64(duration1)/float64(duration2))
		}
	}

	// Test read replica routing
	fmt.Println("\n2. Testing Read Replica Routing")
	router := db.GetQueryRouter()
	if router != nil {
		readQuery := "SELECT * FROM orders LIMIT 10"
		writeQuery := "INSERT INTO orders (user_id, symbol, status) VALUES (1, 'TEST', 'test')"
		
		fmt.Printf("   Read query routing: %s\n", getQueryDestination(readQuery))
		fmt.Printf("   Write query routing: %s\n", getQueryDestination(writeQuery))
	}

	// Test enhanced repository features
	fmt.Println("\n3. Testing Enhanced Repository")
	repo := db.GetRepository()
	if repo != nil {
		// Test cached order retrieval
		start = time.Now()
		orders, err := repo.GetUserOrders(ctx, 1, 10)
		duration := time.Since(start)
		if err != nil {
			fmt.Printf("   Error getting orders: %v\n", err)
		} else {
			fmt.Printf("   Retrieved %d orders in %v\n", len(orders), duration)
		}
	}
}

func getQueryDestination(query string) string {
	// Simple pattern matching for demo
	if containsAny(query, []string{"INSERT", "UPDATE", "DELETE"}) {
		return "Master Database"
	}
	return "Read Replica"
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) && s[:len(sub)] == sub {
			return true
		}
	}
	return false
}

func showMonitoringDashboard(db *database.OptimizedDatabase) {
	monitoring := db.GetMonitoring()
	if monitoring == nil {
		fmt.Println("   Monitoring not available")
		return
	}

	metrics := monitoring.GetCurrentMetrics()
	
	// Display key metrics
	if dbMetrics, ok := metrics["database"].(map[string]interface{}); ok {
		fmt.Printf("   Active Connections: %v\n", dbMetrics["active_connections"])
		fmt.Printf("   Query Count: %v\n", dbMetrics["query_count"])
	}

	if queryMetrics, ok := metrics["query"].(map[string]interface{}); ok {
		fmt.Printf("   Average Query Time: %v\n", queryMetrics["avg_latency"])
		fmt.Printf("   Slow Queries: %v\n", queryMetrics["slow_queries"])
	}

	if cacheMetrics, ok := metrics["cache"].(map[string]interface{}); ok {
		fmt.Printf("   Cache Hit Rate: %.2f%%\n", cacheMetrics["hit_rate"].(float64)*100)
		fmt.Printf("   Cache Memory Usage: %v MB\n", cacheMetrics["memory_usage"])
	}
}

func showCachePerformance(ctx context.Context, db *database.OptimizedDatabase) {
	// Demonstrate cache performance with repeated queries
	queries := []string{
		"SELECT COUNT(*) FROM orders",
		"SELECT COUNT(*) FROM trades",
		"SELECT COUNT(*) FROM users",
	}

	for i, query := range queries {
		fmt.Printf("   Query %d: %s\n", i+1, query)
		
		// Execute multiple times to show cache effect
		var durations []time.Duration
		for j := 0; j < 3; j++ {
			start := time.Now()
			_, err := db.ExecuteQuery(ctx, query)
			duration := time.Since(start)
			durations = append(durations, duration)
			
			if err != nil {
				fmt.Printf("     Execution %d: Error - %v\n", j+1, err)
			} else {
				fmt.Printf("     Execution %d: %v\n", j+1, duration)
			}
		}
		
		// Calculate speedup
		if len(durations) >= 2 && durations[1] < durations[0] {
			speedup := float64(durations[0]) / float64(durations[1])
			fmt.Printf("     Cache speedup: %.2fx\n", speedup)
		}
		fmt.Println()
	}
}

// Example production usage patterns
func ExampleProductionUsage() {
	// Production configuration example
	config := database.DefaultConfig()
	
	// Production tuning
	config.Master.MaxOpenConns = 100
	config.Replica.MaxOpenConns = 200
	config.Cache.Redis.PoolSize = 200
	config.QueryOptimizer.SlowQueryThreshold = 50 * time.Millisecond
	config.Monitoring.AlertThresholds.MaxQueryLatency = 500 * time.Microsecond

	// Initialize system
	db, err := database.NewOptimizedDatabase(config)
	if err != nil {
		log.Fatal(err)
	}

	// Start system
	if err := db.Start(); err != nil {
		log.Fatal(err)
	}
	defer db.Stop()

	// Use the optimized database
	ctx := context.Background()
	
	// High-performance order retrieval
	repo := db.GetRepository()
	orders, err := repo.GetUserOrders(ctx, 12345, 50)
	if err != nil {
		log.Printf("Failed to get orders: %v", err)
	} else {
		fmt.Printf("Retrieved %d orders\n", len(orders))
	}

	// Direct optimized query execution
	result, err := db.ExecuteQuery(ctx, 
		"SELECT * FROM trades WHERE user_id = ? AND created_at > ? ORDER BY created_at DESC LIMIT 100",
		12345, time.Now().Add(-24*time.Hour))
	if err != nil {
		log.Printf("Query failed: %v", err)
	} else {
		fmt.Printf("Query executed in %v\n", result.ExecutionTime)
	}

	// Monitor performance
	monitoring := db.GetMonitoring()
	metrics := monitoring.GetCurrentMetrics()
	fmt.Printf("Current performance metrics: %+v\n", metrics)
}

// Example benchmark for continuous integration
func ExampleCIBenchmark() {
	config := database.DefaultConfig()
	db, _ := database.NewOptimizedDatabase(config)
	db.Start()
	defer db.Stop()

	// Quick CI benchmark
	benchConfig := &database.BenchmarkConfig{
		Duration:      10 * time.Second,
		Concurrency:   10,
		TargetLatency: 1 * time.Millisecond,
		TestDataSize:  100,
	}

	suite := database.NewBenchmarkSuite(db, benchConfig)
	suite.SetupDefaultQueries()

	ctx := context.Background()
	results, err := suite.RunBenchmark(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Fail CI if target not met
	if !results.TargetMet {
		log.Fatalf("Performance regression detected: %v > %v", 
			results.AvgLatency, benchConfig.TargetLatency)
	}

	fmt.Println("✅ Performance target achieved")
}
