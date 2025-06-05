Write-Host "Running Trading Tests One by One" -ForegroundColor Yellow
$testDir = $PSScriptRoot
$testFiles = @(
    "trading_models_test.go",
    "trading_coordination_test.go",
    "trading_integration_test.go",
    "trading_edge_cases_test.go",
    "trading_benchmarks_test.go",
    "trading_handlers_test.go",
    "trading_memory_latency_test.go",
    "trading_stress_load_test.go",
    "trading_service_test.go",
    "trading_security_test.go",
    "trading_performance_test.go",
    "trading_order_types_test.go",
    "trading_websocket_test.go"
)

$commonFile = Join-Path -Path $testDir -ChildPath "common_test_types.go"
cd (Split-Path -Parent $testDir)

foreach ($testFile in $testFiles) {
    $fullPath = Join-Path -Path $testDir -ChildPath $testFile
    Write-Host "`nTesting: $testFile" -ForegroundColor Cyan
    Write-Host "-------------------------------------------" -ForegroundColor Cyan
    $command = "go test -v -tags=trading $fullPath $commonFile"
    
    try {
        Invoke-Expression $command
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✓ Test passed: $testFile" -ForegroundColor Green
        } else {
            Write-Host "✗ Test failed: $testFile" -ForegroundColor Red
        }
    } catch {
        Write-Host "✗ Error running test: $testFile" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
    }
    
    Write-Host "-------------------------------------------`n" -ForegroundColor Cyan
}
