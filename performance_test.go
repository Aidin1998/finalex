// Standalone performance test for accounts module cleanup validation
package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// Define a simplified Account struct for testing
type Account struct {
	UserID             uint64    `json:"user_id"`
	Currency           string    `json:"currency"`
	Balance            string    `json:"balance"`
	AvailableBalance   string    `json:"available_balance"`
	LockedBalance      string    `json:"locked_balance"`
	Version            int64     `json:"version"`
	ConcurrencyVersion int64     `json:"concurrency_version"`
	PartitionKey       string    `json:"partition_key"`
	LastBalanceUpdate  time.Time `json:"last_balance_update"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (a *Account) GetTotalBalance() string {
	return a.Balance
}

func (a *Account) GetPartitionKey() string {
	return a.PartitionKey
}

func main() {
	fmt.Println("=== Accounts Module Performance Test ===")
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("NumCPU: %d\n", runtime.NumCPU())
	fmt.Println()

	// Test model creation performance
	testModelCreation()

	// Test concurrent operations
	testConcurrentOperations()

	// Memory usage test
	testMemoryUsage()

	fmt.Println("=== Performance Test Complete ===")
}

func testModelCreation() {
	fmt.Println("--- Model Creation Performance ---")

	start := time.Now()
	const iterations = 100000

	for i := 0; i < iterations; i++ {
		account := Account{
			UserID:             uint64(i),
			Currency:           "USD",
			Balance:            "1000.00",
			AvailableBalance:   "1000.00",
			LockedBalance:      "0.00",
			Version:            1,
			ConcurrencyVersion: 1,
			PartitionKey:       fmt.Sprintf("part_%d", i%1000),
			LastBalanceUpdate:  time.Now(),
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		_ = account // Use the account to prevent optimization
	}

	duration := time.Since(start)
	fmt.Printf("Created %d accounts in %v\n", iterations, duration)
	fmt.Printf("Rate: %.2f accounts/second\n", float64(iterations)/duration.Seconds())
	fmt.Println()
}

func testConcurrentOperations() {
	fmt.Println("--- Concurrent Operations Performance ---")

	const goroutines = 1000
	const operationsPerGoroutine = 1000

	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Simulate account operations
				account := Account{
					UserID:             uint64(id*operationsPerGoroutine + j),
					Currency:           "USD",
					Balance:            "1000.00",
					AvailableBalance:   "1000.00",
					LockedBalance:      "0.00",
					Version:            1,
					ConcurrencyVersion: 1,
					PartitionKey:       fmt.Sprintf("part_%d", (id*operationsPerGoroutine+j)%1000),
					LastBalanceUpdate:  time.Now(),
					CreatedAt:          time.Now(),
					UpdatedAt:          time.Now(),
				}

				// Simulate balance calculation
				_ = account.GetTotalBalance()
				_ = account.GetPartitionKey()
			}
		}(id)
	}

	wg.Wait()
	duration := time.Since(start)
	totalOps := goroutines * operationsPerGoroutine

	fmt.Printf("Completed %d operations with %d goroutines in %v\n", totalOps, goroutines, duration)
	fmt.Printf("Rate: %.2f operations/second\n", float64(totalOps)/duration.Seconds())
	fmt.Printf("Average: %.2f µs/operation\n", float64(duration.Nanoseconds())/float64(totalOps)/1000)
	fmt.Println()
}

func testMemoryUsage() {
	fmt.Println("--- Memory Usage Test ---")

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create many accounts to test memory usage
	accounts := make([]Account, 100000)
	for i := range accounts {
		accounts[i] = Account{
			UserID:             uint64(i),
			Currency:           "USD",
			Balance:            "1000.00",
			AvailableBalance:   "1000.00",
			LockedBalance:      "0.00",
			Version:            1,
			ConcurrencyVersion: 1,
			PartitionKey:       fmt.Sprintf("part_%d", i%1000),
			LastBalanceUpdate:  time.Now(),
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	fmt.Printf("Memory allocated: %d KB\n", (m2.Alloc-m1.Alloc)/1024)
	fmt.Printf("Memory per account: %d bytes\n", (m2.Alloc-m1.Alloc)/100000)
	fmt.Printf("Total allocations: %d\n", m2.TotalAlloc-m1.TotalAlloc)
	fmt.Println()

	// Keep reference to prevent GC
	_ = accounts
}
