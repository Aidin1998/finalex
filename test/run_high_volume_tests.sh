#!/bin/bash
# High Volume Trading Test Runner for Linux/Mac

echo -e "\033[1;33mRunning High Volume Trading Tests\033[0m"
echo -e "\033[1;36mThese tests will run 100K orders through both heap and B-tree matching engines\033[0m"
echo ""

# Set working directory to the test directory
cd "$(dirname "$0")"
TEST_DIR=$(pwd)
COMMON_FILE="$TEST_DIR/common_test_types.go"

# Go back to project root
cd ..
PROJECT_ROOT=$(pwd)

# Define tests to run
TESTS=(
  "trading_integration_test.go TestTradingIntegrationTestSuite/TestPerformance 'High volume order placement through regular matching engine (heap-based)'"
  "trading_performance_test.go TestTradingPerformanceTestSuite/TestHighVolumeOrderPlacement 'High performance order placement benchmark'"
  "trading_stress_load_test.go TestTradingStressLoadTestSuite/TestHighVolumeOrderProcessing 'Stress test with high volume order processing'"
)

# Run each test
for test_info in "${TESTS[@]}"; do
  # Split the test info into components
  read -r file test_name description <<< "$test_info"
  
  echo -e "\n\033[1;33m==================================================\033[0m"
  echo -e "\033[1;36mRunning: $description\033[0m"
  echo -e "\033[1;36mTest: $test_name\033[0m"
  echo -e "\033[1;36mFile: $file\033[0m"
  echo -e "\033[1;33m==================================================\033[0m"
  
  # Run the test
  go test -v -tags=trading -timeout=10m -run="$test_name" "$TEST_DIR/$file" "$COMMON_FILE"
  
  # Check the result
  if [ $? -eq 0 ]; then
    echo -e "\n\033[1;32m✓ Success: $description\033[0m"
  else
    echo -e "\n\033[1;31m✗ Failed: $description\033[0m"
  fi
done

# Run benchmarks
echo -e "\n\033[1;33m==================================================\033[0m"
echo -e "\033[1;36mRunning Trading Engine Benchmarks\033[0m"
echo -e "\033[1;33m==================================================\033[0m"

go test -v -tags=trading -timeout=5m -bench=. "$TEST_DIR/trading_benchmarks_test.go" "$COMMON_FILE"

if [ $? -eq 0 ]; then
  echo -e "\n\033[1;32m✓ Success: Trading Engine Benchmarks\033[0m"
else
  echo -e "\n\033[1;31m✗ Failed: Trading Engine Benchmarks\033[0m"
fi

echo -e "\n\033[1;33m==================================================\033[0m"
echo -e "\033[1;36mTest Summary\033[0m"
echo -e "\033[1;33m==================================================\033[0m"
