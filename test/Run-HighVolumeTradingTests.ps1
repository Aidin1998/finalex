Write-Host "Running High Volume Trading Tests" -ForegroundColor Yellow
Write-Host "----------------------------------------" -ForegroundColor Yellow
Write-Host "These tests will run 100K orders through both heap and B-tree matching engines" -ForegroundColor Cyan

$errorCount = 0
$testDir = $PSScriptRoot
$commonFile = Join-Path -Path $testDir -ChildPath "common_test_types.go"
cd (Split-Path -Parent $testDir)

# Define the tests we want to run
$tests = @(
    @{
        File = "trading_integration_test.go"
        TestName = "TestTradingIntegrationTestSuite/TestPerformance" 
        Description = "High volume order placement through regular matching engine (heap-based)"
    },
    @{
        File = "trading_performance_test.go"
        TestName = "TestTradingPerformanceTestSuite/TestHighVolumeOrderPlacement"
        Description = "High performance order placement benchmark"
    },
    @{
        File = "trading_stress_load_test.go"
        TestName = "TestTradingStressLoadTestSuite/TestHighVolumeOrderProcessing"
        Description = "Stress test with high volume order processing"
    }
)

# Run each test
foreach ($test in $tests) {
    $fullPath = Join-Path -Path $testDir -ChildPath $test.File
    
    Write-Host "`n==================================================" -ForegroundColor Yellow
    Write-Host "Running: $($test.Description)" -ForegroundColor Cyan
    Write-Host "Test: $($test.TestName)" -ForegroundColor Cyan
    Write-Host "File: $($test.File)" -ForegroundColor Cyan
    Write-Host "==================================================" -ForegroundColor Yellow
    
    $command = "go test -v -tags=trading -timeout=10m -run=$($test.TestName) $fullPath $commonFile"
    
    try {
        Invoke-Expression $command
        if ($LASTEXITCODE -eq 0) {
            Write-Host "`n✓ Success: $($test.Description)" -ForegroundColor Green
        } else {
            Write-Host "`n✗ Failed: $($test.Description)" -ForegroundColor Red
            $errorCount++
        }
    } catch {
        Write-Host "`n✗ Error running test: $($test.Description)" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        $errorCount++
    }
}

# Run benchmarks for trading engine with both implementations
Write-Host "`n==================================================" -ForegroundColor Yellow
Write-Host "Running Trading Engine Benchmarks" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Yellow

$benchFile = Join-Path -Path $testDir -ChildPath "trading_benchmarks_test.go"
$command = "go test -v -tags=trading -timeout=5m -bench=. $benchFile $commonFile"

try {
    Invoke-Expression $command
    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✓ Success: Trading Engine Benchmarks" -ForegroundColor Green
    } else {
        Write-Host "`n✗ Failed: Trading Engine Benchmarks" -ForegroundColor Red
        $errorCount++
    }
} catch {
    Write-Host "`n✗ Error running benchmarks" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    $errorCount++
}

# Report results
Write-Host "`n==================================================" -ForegroundColor Yellow
Write-Host "Test Summary" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Yellow

if ($errorCount -eq 0) {
    Write-Host "✓ All tests completed successfully!" -ForegroundColor Green
} else {
    Write-Host "✗ $errorCount tests failed." -ForegroundColor Red
}
