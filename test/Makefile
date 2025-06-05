# Ultra-High Concurrency Database Layer Test Suite Makefile
# Provides convenient targets for running different test categories

.PHONY: help test test-unit test-integration test-benchmark test-all test-quick test-ci clean setup coverage

# Default target
help:
	@echo "Ultra-High Concurrency Database Layer Test Suite"
	@echo "================================================="
	@echo ""
	@echo "Available targets:"
	@echo "  help              Show this help message"
	@echo "  setup             Setup test environment and dependencies"
	@echo "  test-unit         Run unit tests only"
	@echo "  test-integration  Run integration tests only"
	@echo "  test-benchmark    Run benchmark tests only"
	@echo "  test-quick        Run quick test suite (unit + basic integration)"
	@echo "  test-ci           Run CI test suite (unit + integration, no benchmarks)"
	@echo "  test-all          Run complete test suite"
	@echo "  coverage          Generate test coverage report"
	@echo "  clean             Clean test artifacts and temporary files"
	@echo ""
	@echo "Environment variables:"
	@echo "  POSTGRES_TEST_URL    PostgreSQL connection string for tests"
	@echo "  REDIS_TEST_URL       Redis connection string for tests" 
	@echo "  TEST_TIMEOUT         Test execution timeout (default: 30m)"
	@echo "  BENCHMARK_DURATION   Benchmark test duration (default: 60s)"

# Test configuration
TEST_TIMEOUT ?= 30m
BENCHMARK_DURATION ?= 60s
UNIT_TIMEOUT ?= 5m
INTEGRATION_TIMEOUT ?= 10m

# Test directory
TEST_DIR = ./test

# Setup test environment
setup:
	@echo "Setting up test environment..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies downloaded"
	@echo "Please ensure PostgreSQL and Redis are running for integration tests"
	@echo "  PostgreSQL: $(POSTGRES_TEST_URL)"
	@echo "  Redis: $(REDIS_TEST_URL)"

# Run unit tests
test-unit:
	@echo "Running unit tests..."
	@cd $(TEST_DIR) && go test -v -tags=unit -timeout=$(UNIT_TIMEOUT) -race ./...
	@echo "✓ Unit tests completed"

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@echo "⚠️  Ensure PostgreSQL and Redis are running"
	@cd $(TEST_DIR) && go test -v -tags=integration -timeout=$(INTEGRATION_TIMEOUT) -race ./...
	@echo "✓ Integration tests completed"

# Run benchmark tests
test-benchmark:
	@echo "Running benchmark tests..."
	@echo "⚠️  This may take several minutes..."
	@cd $(TEST_DIR) && go test -v -tags=benchmark -timeout=$(TEST_TIMEOUT) -benchmem -run=XXX -bench=. ./...
	@echo "✓ Benchmark tests completed"

# Run quick test suite
test-quick:
	@echo "Running quick test suite (unit + basic integration)..."
	@$(MAKE) test-unit
	@echo "Running basic integration tests..."
	@cd $(TEST_DIR) && go test -v -tags=integration -timeout=$(INTEGRATION_TIMEOUT) -run="TestCache.*Basic|TestRepository.*Basic" ./...
	@echo "✓ Quick test suite completed"

# Run CI test suite
test-ci:
	@echo "Running CI test suite (unit + integration)..."
	@$(MAKE) test-unit
	@$(MAKE) test-integration
	@echo "✓ CI test suite completed"

# Run all tests
test-all:
	@echo "Running complete test suite..."
	@$(MAKE) test-unit
	@$(MAKE) test-integration
	@$(MAKE) test-benchmark
	@echo "✓ Complete test suite completed successfully!"

# Default test target
test: test-quick

# Generate coverage report
coverage:
	@echo "Generating test coverage report..."
	@cd $(TEST_DIR) && go test -tags=unit -coverprofile=coverage.out ./...
	@cd $(TEST_DIR) && go tool cover -html=coverage.out -o coverage.html
	@cd $(TEST_DIR) && go tool cover -func=coverage.out
	@echo "✓ Coverage report generated: $(TEST_DIR)/coverage.html"

# Performance profiling
profile-cpu:
	@echo "Running CPU profiling..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkBalanceQuery -cpuprofile=cpu.prof ./...
	@echo "✓ CPU profile generated: $(TEST_DIR)/cpu.prof"
	@echo "  View with: go tool pprof $(TEST_DIR)/cpu.prof"

profile-mem:
	@echo "Running memory profiling..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkBalanceUpdate -memprofile=mem.prof ./...
	@echo "✓ Memory profile generated: $(TEST_DIR)/mem.prof"  
	@echo "  View with: go tool pprof $(TEST_DIR)/mem.prof"

profile-trace:
	@echo "Running execution tracing..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkMixedWorkload -trace=trace.out ./...
	@echo "✓ Execution trace generated: $(TEST_DIR)/trace.out"
	@echo "  View with: go tool trace $(TEST_DIR)/trace.out"

# Clean test artifacts
clean:
	@echo "Cleaning test artifacts..."
	@cd $(TEST_DIR) && rm -f *.out *.prof *.html *.log
	@echo "✓ Test artifacts cleaned"

# Individual test files
test-models:
	@echo "Running models tests..."
	@cd $(TEST_DIR) && go test -v -tags=unit -timeout=$(UNIT_TIMEOUT) ./accounts_models_test.go

test-cache:
	@echo "Running cache tests..."
	@cd $(TEST_DIR) && go test -v -tags=integration -timeout=$(INTEGRATION_TIMEOUT) ./accounts_cache_test.go

test-repository:
	@echo "Running repository tests..."
	@cd $(TEST_DIR) && go test -v -tags=integration -timeout=$(INTEGRATION_TIMEOUT) ./accounts_repository_test.go

test-performance:
	@echo "Running performance benchmarks..."
	@cd $(TEST_DIR) && go test -v -tags=benchmark -timeout=$(TEST_TIMEOUT) -benchmem -run=XXX -bench=. ./accounts_benchmark_test.go

# Docker-based testing
docker-setup:
	@echo "Setting up Docker test environment..."
	@docker network create accounts_test_network 2>/dev/null || true
	@docker run -d --name postgres_test --network accounts_test_network \
		-e POSTGRES_PASSWORD=test_password \
		-e POSTGRES_USER=test_user \
		-e POSTGRES_DB=accounts_test \
		-p 5432:5432 postgres:15-alpine
	@docker run -d --name redis_test --network accounts_test_network \
		-p 6379:6379 redis:7-alpine
	@echo "✓ Docker containers started"
	@echo "  PostgreSQL: localhost:5432"
	@echo "  Redis: localhost:6379"

docker-cleanup:
	@echo "Cleaning up Docker test environment..."
	@docker stop postgres_test redis_test 2>/dev/null || true
	@docker rm postgres_test redis_test 2>/dev/null || true
	@docker network rm accounts_test_network 2>/dev/null || true
	@echo "✓ Docker containers cleaned up"

# Test with Docker
test-docker: docker-setup
	@echo "Running tests with Docker containers..."
	@sleep 10  # Wait for containers to be ready
	@$(MAKE) test-ci
	@$(MAKE) docker-cleanup

# Continuous testing (watch mode)
test-watch:
	@echo "Starting continuous testing (watch mode)..."
	@echo "Press Ctrl+C to stop"
	@while true; do \
		$(MAKE) test-unit; \
		echo "Waiting for changes... (press Ctrl+C to exit)"; \
		sleep 5; \
	done

# Load testing
load-test:
	@echo "Running load testing..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkSustainedLoad -timeout=10m ./...

# Stress testing  
stress-test:
	@echo "Running stress testing..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkStressTest -timeout=15m ./...

# Test specific scenarios
test-concurrency:
	@echo "Testing concurrency scenarios..."
	@cd $(TEST_DIR) && go test -tags=integration -run=".*Concurrent.*" -timeout=$(INTEGRATION_TIMEOUT) ./...

test-failover:
	@echo "Testing failover scenarios..."
	@cd $(TEST_DIR) && go test -tags=integration -run=".*Failover.*" -timeout=$(INTEGRATION_TIMEOUT) ./...

test-recovery:
	@echo "Testing recovery scenarios..."
	@cd $(TEST_DIR) && go test -tags=integration -run=".*Recovery.*" -timeout=$(INTEGRATION_TIMEOUT) ./...

# Validation targets
validate-performance:
	@echo "Validating performance requirements..."
	@cd $(TEST_DIR) && go test -tags=benchmark -bench=BenchmarkBalanceQuery -timeout=5m ./... | \
		grep "BenchmarkBalanceQuery" | awk '{if ($$3 < 50000) exit 1}'
	@echo "✓ Performance requirements validated"

validate-coverage:
	@echo "Validating test coverage..."
	@cd $(TEST_DIR) && go test -tags=unit -coverprofile=coverage.out ./... >/dev/null
	@cd $(TEST_DIR) && coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
		if [ $${coverage%.*} -lt 85 ]; then \
			echo "✗ Coverage too low: $$coverage% (minimum: 85%)"; \
			exit 1; \
		else \
			echo "✓ Coverage requirement met: $$coverage%"; \
		fi

# Release validation
test-release: clean setup test-all validate-performance validate-coverage
	@echo "✓ Release validation completed successfully!"

# Help for specific targets
help-docker:
	@echo "Docker Testing Help"
	@echo "=================="
	@echo ""
	@echo "  docker-setup      Start PostgreSQL and Redis containers"
	@echo "  docker-cleanup    Stop and remove test containers"
	@echo "  test-docker       Run full test suite with Docker"
	@echo ""
	@echo "Docker containers:"
	@echo "  PostgreSQL: postgres:15-alpine on port 5432"
	@echo "  Redis: redis:7-alpine on port 6379"

help-performance:
	@echo "Performance Testing Help"
	@echo "======================="
	@echo ""
	@echo "  test-benchmark       Run all benchmark tests"
	@echo "  test-performance     Run performance benchmarks only"
	@echo "  load-test           Run sustained load testing"
	@echo "  stress-test         Run stress testing scenarios"
	@echo "  profile-cpu         Generate CPU profile"
	@echo "  profile-mem         Generate memory profile"
	@echo "  profile-trace       Generate execution trace"
	@echo "  validate-performance Validate performance requirements"
	@echo ""
	@echo "Performance targets:"
	@echo "  Balance queries: 50,000+ RPS"
	@echo "  Balance updates: 25,000+ RPS"
	@echo "  Mixed workload: 35,000+ RPS"
