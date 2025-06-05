#!/bin/bash
# Ultra-high concurrency database layer test runner for Accounts module
# Supports batch execution of all test categories with detailed reporting

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
TEST_TIMEOUT=30m
BENCHMARK_DURATION=60s
INTEGRATION_TIMEOUT=10m
UNIT_TIMEOUT=5m

echo -e "${BLUE}Ultra-High Concurrency Database Layer Test Suite${NC}"
echo -e "${BLUE}=================================================${NC}"
echo ""

# Function to print section headers
print_section() {
    echo -e "${YELLOW}$1${NC}"
    echo -e "${YELLOW}$(printf '=%.0s' $(seq 1 ${#1}))${NC}"
}

# Function to run tests with timing
run_test_category() {
    local category=$1
    local tag=$2
    local timeout=$3
    local extra_flags=$4
    
    print_section "Running $category Tests"
    echo "Build tag: $tag"
    echo "Timeout: $timeout"
    echo ""
    
    start_time=$(date +%s)
    
    if go test -v -tags="$tag" -timeout="$timeout" $extra_flags ./...; then
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo -e "${GREEN}✓ $category tests passed in ${duration}s${NC}"
    else
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo -e "${RED}✗ $category tests failed after ${duration}s${NC}"
        return 1
    fi
    echo ""
}

# Function to run specific test file
run_test_file() {
    local file=$1
    local tag=$2
    local timeout=$3
    
    print_section "Running $(basename $file)"
    echo "File: $file"
    echo "Build tag: $tag"
    echo ""
    
    if go test -v -tags="$tag" -timeout="$timeout" "$file"; then
        echo -e "${GREEN}✓ $(basename $file) passed${NC}"
    else
        echo -e "${RED}✗ $(basename $file) failed${NC}"
        return 1
    fi
    echo ""
}

# Parse command line arguments
case "${1:-all}" in
    "unit")
        echo "Running Unit Tests Only"
        echo ""
        run_test_category "Unit" "unit" "$UNIT_TIMEOUT"
        ;;
    
    "integration")
        echo "Running Integration Tests Only"
        echo ""
        run_test_category "Integration" "integration" "$INTEGRATION_TIMEOUT"
        ;;
    
    "benchmark")
        echo "Running Benchmark Tests Only"
        echo ""
        run_test_category "Benchmark" "benchmark" "$TEST_TIMEOUT" "-benchmem -run=XXX -bench=."
        ;;
    
    "models")
        echo "Running Models Tests Only"
        echo ""
        run_test_file "./accounts_models_test.go" "unit" "$UNIT_TIMEOUT"
        ;;
    
    "cache")
        echo "Running Cache Tests Only"
        echo ""
        run_test_file "./accounts_cache_test.go" "integration" "$INTEGRATION_TIMEOUT"
        ;;
    
    "repository")
        echo "Running Repository Tests Only"
        echo ""
        run_test_file "./accounts_repository_test.go" "integration" "$INTEGRATION_TIMEOUT"
        ;;
    
    "performance")
        echo "Running Performance Benchmarks Only"
        echo ""
        run_test_file "./accounts_benchmark_test.go" "benchmark" "$TEST_TIMEOUT" "-benchmem -run=XXX -bench=."
        ;;
    
    "quick")
        echo "Running Quick Test Suite (Unit + Basic Integration)"
        echo ""
        run_test_category "Unit" "unit" "$UNIT_TIMEOUT"
        echo -e "${BLUE}Running basic integration tests...${NC}"
        go test -v -tags="integration" -timeout="$INTEGRATION_TIMEOUT" -run="TestCache.*Basic|TestRepository.*Basic" ./...
        ;;
    
    "ci")
        echo "Running CI Test Suite (Unit + Integration, no benchmarks)"
        echo ""
        run_test_category "Unit" "unit" "$UNIT_TIMEOUT"
        run_test_category "Integration" "integration" "$INTEGRATION_TIMEOUT"
        ;;
    
    "all"|*)
        echo "Running Complete Test Suite"
        echo ""
        
        # Unit tests first (fastest)
        run_test_category "Unit" "unit" "$UNIT_TIMEOUT"
        
        # Integration tests
        run_test_category "Integration" "integration" "$INTEGRATION_TIMEOUT"
        
        # Benchmark tests (slowest)
        echo -e "${YELLOW}Running performance benchmarks (this may take several minutes)...${NC}"
        run_test_category "Benchmark" "benchmark" "$TEST_TIMEOUT" "-benchmem -run=XXX -bench=."
        
        echo -e "${GREEN}All test categories completed successfully!${NC}"
        ;;
esac

echo -e "${BLUE}Test execution completed.${NC}"
