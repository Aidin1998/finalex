package main

import (
	"fmt"
	"os"
	"os/exec"
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
	// Execute the command using PowerShell
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		fmt.Println("Error: empty command")
		return 1
	}

	// Create the command
	execCmd := exec.Command(parts[0], parts[1:]...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Dir = "."

	// Run the command
	if err := execCmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode()
		}
		fmt.Printf("Error running command: %v\n", err)
		return 1
	}

	return 0
}
