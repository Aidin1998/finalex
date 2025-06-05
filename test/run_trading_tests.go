// Run this with: go run run_trading_tests.go
//go:build runner
// +build runner

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type TestCase struct {
	File        string
	TestName    string
	Description string
	Timeout     string
}

func main() {
	// Get the absolute path to the test directory
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)

	// Change to project root directory
	os.Chdir(filepath.Join(testDir, ".."))

	fmt.Println("=== Running Trading Engine Tests ===")
	fmt.Println("These tests will run 100K orders through both heap and B-tree matching")
	fmt.Println()

	commonFile := filepath.Join(testDir, "common_test_types.go")

	// Define tests to run
	tests := []TestCase{
		{
			File:        "trading_integration_test.go",
			TestName:    "TestTradingIntegrationTestSuite/TestPerformance",
			Description: "High volume order placement through regular matching engine (heap-based)",
			Timeout:     "5m",
		},
		{
			File:        "trading_benchmarks_test.go",
			TestName:    "", // Empty means run all tests in the file
			Description: "Trading engine benchmarks",
			Timeout:     "5m",
		},
		{
			File:        "trading_performance_test.go",
			TestName:    "TestTradingPerformanceTestSuite/TestHighVolumeOrderPlacement",
			Description: "High performance order placement benchmark",
			Timeout:     "5m",
		},
		{
			File:        "trading_stress_load_test.go",
			TestName:    "TestTradingStressLoadTestSuite/TestHighVolumeOrderProcessing",
			Description: "Stress test with high volume order processing",
			Timeout:     "5m",
		},
	}

	// Run each test
	for _, test := range tests {
		fmt.Println("===================================================")
		fmt.Printf("Running: %s\n", test.Description)
		fmt.Printf("File: %s\n", test.File)
		if test.TestName != "" {
			fmt.Printf("Test: %s\n", test.TestName)
		}
		fmt.Println("===================================================")

		testFile := filepath.Join(testDir, test.File)

		// Build the command
		args := []string{"test", "-v", "-tags=trading", "-timeout=" + test.Timeout}
		if test.TestName != "" {
			args = append(args, "-run="+test.TestName)
		}
		if strings.Contains(test.File, "benchmark") {
			args = append(args, "-bench=.")
		}
		args = append(args, testFile, commonFile)

		// Execute command
		cmd := exec.Command("go", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		startTime := time.Now()
		err := cmd.Run()
		duration := time.Since(startTime)

		if err == nil {
			fmt.Printf("\n✓ Success: %s (%.2fs)\n", test.Description, duration.Seconds())
		} else {
			fmt.Printf("\n✗ Failed: %s (%.2fs)\n", test.Description, duration.Seconds())
			fmt.Printf("Error: %v\n", err)
		}
		fmt.Println()
	}

	fmt.Println("=== Testing Complete ===")
}
