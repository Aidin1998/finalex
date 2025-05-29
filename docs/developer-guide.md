# Developer Guide

## Onboarding

1. Clone the repository:
   ```powershell
   git clone https://github.com/Aidin1998/pincex_unified.git
   cd pincex_unified
   ```
2. Copy `.env.example` to `.env` and set your environment variables:
   ```env
   PORT=8080
   DB_URL=postgres://user:pass@localhost:5432/pincex
   REDIS_URL=redis://localhost:6379/0
   LOG_LEVEL=debug
   ```
3. Install dependencies:
   ```powershell
   go mod download
   ```
4. Start local services:
   ```powershell
   docker-compose up -d db redis
   ```
5. Run tests:
   ```powershell
   go test ./...  
   ```
6. Build and run:
   ```powershell
   go build -o pincex.exe ./cmd/pincex
   .\pincex.exe
   ```
7. Run performance benchmarks:
   ```powershell
   go test -tags=performance -bench=BenchmarkOrderPlacementThroughput ./test/
   ```
   - For profiling (CPU, memory, mutex):
     ```powershell
     go test -tags=performance -bench=BenchmarkOrderPlacementThroughput -cpuprofile=cpu.out -memprofile=mem.out -mutexprofile=mutex.out ./test/
     go tool pprof cpu.out
     go tool pprof mem.out
     go tool pprof mutex.out
     ```
   - For flamegraphs and advanced analysis, see Go pprof documentation.
8. Run matching engine latency benchmarks:
   ```powershell
   go test -tags=performance -bench=BenchmarkMatchingEngineLatency ./test/
   ```
   - For profiling (CPU, memory, mutex):
     ```powershell
     go test -tags=performance -bench=BenchmarkMatchingEngineLatency -cpuprofile=cpu.out -memprofile=mem.out -mutexprofile=mutex.out ./test/
     go tool pprof cpu.out
     go tool pprof mem.out
     go tool pprof mutex.out
     ```
   - For flamegraphs and advanced analysis, see Go pprof documentation.
9. Run market data distribution latency benchmarks:
   ```powershell
   go test -tags=performance -bench=BenchmarkMarketDataDistributionLatency ./test/
   ```
   - For profiling (CPU, memory, mutex):
     ```powershell
     go test -tags=performance -bench=BenchmarkMarketDataDistributionLatency -cpuprofile=cpu.out -memprofile=mem.out -mutexprofile=mutex.out ./test/
     go tool pprof cpu.out
     go tool pprof mem.out
     go tool pprof mutex.out
     ```
   - For flamegraphs and advanced analysis, see Go pprof documentation.
10. Run database read/write performance benchmarks:
    ```powershell
    go test -tags=performance -bench=BenchmarkDatabaseReadWritePerformance ./test/
    ```
    - For profiling (CPU, memory, mutex):
      ```powershell
      go test -tags=performance -bench=BenchmarkDatabaseReadWritePerformance -cpuprofile=cpu.out -memprofile=mem.out -mutexprofile=mutex.out ./test/
      go tool pprof cpu.out
      go tool pprof mem.out
      go tool pprof mutex.out
      ```
    - For flamegraphs and advanced analysis, see Go pprof documentation.
11. Run end-to-end transaction latency benchmarks:
    ```powershell
    go test -tags=performance -bench=BenchmarkEndToEndTransactionLatency ./test/
    ```
    - For profiling (CPU, memory, mutex):
      ```powershell
      go test -tags=performance -bench=BenchmarkEndToEndTransactionLatency -cpuprofile=cpu.out -memprofile=mem.out -mutexprofile=mutex.out ./test/
      go tool pprof cpu.out
      go tool pprof mem.out
      go tool pprof mutex.out
      ```
    - For flamegraphs and advanced analysis, see Go pprof documentation.

## Code Conventions

- Use dependency injection via constructors:
  ```go
  NewServer(logger, identitiesSvc, bookkeeperSvc, fiatSvc, marketFeedsSvc, tradingSvc)
  ```
- Centralize error mapping in `errorMapper.mapError` and use `writeError` for HTTP handlers.
- Structured logging with `go.uber.org/zap`.
- Follow standard layout: `cmd/`, `internal/`, `pkg/`.

## Configuration

All configuration is managed by Viper in `internal/config/config.go`. Retrieve values:

```go
cfg := config.Load()
port := cfg.GetString("PORT")
```

## Logging

Use the `pkg/logger` helper to create a global logger:

```go
log := logger.NewLogger(cfg.LogLevel)
```
