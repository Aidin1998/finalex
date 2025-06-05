# Run-TradingBenchmarks.ps1
# Script to run trading benchmarks one by one in PowerShell

Write-Host "============================================================" -ForegroundColor Green
Write-Host "Running Trading Engine Benchmarks for Different Order Types" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green

# Basic Order Processing Benchmarks (Simple Order Types)
Write-Host "`nRunning Basic Order Processing Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkOrderProcessing -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Concurrent Orders Benchmarks
Write-Host "`nRunning Concurrent Orders Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkConcurrentOrders -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Trading Engine Benchmarks (Complex scenarios)
Write-Host "`nRunning Trading Engine Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkTradingEngine -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Concurrent Order Processing with Different Concurrency Levels
Write-Host "`nRunning Concurrent Order Processing Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkConcurrentOrderProcessing -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Order Book Operations Benchmarks
Write-Host "`nRunning Order Book Operations Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkOrderBookOperations -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Market Data Distribution Benchmarks
Write-Host "`nRunning Market Data Distribution Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkMarketDataDistribution -benchmem -v -tags=trading

Write-Host "`n============================================================"

# WebSocket Broadcast Benchmarks
Write-Host "`nRunning WebSocket Broadcast Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkWebSocketBroadcast -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Real-world Scenarios Benchmarks
Write-Host "`nRunning Real-world Scenario Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkRealWorldScenarios -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Order Type Comparison Benchmarks (comparing different order types)
Write-Host "`nRunning Order Type Comparison Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkOrderTypeComparison -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Mixed Order Load Benchmarks
Write-Host "`nRunning Mixed Order Load Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkMixedOrderLoad -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Memory Usage Benchmarks
Write-Host "`nRunning Memory Usage Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkMemoryUsage -benchmem -v -tags=trading

Write-Host "`n============================================================"

# Latency Under Load Benchmarks
Write-Host "`nRunning Latency Under Load Benchmarks..." -ForegroundColor Cyan
go test -bench=BenchmarkLatencyUnderLoad -benchmem -v -tags=trading

Write-Host "`n============================================================"
Write-Host "All trading benchmarks complete!" -ForegroundColor Green
