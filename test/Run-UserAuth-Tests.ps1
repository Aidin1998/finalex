
# UserAuth Test Runner with Metrics Collection
# This script runs all userauth tests and collects performance metrics

# Set error action preference to stop on errors
$ErrorActionPreference = "Stop"

# Define colors for output
$successColor = "Green"
$errorColor = "Red"
$infoColor = "Cyan"
$warnColor = "Yellow"

# Start timestamp for overall execution time
$overallStart = Get-Date

# Create results folder if it doesn't exist
$resultsDir = "test_results"
if (-not (Test-Path -Path $resultsDir)) {
    New-Item -Path $resultsDir -ItemType Directory | Out-Null
}

# Generate timestamp for this test run
$timestamp = (Get-Date).ToString("yyyy-MM-dd_HH-mm-ss")
$resultsFile = "$resultsDir\userauth_test_metrics_$timestamp.txt"
$coverageFile = "$resultsDir\coverage_$timestamp.out"
$coverageHtml = "$resultsDir\coverage_$timestamp.html"

# Write header to results file
"USERAUTH MODULE TEST RESULTS - $timestamp" | Out-File -FilePath $resultsFile
"=======================================================" | Out-File -FilePath $resultsFile -Append
"" | Out-File -FilePath $resultsFile -Append

# Function to run a test and capture metrics
function Run-Test {
    param (
        [string]$TestName,
        [string]$Tags,
        [int]$Timeout = 60
    )
    
    Write-Host "Running test: $TestName..." -ForegroundColor $infoColor
    "## $TestName ##" | Out-File -FilePath $resultsFile -Append
    
    $testStart = Get-Date
    $testCommand = "go test -tags `"$Tags`" ./test -v -run `"$TestName`" -timeout ${Timeout}s"
    
    "Command: $testCommand" | Out-File -FilePath $resultsFile -Append
    
    try {
        $output = Invoke-Expression $testCommand 2>&1
        $success = $LASTEXITCODE -eq 0
        
        # Record output
        $output | Out-File -FilePath $resultsFile -Append
        
        # Calculate and record duration
        $testDuration = (Get-Date) - $testStart
        "Duration: $($testDuration.TotalSeconds) seconds" | Out-File -FilePath $resultsFile -Append
        
        # Print result to console
        if ($success) {
            Write-Host "  √ PASSED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $successColor
        } else {
            Write-Host "  × FAILED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $errorColor
        }
        
        return $success
    } catch {
        "ERROR: $_" | Out-File -FilePath $resultsFile -Append
        Write-Host "  × FAILED (exception)" -ForegroundColor $errorColor
        return $false
    } finally {
        "" | Out-File -FilePath $resultsFile -Append
    }
}

# Function to run all tests with coverage
function Run-TestsWithCoverage {
    Write-Host "Running all tests with coverage..." -ForegroundColor $infoColor
    "## ALL TESTS WITH COVERAGE ##" | Out-File -FilePath $resultsFile -Append
    
    $testStart = Get-Date
    $testCommand = "go test -tags `"userauth`" ./test -v -coverprofile=$coverageFile"
    
    "Command: $testCommand" | Out-File -FilePath $resultsFile -Append
    
    try {
        $output = Invoke-Expression $testCommand 2>&1
        $success = $LASTEXITCODE -eq 0
        
        # Record output
        $output | Out-File -FilePath $resultsFile -Append
        
        # Calculate and record duration
        $testDuration = (Get-Date) - $testStart
        "Duration: $($testDuration.TotalSeconds) seconds" | Out-File -FilePath $resultsFile -Append
        
        # Generate HTML coverage report if tests succeeded
        if ($success) {
            Write-Host "  √ PASSED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $successColor
            Write-Host "Generating coverage report..." -ForegroundColor $infoColor
            
            $coverageCommand = "go tool cover -html=$coverageFile -o $coverageHtml"
            Invoke-Expression $coverageCommand
            
            Write-Host "  Coverage report generated: $coverageHtml" -ForegroundColor $successColor
        } else {
            Write-Host "  × FAILED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $errorColor
        }
        
        return $success
    } catch {
        "ERROR: $_" | Out-File -FilePath $resultsFile -Append
        Write-Host "  × FAILED (exception)" -ForegroundColor $errorColor
        return $false
    } finally {
        "" | Out-File -FilePath $resultsFile -Append
    }
}

# Function to run stress tests
function Run-StressTests {
    Write-Host "Running stress tests..." -ForegroundColor $infoColor
    "## STRESS TESTS ##" | Out-File -FilePath $resultsFile -Append
    
    $testStart = Get-Date
    $testCommand = "go test -tags `"userauth stress`" ./test -v -timeout 5m"
    
    "Command: $testCommand" | Out-File -FilePath $resultsFile -Append
    
    try {
        $output = Invoke-Expression $testCommand 2>&1
        $success = $LASTEXITCODE -eq 0
        
        # Record output
        $output | Out-File -FilePath $resultsFile -Append
        
        # Calculate and record duration
        $testDuration = (Get-Date) - $testStart
        "Duration: $($testDuration.TotalSeconds) seconds" | Out-File -FilePath $resultsFile -Append
        
        # Extract performance metrics (this is a simple approach, can be enhanced)
        $metrics = $output | Select-String -Pattern "requests/second|average|time" | ForEach-Object { $_.Line.Trim() }
        
        if ($metrics) {
            "Performance Metrics:" | Out-File -FilePath $resultsFile -Append
            $metrics | Out-File -FilePath $resultsFile -Append
        }
        
        # Print result to console
        if ($success) {
            Write-Host "  √ PASSED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $successColor
        } else {
            Write-Host "  × FAILED ($($testDuration.TotalSeconds) seconds)" -ForegroundColor $errorColor
        }
        
        return $success
    } catch {
        "ERROR: $_" | Out-File -FilePath $resultsFile -Append
        Write-Host "  × FAILED (exception)" -ForegroundColor $errorColor
        return $false
    } finally {
        "" | Out-File -FilePath $resultsFile -Append
    }
}

# Define test categories
$testCategories = @(
    @{ Name = "Registration Tests"; TestFunc = { Run-Test -TestName "TestRegister" -Tags "userauth" } },
    @{ Name = "Authentication Tests"; TestFunc = { Run-Test -TestName "TestAuthenticate" -Tags "userauth" } },
    @{ Name = "2FA Tests"; TestFunc = { Run-Test -TestName "Test.*2FA|TestTwoFactor" -Tags "userauth" } },
    @{ Name = "Integration Tests"; TestFunc = { Run-Test -TestName "TestFull.*Flow|TestIntegration" -Tags "userauth integration" -Timeout 120 } }
)

# Print test plan
Write-Host "`n=======================================================" -ForegroundColor $infoColor
Write-Host "USERAUTH MODULE TEST PLAN" -ForegroundColor $infoColor
Write-Host "=======================================================" -ForegroundColor $infoColor
Write-Host "1. Run individual test categories"
Write-Host "2. Run all tests with coverage"
Write-Host "3. Run stress tests"
Write-Host "4. Generate combined metrics report"
Write-Host "=======================================================" -ForegroundColor $infoColor

# Run test categories
$testResults = @{}
foreach ($category in $testCategories) {
    Write-Host "`n=======================================================" -ForegroundColor $infoColor
    Write-Host "RUNNING: $($category.Name)" -ForegroundColor $infoColor
    Write-Host "=======================================================" -ForegroundColor $infoColor
    
    $result = & $category.TestFunc
    $testResults[$category.Name] = $result
}

# Run all tests with coverage
Write-Host "`n=======================================================" -ForegroundColor $infoColor
Write-Host "RUNNING: All Tests with Coverage" -ForegroundColor $infoColor
Write-Host "=======================================================" -ForegroundColor $infoColor

$coverageResult = Run-TestsWithCoverage
$testResults["Coverage Tests"] = $coverageResult

# Run stress tests
Write-Host "`n=======================================================" -ForegroundColor $infoColor
Write-Host "RUNNING: Stress Tests" -ForegroundColor $infoColor
Write-Host "=======================================================" -ForegroundColor $infoColor

$stressResult = Run-StressTests
$testResults["Stress Tests"] = $stressResult

# Calculate overall execution time
$overallDuration = (Get-Date) - $overallStart
$overallSeconds = $overallDuration.TotalSeconds

# Write summary to results file
"" | Out-File -FilePath $resultsFile -Append
"## SUMMARY ##" | Out-File -FilePath $resultsFile -Append
"Total execution time: $overallSeconds seconds" | Out-File -FilePath $resultsFile -Append
"" | Out-File -FilePath $resultsFile -Append

foreach ($key in $testResults.Keys) {
    $status = if ($testResults[$key]) { "PASSED" } else { "FAILED" }
    "$key : $status" | Out-File -FilePath $resultsFile -Append
}

# Print summary to console
Write-Host "`n=======================================================" -ForegroundColor $infoColor
Write-Host "TEST SUMMARY" -ForegroundColor $infoColor
Write-Host "=======================================================" -ForegroundColor $infoColor
Write-Host "Total execution time: $overallSeconds seconds`n"

foreach ($key in $testResults.Keys) {
    $color = if ($testResults[$key]) { $successColor } else { $errorColor }
    $status = if ($testResults[$key]) { "PASSED" } else { "FAILED" }
    Write-Host "$key : " -NoNewline
    Write-Host $status -ForegroundColor $color
}

Write-Host "`nDetailed results saved to: $resultsFile" -ForegroundColor $infoColor

if ($coverageResult) {
    Write-Host "Coverage report available at: $coverageHtml" -ForegroundColor $infoColor
}

# Return success if all tests passed
$allPassed = $testResults.Values | ForEach-Object { $_ } | Where-Object { -not $_ } | Measure-Object | Select-Object -ExpandProperty Count
exit $allPassed
