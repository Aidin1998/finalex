# Ultra-High Concurrency Database Layer Test Runner (PowerShell)
# Comprehensive test execution for Windows environments

param(
    [Parameter(Position=0)]
    [ValidateSet("all", "unit", "integration", "benchmark", "models", "cache", "repository", "performance", "quick", "ci", "docker", "load", "stress")]
    [string]$TestType = "all",
    
    [switch]$Verbose,
    [switch]$Coverage,
    [switch]$Race,
    [string]$Timeout = "30m",
    [string]$Output = "console"
)

# Configuration
$TestTimeout = $Timeout
$BenchmarkDuration = "60s"
$IntegrationTimeout = "10m"
$UnitTimeout = "5m"

# Colors for output
$Colors = @{
    Red = "Red"
    Green = "Green"
    Yellow = "Yellow"
    Blue = "Blue"
    Cyan = "Cyan"
    White = "White"
}

function Write-ColorOutput {
    param([string]$Message, [string]$Color = "White")
    Write-Host $Message -ForegroundColor $Colors[$Color]
}

function Write-Section {
    param([string]$Title)
    Write-ColorOutput "`n$Title" "Yellow"
    Write-ColorOutput ("=" * $Title.Length) "Yellow"
}

function Invoke-TestCategory {
    param(
        [string]$CategoryName,
        [string]$BuildTag,
        [string]$TimeoutValue,
        [string[]]$ExtraFlags = @()
    )
    
    Write-Section "Running $CategoryName Tests"
    Write-Host "Build tag: $BuildTag"
    Write-Host "Timeout: $TimeoutValue"
    Write-Host ""
    
    $startTime = Get-Date
    
    # Build test command
    $testArgs = @(
        "test"
        "-v"
        "-tags=$BuildTag"
        "-timeout=$TimeoutValue"
    )
    
    if ($Race) {
        $testArgs += "-race"
    }
    
    if ($Coverage -and $BuildTag -eq "unit") {
        $testArgs += "-coverprofile=coverage.out"
    }
    
    $testArgs += $ExtraFlags
    $testArgs += "./..."
    
    try {
        Push-Location "test"
        $result = & go @testArgs
        $exitCode = $LASTEXITCODE
        
        $endTime = Get-Date
        $duration = ($endTime - $startTime).TotalSeconds
        
        if ($exitCode -eq 0) {
            Write-ColorOutput "✓ $CategoryName tests passed in $([math]::Round($duration, 1))s" "Green"
            return $true
        } else {
            Write-ColorOutput "✗ $CategoryName tests failed after $([math]::Round($duration, 1))s" "Red"
            if ($Verbose) {
                Write-Host $result
            }
            return $false
        }
    }
    catch {
        Write-ColorOutput "✗ Error running $CategoryName tests: $($_.Exception.Message)" "Red"
        return $false
    }
    finally {
        Pop-Location
    }
}

function Invoke-TestFile {
    param(
        [string]$FilePath,
        [string]$BuildTag,
        [string]$TimeoutValue,
        [string[]]$ExtraFlags = @()
    )
    
    Write-Section "Running $(Split-Path $FilePath -Leaf)"
    Write-Host "File: $FilePath"
    Write-Host "Build tag: $BuildTag"
    Write-Host ""
    
    $testArgs = @(
        "test"
        "-v"
        "-tags=$BuildTag"
        "-timeout=$TimeoutValue"
    )
    
    if ($Race) {
        $testArgs += "-race"
    }
    
    $testArgs += $ExtraFlags
    $testArgs += $FilePath
    
    try {
        Push-Location "test"
        $result = & go @testArgs
        $exitCode = $LASTEXITCODE
        
        if ($exitCode -eq 0) {
            Write-ColorOutput "✓ $(Split-Path $FilePath -Leaf) passed" "Green"
            return $true
        } else {
            Write-ColorOutput "✗ $(Split-Path $FilePath -Leaf) failed" "Red"
            if ($Verbose) {
                Write-Host $result
            }
            return $false
        }
    }
    catch {
        Write-ColorOutput "✗ Error running test file: $($_.Exception.Message)" "Red"
        return $false
    }
    finally {
        Pop-Location
    }
}

function Test-Prerequisites {
    Write-Section "Checking Prerequisites"
    
    # Check Go installation
    try {
        $goVersion = & go version
        Write-ColorOutput "✓ Go: $goVersion" "Green"
    }
    catch {
        Write-ColorOutput "✗ Go is not installed or not in PATH" "Red"
        return $false
    }
    
    # Check if we're in the right directory
    if (-not (Test-Path "go.mod")) {
        Write-ColorOutput "✗ go.mod not found. Please run from project root." "Red"
        return $false
    }
    
    # Check test directory
    if (-not (Test-Path "test")) {
        Write-ColorOutput "✗ Test directory not found" "Red"
        return $false
    }
    
    Write-ColorOutput "✓ All prerequisites met" "Green"
    return $true
}

function Start-DockerEnvironment {
    Write-Section "Starting Docker Test Environment"
    
    try {
        Push-Location "test"
        
        # Stop any existing containers
        & docker-compose -f docker-compose.test.yml down -v 2>$null
        
        # Start test environment
        & docker-compose -f docker-compose.test.yml up -d postgres-test redis-test
        
        # Wait for services to be healthy
        Write-Host "Waiting for services to be ready..."
        $timeout = 60
        $elapsed = 0
        
        do {
            Start-Sleep 2
            $elapsed += 2
            
            $pgReady = & docker-compose -f docker-compose.test.yml exec -T postgres-test pg_isready -U test_user -d accounts_test
            $redisReady = & docker-compose -f docker-compose.test.yml exec -T redis-test redis-cli ping
            
            if ($pgReady -match "accepting connections" -and $redisReady -match "PONG") {
                Write-ColorOutput "✓ Services are ready" "Green"
                return $true
            }
        } while ($elapsed -lt $timeout)
        
        Write-ColorOutput "✗ Services failed to start within timeout" "Red"
        return $false
    }
    catch {
        Write-ColorOutput "✗ Error starting Docker environment: $($_.Exception.Message)" "Red"
        return $false
    }
    finally {
        Pop-Location
    }
}

function Stop-DockerEnvironment {
    Write-Section "Stopping Docker Test Environment"
    
    try {
        Push-Location "test"
        & docker-compose -f docker-compose.test.yml down -v
        Write-ColorOutput "✓ Docker environment stopped" "Green"
    }
    catch {
        Write-ColorOutput "✗ Error stopping Docker environment: $($_.Exception.Message)" "Red"
    }
    finally {
        Pop-Location
    }
}

function Invoke-LoadTest {
    Write-Section "Running Load Tests with k6"
    
    try {
        Push-Location "test"
        
        # Start application container if not running
        & docker-compose -f docker-compose.test.yml up -d accounts-app
        Start-Sleep 10
        
        # Run k6 load test
        & docker-compose -f docker-compose.test.yml run --rm k6-load-test run /scripts/ultra-high-concurrency-test.js
        
        Write-ColorOutput "✓ Load test completed" "Green"
        return $true
    }
    catch {
        Write-ColorOutput "✗ Load test failed: $($_.Exception.Message)" "Red"
        return $false
    }
    finally {
        Pop-Location
    }
}

function Generate-CoverageReport {
    if (Test-Path "test/coverage.out") {
        Write-Section "Generating Coverage Report"
        
        try {
            Push-Location "test"
            
            # Generate HTML coverage report
            & go tool cover -html=coverage.out -o coverage.html
            
            # Show coverage summary
            $coverageData = & go tool cover -func=coverage.out
            $totalCoverage = ($coverageData | Select-String "total:" | ForEach-Object { $_.Line.Split("`t")[-1] })
            
            Write-ColorOutput "Coverage Report Generated: test/coverage.html" "Green"
            Write-ColorOutput "Total Coverage: $totalCoverage" "Cyan"
            
            # Check coverage threshold
            $coveragePercent = [float]($totalCoverage -replace '%', '')
            if ($coveragePercent -lt 85.0) {
                Write-ColorOutput "⚠️  Coverage below threshold (85%): $coveragePercent%" "Yellow"
            } else {
                Write-ColorOutput "✓ Coverage meets threshold: $coveragePercent%" "Green"
            }
        }
        catch {
            Write-ColorOutput "✗ Error generating coverage report: $($_.Exception.Message)" "Red"
        }
        finally {
            Pop-Location
        }
    }
}

# Main execution logic
Write-ColorOutput "Ultra-High Concurrency Database Layer Test Suite" "Blue"
Write-ColorOutput "=================================================" "Blue"
Write-Host ""

if (-not (Test-Prerequisites)) {
    exit 1
}

$success = $true

try {
    switch ($TestType.ToLower()) {
        "unit" {
            Write-Host "Running Unit Tests Only"
            $success = Invoke-TestCategory "Unit" "unit" $UnitTimeout
        }
        
        "integration" {
            Write-Host "Running Integration Tests Only"
            $success = Invoke-TestCategory "Integration" "integration" $IntegrationTimeout
        }
        
        "benchmark" {
            Write-Host "Running Benchmark Tests Only"
            $success = Invoke-TestCategory "Benchmark" "benchmark" $TestTimeout @("-benchmem", "-run=XXX", "-bench=.")
        }
        
        "models" {
            Write-Host "Running Models Tests Only"
            $success = Invoke-TestFile "./accounts_models_test.go" "unit" $UnitTimeout
        }
        
        "cache" {
            Write-Host "Running Cache Tests Only"
            $success = Invoke-TestFile "./accounts_cache_test.go" "integration" $IntegrationTimeout
        }
        
        "repository" {
            Write-Host "Running Repository Tests Only"
            $success = Invoke-TestFile "./accounts_repository_test.go" "integration" $IntegrationTimeout
        }
        
        "performance" {
            Write-Host "Running Performance Benchmarks Only"
            $success = Invoke-TestFile "./accounts_benchmark_test.go" "benchmark" $TestTimeout @("-benchmem", "-run=XXX", "-bench=.")
        }
        
        "quick" {
            Write-Host "Running Quick Test Suite (Unit + Basic Integration)"
            $success = Invoke-TestCategory "Unit" "unit" $UnitTimeout
            if ($success) {
                Write-ColorOutput "Running basic integration tests..." "Blue"
                Push-Location "test"
                $result = & go test -v -tags=integration -timeout=$IntegrationTimeout -run="TestCache.*Basic|TestRepository.*Basic" ./...
                $success = $LASTEXITCODE -eq 0
                Pop-Location
            }
        }
        
        "ci" {
            Write-Host "Running CI Test Suite (Unit + Integration, no benchmarks)"
            $success = Invoke-TestCategory "Unit" "unit" $UnitTimeout
            if ($success) {
                $success = Invoke-TestCategory "Integration" "integration" $IntegrationTimeout
            }
        }
        
        "docker" {
            Write-Host "Running Tests with Docker Environment"
            if (Start-DockerEnvironment) {
                $env:POSTGRES_TEST_URL = "postgres://test_user:test_password@localhost:5432/accounts_test"
                $env:REDIS_TEST_URL = "redis://localhost:6379/15"
                
                $success = Invoke-TestCategory "Unit" "unit" $UnitTimeout
                if ($success) {
                    $success = Invoke-TestCategory "Integration" "integration" $IntegrationTimeout
                }
                
                Stop-DockerEnvironment
            } else {
                $success = $false
            }
        }
        
        "load" {
            Write-Host "Running Load Tests"
            if (Start-DockerEnvironment) {
                $success = Invoke-LoadTest
                Stop-DockerEnvironment
            } else {
                $success = $false
            }
        }
        
        "stress" {
            Write-Host "Running Stress Tests"
            $success = Invoke-TestCategory "Benchmark" "benchmark" $TestTimeout @("-benchmem", "-run=XXX", "-bench=BenchmarkStress.*")
        }
        
        default {
            Write-Host "Running Complete Test Suite"
            
            # Unit tests first (fastest)
            $success = Invoke-TestCategory "Unit" "unit" $UnitTimeout
            
            if ($success) {
                # Integration tests
                $success = Invoke-TestCategory "Integration" "integration" $IntegrationTimeout
            }
            
            if ($success) {
                # Benchmark tests (slowest)
                Write-ColorOutput "Running performance benchmarks (this may take several minutes)..." "Yellow"
                $success = Invoke-TestCategory "Benchmark" "benchmark" $TestTimeout @("-benchmem", "-run=XXX", "-bench=.")
            }
            
            if ($success) {
                Write-ColorOutput "All test categories completed successfully!" "Green"
            }
        }
    }
}
finally {
    if ($Coverage) {
        Generate-CoverageReport
    }
}

Write-Host ""
if ($success) {
    Write-ColorOutput "Test execution completed successfully." "Green"
    exit 0
} else {
    Write-ColorOutput "Test execution failed." "Red"
    exit 1
}
