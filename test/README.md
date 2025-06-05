# Testing Documentation
# Ultra-High Concurrency Database Layer Test Suite

## Overview

This test suite validates the ultra-high concurrency database layer implementation for the Accounts module, supporting 100,000+ requests per second with zero data loss. The testing framework is organized into multiple categories with proper build tags for selective execution.

## Test Categories

### 1. Unit Tests (`//go:build unit`)
**File**: `accounts_models_test.go`  
**Purpose**: Tests database models, validations, constraints, and serialization  
**Dependencies**: None (pure unit tests)  
**Execution**: `go test -tags=unit`

**Test Coverage**:
- Account model validation and constraints
- Optimistic concurrency control
- Decimal precision handling for financial data
- Model serialization and deserialization
- Constraint validation (unique indexes, foreign keys)
- Audit trail functionality

### 2. Integration Tests (`//go:build integration`)
**Files**: `accounts_cache_test.go`, `accounts_repository_test.go`  
**Purpose**: Tests system integration with Redis and PostgreSQL  
**Dependencies**: Redis, PostgreSQL  
**Execution**: `go test -tags=integration`

**Cache Tests (`accounts_cache_test.go`)**:
- Hot/warm/cold data tiering
- Cache promotion and demotion strategies
- Data invalidation and consistency
- Concurrent access patterns
- Cache miss handling
- Memory usage optimization

**Repository Tests (`accounts_repository_test.go`)**:
- Atomic operations and transactions
- Distributed locking with Redlock
- Optimistic concurrency control
- Connection pool management
- Failover and recovery scenarios
- Data consistency validation

### 3. Benchmark Tests (`//go:build benchmark`)
**File**: `accounts_benchmark_test.go`  
**Purpose**: Performance testing for 100k+ RPS validation  
**Dependencies**: Redis, PostgreSQL  
**Execution**: `go test -tags=benchmark -bench=.`

**Benchmark Coverage**:
- Balance query operations (target: 50k+ RPS)
- Balance update operations (target: 25k+ RPS)
- Mixed read/write workloads (target: 35k+ RPS)
- Sustained load testing (2+ minutes)
- Memory and CPU profiling
- Latency percentile analysis (P50, P95, P99)

## Test Execution

### Quick Start
```bash
# Run all tests
./run_tests.sh all

# Run specific category
./run_tests.sh unit
./run_tests.sh integration  
./run_tests.sh benchmark

# Run specific test file
./run_tests.sh models
./run_tests.sh cache
./run_tests.sh repository
./run_tests.sh performance
```

### Windows
```cmd
REM Run all tests
run_tests.bat all

REM Run specific category
run_tests.bat unit
run_tests.bat integration
run_tests.bat benchmark
```

### Manual Execution
```bash
# Unit tests only
go test -v -tags=unit -timeout=5m ./...

# Integration tests only  
go test -v -tags=integration -timeout=10m ./...

# Benchmark tests only
go test -v -tags=benchmark -timeout=30m -benchmem -run=XXX -bench=. ./...

# Specific test file
go test -v -tags=unit ./accounts_models_test.go
go test -v -tags=integration ./accounts_cache_test.go
go test -v -tags=integration ./accounts_repository_test.go
go test -v -tags=benchmark -bench=. ./accounts_benchmark_test.go
```

## Test Environment Setup

### Prerequisites
1. **Go 1.21+** with proper module support
2. **PostgreSQL 15+** for integration and benchmark tests
3. **Redis 7+** for caching and distributed locking tests
4. **Docker** (optional) for containerized testing

### Database Setup
```sql
-- Create test database
CREATE DATABASE accounts_test;
CREATE USER test_user WITH PASSWORD 'test_password';
GRANT ALL PRIVILEGES ON DATABASE accounts_test TO test_user;

-- In accounts_test database
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
```

### Redis Setup
```bash
# Start Redis with test configuration
redis-server --port 6379 --databases 16 --save ""
```

### Environment Variables
```bash
export POSTGRES_TEST_URL="postgres://test_user:test_password@localhost:5432/accounts_test"
export REDIS_TEST_URL="redis://localhost:6379/15"
export TEST_ENVIRONMENT="test"
```

## Test Configuration

The test suite uses `test.toml` for configuration management:

```toml
[test.unit]
timeout = "5m"
parallel = true
coverage = true

[test.integration]
timeout = "10m"
parallel = false
cleanup_timeout = "2m"

[test.benchmark]
timeout = "30m"
duration = "60s"
concurrent_users = 1000
```

## Performance Targets

### Throughput Requirements
- **Balance Queries**: 50,000+ RPS
- **Balance Updates**: 25,000+ RPS  
- **Mixed Workload**: 35,000+ RPS
- **Overall System**: 100,000+ RPS

### Latency Requirements
- **P95 Balance Query**: ≤ 5ms
- **P95 Balance Update**: ≤ 10ms
- **P95 Reservation Create**: ≤ 15ms
- **P95 Transaction Process**: ≤ 20ms

### Reliability Requirements
- **Error Rate**: ≤ 0.1%
- **Timeout Rate**: ≤ 0.05%
- **Lock Contention**: ≤ 5.0%
- **Data Loss**: 0% (zero tolerance)

## Test Data Management

### Test Data Generation
```go
// Generate test accounts
func generateTestAccounts(count int) []*accounts.Account {
    accounts := make([]*accounts.Account, count)
    for i := 0; i < count; i++ {
        accounts[i] = &accounts.Account{
            ID:       uuid.New(),
            UserID:   uuid.New(),
            Currency: "USD",
            Balance:  decimal.NewFromFloat(rand.Float64() * 10000),
        }
    }
    return accounts
}
```

### Data Cleanup
- Unit tests: No cleanup required (no external dependencies)
- Integration tests: Automatic cleanup after each test
- Benchmark tests: Cleanup after benchmark completion

## Continuous Integration

### CI Pipeline
```yaml
# Example GitHub Actions workflow
name: Ultra-High Concurrency Tests
on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
      - run: go test -tags=unit -race -coverprofile=coverage.out ./...

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test_password
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
      - run: go test -tags=integration ./...

  benchmark-tests:
    runs-on: ubuntu-latest
    if: github.event_name == 'schedule' # Run benchmarks nightly
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
      - run: go test -tags=benchmark -bench=. ./...
```

### Test Execution Modes

1. **Development Mode**: `./run_tests.sh quick`
   - Unit tests + basic integration tests
   - Fast feedback for development

2. **CI Mode**: `./run_tests.sh ci`
   - Unit + integration tests
   - No benchmarks (too slow for CI)

3. **Full Mode**: `./run_tests.sh all`
   - All test categories including benchmarks
   - Complete validation

4. **Performance Mode**: `./run_tests.sh benchmark`
   - Benchmark tests only
   - Detailed performance analysis

## Troubleshooting

### Common Issues

1. **Test Database Connection Failures**
   ```bash
   # Check PostgreSQL is running
   pg_isready -h localhost -p 5432
   
   # Verify test database exists
   psql -h localhost -U test_user -d accounts_test -c "SELECT 1;"
   ```

2. **Redis Connection Failures**
   ```bash
   # Check Redis is running
   redis-cli ping
   
   # Verify test database is accessible
   redis-cli -n 15 ping
   ```

3. **Memory Issues During Benchmarks**
   ```bash
   # Increase memory limits
   export GOGC=100
   export GOMEMLIMIT=8GiB
   
   # Monitor memory usage
   go test -tags=benchmark -bench=. -memprofile=mem.prof
   go tool pprof mem.prof
   ```

4. **Race Condition Failures**
   ```bash
   # Run with race detector
   go test -tags=integration -race ./...
   
   # Increase timeout for slow systems
   go test -tags=integration -timeout=20m ./...
   ```

### Performance Debugging

```bash
# CPU profiling
go test -tags=benchmark -bench=BenchmarkBalanceQuery -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory profiling  
go test -tags=benchmark -bench=BenchmarkBalanceUpdate -memprofile=mem.prof
go tool pprof mem.prof

# Execution tracing
go test -tags=benchmark -bench=BenchmarkMixedWorkload -trace=trace.out
go tool trace trace.out
```

## Test Metrics and Reporting

### Automated Metrics Collection
- Test execution duration
- Memory usage patterns
- CPU utilization
- Database connection pool statistics
- Redis operation latencies
- Error rates and classifications

### Performance Reporting
- Throughput measurements (RPS)
- Latency percentiles (P50, P95, P99)
- Resource utilization trends
- Bottleneck identification
- Comparison with previous runs

### Coverage Reporting
```bash
# Generate coverage report
go test -tags=unit -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Coverage threshold validation
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//'
```

This comprehensive test suite ensures the ultra-high concurrency database layer meets all performance, reliability, and scalability requirements while providing detailed feedback for development and production monitoring.
