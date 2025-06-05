Write-Host "Running Trading Integration Test with High Volume" -ForegroundColor Yellow
$testDir = $PSScriptRoot
$commonFile = Join-Path -Path $testDir -ChildPath "common_test_types.go"
$integrationFile = Join-Path -Path $testDir -ChildPath "trading_integration_test.go"

cd (Split-Path -Parent $testDir)

Write-Host "Running test with 100K orders - this may take a while..." -ForegroundColor Cyan
Write-Host "-------------------------------------------" -ForegroundColor Cyan

$command = "go test -v -tags=trading -run TestTradingIntegrationTestSuite/TestHighVolume $integrationFile $commonFile"
    
try {
    Invoke-Expression $command
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ Integration test passed!" -ForegroundColor Green
    } else {
        Write-Host "✗ Integration test failed!" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Error running integration test" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
}
    
Write-Host "-------------------------------------------" -ForegroundColor Cyan

# Run benchmarks
Write-Host "`nRunning Trading Benchmarks" -ForegroundColor Yellow
$benchmarkFile = Join-Path -Path $testDir -ChildPath "trading_benchmarks_test.go"

Write-Host "Running benchmarks - this may take a while..." -ForegroundColor Cyan
Write-Host "-------------------------------------------" -ForegroundColor Cyan

$command = "go test -v -tags=trading -bench=. $benchmarkFile $commonFile"
    
try {
    Invoke-Expression $command
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ Benchmarks completed successfully!" -ForegroundColor Green
    } else {
        Write-Host "✗ Benchmarks failed!" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Error running benchmarks" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
}
    
Write-Host "-------------------------------------------" -ForegroundColor Cyan
