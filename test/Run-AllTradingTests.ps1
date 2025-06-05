Write-Host "Running All Trading Tests" -ForegroundColor Yellow
$testDir = $PSScriptRoot
$commonFile = Join-Path -Path $testDir -ChildPath "common_test_types.go"
cd (Split-Path -Parent $testDir)

# Define all tests in order of dependency
$tests = @(
    @{ File = "trading_models_test.go"; Description = "Trading Models Test" },
    @{ File = "trading_service_test.go"; Description = "Trading Service Test" },
    @{ File = "trading_coordination_test.go"; Description = "Trading Coordination Test" },
    @{ File = "trading_order_types_test.go"; Description = "Trading Order Types Test" },
    @{ File = "trading_edge_cases_test.go"; Description = "Trading Edge Cases Test" },
    @{ File = "trading_handlers_test.go"; Description = "Trading Handlers Test" },
    @{ File = "trading_websocket_test.go"; Description = "Trading WebSocket Test" },
    @{ File = "trading_security_test.go"; Description = "Trading Security Test" },
    @{ File = "trading_integration_test.go"; Description = "Trading Integration Test" },
    @{ File = "trading_performance_test.go"; Description = "Trading Performance Test" },
    @{ File = "trading_benchmarks_test.go"; Description = "Trading Benchmarks Test" },
    @{ File = "trading_stress_load_test.go"; Description = "Trading Stress Load Test" },
    @{ File = "trading_memory_latency_test.go"; Description = "Trading Memory Latency Test" }
)

$passedTests = 0
$failedTests = 0

foreach ($test in $tests) {
    $fullPath = Join-Path -Path $testDir -ChildPath $test.File
    Write-Host "`nTesting: $($test.Description)" -ForegroundColor Cyan
    Write-Host "-------------------------------------------" -ForegroundColor Cyan
    
    $command = "go test -v -tags=trading $fullPath $commonFile"
    
    try {
        Invoke-Expression $command
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✓ Test passed: $($test.Description)" -ForegroundColor Green
            $passedTests++
        } else {
            Write-Host "✗ Test failed: $($test.Description)" -ForegroundColor Red
            $failedTests++
        }
    } catch {
        Write-Host "✗ Error running test: $($test.Description)" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        $failedTests++
    }
    
    Write-Host "-------------------------------------------`n" -ForegroundColor Cyan
}

Write-Host "Test Summary:" -ForegroundColor Yellow
Write-Host "Passed: $passedTests" -ForegroundColor Green
Write-Host "Failed: $failedTests" -ForegroundColor $(if ($failedTests -eq 0) { "Green" } else { "Red" })
