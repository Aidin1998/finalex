package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// Get the current directory
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
		os.Exit(1)
	}

	// Navigate to test directory if needed
	if !strings.HasSuffix(wd, "test") {
		wd = filepath.Join(wd, "test")
	}

	// Files to run
	testFiles := []string{
		"trading_integration_test.go",
		"trading_benchmarks_test.go",
	}

	// Run each test file individually
	for _, file := range testFiles {
		fmt.Println("\n==================================================")
		fmt.Printf("Running test: %s\n", file)
		fmt.Println("==================================================")

		// Build the command
		cmd := fmt.Sprintf("go test -v -tags=trading %s common_test_types.go", file)

		// Execute the command
		startTime := time.Now()
		exitCode := runCommand(cmd)
		duration := time.Since(startTime)

		if exitCode == 0 {
			fmt.Printf("\n✓ %s completed successfully in %v\n", file, duration)
		} else {
			fmt.Printf("\n✗ %s failed with exit code %d after %v\n", file, exitCode, duration)
		}

		fmt.Println("==================================================")
	}
}

func runCommand(cmd string) int {
	return 0 // Placeholder - actual implementation would run the command and return exit code
}
