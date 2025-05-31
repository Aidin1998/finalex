#!/usr/bin/env powershell

# Order Queue System Test Runner
# Validates zero data loss and sub-30-second recovery requirements

param(
    [string]$TestType = "all",
    [string]$OutputDir = "test-results",
    [switch]$Verbose,
    [switch]$BenchmarkOnly,
    [switch]$ChaosOnly,
    [int]$Timeout = 1800 # 30 minutes default timeout
)

$ErrorActionPreference = "Stop"

Write-Host "=" * 80 -ForegroundColor Green
Write-Host "ORDER QUEUE ZERO DATA LOSS SYSTEM - TEST RUNNER" -ForegroundColor Green
Write-Host "=" * 80 -ForegroundColor Green

# Setup
$WorkspaceRoot = Split-Path $PSScriptRoot
$TestPackage = "github.com/Aidin1998/pincex_unified/internal/orderqueue"
$ResultsDir = Join-Path $WorkspaceRoot $OutputDir

# Create results directory
if (!(Test-Path $ResultsDir)) {
    New-Item -ItemType Directory -Path $ResultsDir -Force | Out-Null
}

$Timestamp = Get-Date -Format "yyyy-MM-dd_HH-mm-ss"

function Write-TestHeader {
    param([string]$Title, [string]$Color = "Cyan")
    Write-Host ""
    Write-Host ("-" * 60) -ForegroundColor $Color
    Write-Host $Title -ForegroundColor $Color
    Write-Host ("-" * 60) -ForegroundColor $Color
}

function Run-TestSuite {
    param([string]$Pattern, [string]$Name, [string]$OutputFile)
    
    Write-TestHeader "Running $Name Tests"
    
    $TestCmd = "go test -v -timeout ${Timeout}s"
    if ($Pattern) {
        $TestCmd += " -run `"$Pattern`""
    }
    $TestCmd += " $TestPackage"
    
    if ($Verbose) {
        Write-Host "Command: $TestCmd" -ForegroundColor Gray
    }
    
    $StartTime = Get-Date
    
    try {
        # Run tests and capture output
        $Output = Invoke-Expression $TestCmd 2>&1
        $ExitCode = $LASTEXITCODE
        
        $EndTime = Get-Date
        $Duration = $EndTime - $StartTime
        
        # Write output to file
        $Output | Out-File -FilePath $OutputFile -Encoding UTF8
        
        # Parse results
        $PassedTests = ($Output | Select-String "PASS:" | Measure-Object).Count
        $FailedTests = ($Output | Select-String "FAIL:" | Measure-Object).Count
        $TotalTests = $PassedTests + $FailedTests
        
        # Display summary
        Write-Host ""
        Write-Host "Test Results for $Name" -ForegroundColor White
        Write-Host "Duration: $($Duration.ToString('mm\:ss'))" -ForegroundColor Gray
        Write-Host "Total Tests: $TotalTests" -ForegroundColor White
        Write-Host "Passed: $PassedTests" -ForegroundColor Green
        Write-Host "Failed: $FailedTests" -ForegroundColor $(if ($FailedTests -gt 0) { "Red" } else { "Green" })
        
        if ($ExitCode -eq 0) {
            Write-Host "✅ $Name: ALL TESTS PASSED" -ForegroundColor Green
        } else {
            Write-Host "❌ $Name: TESTS FAILED" -ForegroundColor Red
            
            # Show failed test details
            $FailureLines = $Output | Select-String "FAIL:|panic:|Error:"
            if ($FailureLines) {
                Write-Host "Failure Details:" -ForegroundColor Red
                $FailureLines | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
            }
        }
        
        return @{
            ExitCode = $ExitCode
            Passed = $PassedTests
            Failed = $FailedTests
            Duration = $Duration
            Output = $Output
        }
    }
    catch {
        Write-Host "❌ Error running $Name tests: $_" -ForegroundColor Red
        return @{
            ExitCode = 1
            Passed = 0
            Failed = 1
            Duration = $Duration
            Output = $_.Exception.Message
        }
    }
}

function Run-Benchmarks {
    Write-TestHeader "Running Performance Benchmarks" "Yellow"
    
    $BenchCmd = "go test -bench=. -benchmem -timeout ${Timeout}s $TestPackage"
    $OutputFile = Join-Path $ResultsDir "benchmarks_$Timestamp.txt"
    
    if ($Verbose) {
        Write-Host "Command: $BenchCmd" -ForegroundColor Gray
    }
    
    try {
        $Output = Invoke-Expression $BenchCmd 2>&1
        $Output | Out-File -FilePath $OutputFile -Encoding UTF8
        
        # Parse benchmark results
        $BenchmarkLines = $Output | Select-String "^Benchmark"
        
        Write-Host ""
        Write-Host "Benchmark Results:" -ForegroundColor Yellow
        foreach ($line in $BenchmarkLines) {
            if ($line -match "Benchmark(\w+).*?(\d+\.\d+)\s+ns/op") {
                $TestName = $matches[1]
                $NsPerOp = [double]$matches[2]
                
                if ($NsPerOp -lt 1000000) { # < 1ms
                    $Color = "Green"
                } elseif ($NsPerOp -lt 10000000) { # < 10ms
                    $Color = "Yellow"
                } else {
                    $Color = "Red"
                }
                
                Write-Host "  $line" -ForegroundColor $Color
            } else {
                Write-Host "  $line" -ForegroundColor Gray
            }
        }
        
        return $LASTEXITCODE -eq 0
    }
    catch {
        Write-Host "❌ Error running benchmarks: $_" -ForegroundColor Red
        return $false
    }
}

function Validate-Requirements {
    param([array]$TestResults)
    
    Write-TestHeader "REQUIREMENTS VALIDATION" "Magenta"
    
    $RequirementsMet = $true
    
    # Check for zero data loss
    $DataLossPattern = "DataLoss.*true|Data.*[Ll]oss.*detected"
    $DataLossFound = $false
    
    foreach ($result in $TestResults) {
        if ($result.Output | Select-String $DataLossPattern) {
            $DataLossFound = $true
            break
        }
    }
    
    Write-Host "Zero Data Loss Requirement:" -NoNewline
    if ($DataLossFound) {
        Write-Host " ❌ VIOLATED" -ForegroundColor Red
        Write-Host "  Data loss was detected during testing" -ForegroundColor Red
        $RequirementsMet = $false
    } else {
        Write-Host " ✅ SATISFIED" -ForegroundColor Green
    }
    
    # Check for recovery time requirement
    $RecoveryTimePattern = "Recovery.*time.*(\d+(?:\.\d+)?)\s*s"
    $MaxRecoveryTime = 0
    
    foreach ($result in $TestResults) {
        $matches = $result.Output | Select-String $RecoveryTimePattern
        foreach ($match in $matches) {
            if ($match.Matches[0].Groups[1].Value) {
                $time = [double]$match.Matches[0].Groups[1].Value
                if ($time -gt $MaxRecoveryTime) {
                    $MaxRecoveryTime = $time
                }
            }
        }
    }
    
    Write-Host "Recovery Time < 30s Requirement:" -NoNewline
    if ($MaxRecoveryTime -gt 30) {
        Write-Host " ❌ VIOLATED" -ForegroundColor Red
        Write-Host "  Maximum recovery time: ${MaxRecoveryTime}s" -ForegroundColor Red
        $RequirementsMet = $false
    } else {
        Write-Host " ✅ SATISFIED" -ForegroundColor Green
        if ($MaxRecoveryTime -gt 0) {
            Write-Host "  Maximum recovery time: ${MaxRecoveryTime}s" -ForegroundColor Green
        }
    }
    
    # Check for critical test failures
    $TotalFailed = ($TestResults | Measure-Object -Property Failed -Sum).Sum
    
    Write-Host "All Critical Tests Passed:" -NoNewline
    if ($TotalFailed -gt 0) {
        Write-Host " ❌ $TotalFailed TESTS FAILED" -ForegroundColor Red
        $RequirementsMet = $false
    } else {
        Write-Host " ✅ ALL PASSED" -ForegroundColor Green
    }
    
    return $RequirementsMet
}

# Main execution
try {
    Set-Location $WorkspaceRoot
    
    Write-Host "Workspace: $WorkspaceRoot" -ForegroundColor Gray
    Write-Host "Test Package: $TestPackage" -ForegroundColor Gray
    Write-Host "Results Directory: $ResultsDir" -ForegroundColor Gray
    Write-Host "Test Type: $TestType" -ForegroundColor Gray
    Write-Host ""
    
    # Verify dependencies
    Write-Host "Verifying dependencies..." -ForegroundColor Gray
    try {
        go mod download 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to download dependencies"
        }
    }
    catch {
        Write-Host "❌ Error downloading dependencies: $_" -ForegroundColor Red
        exit 1
    }
    
    $AllResults = @()
    $OverallSuccess = $true
    
    # Run test suites based on type
    switch ($TestType.ToLower()) {
        "all" {
            # Run complete test suite
            $result = Run-TestSuite "TestCompleteOrderQueueSystem" "Complete System" (Join-Path $ResultsDir "complete_system_$Timestamp.txt")
            $AllResults += $result
            if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
            
            # Run individual test categories
            $result = Run-TestSuite "TestBadgerQueue" "Unit Tests" (Join-Path $ResultsDir "unit_tests_$Timestamp.txt")
            $AllResults += $result
            if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
            
            if (!$BenchmarkOnly) {
                $result = Run-TestSuite "TestChaosEngineering" "Chaos Engineering" (Join-Path $ResultsDir "chaos_tests_$Timestamp.txt")
                $AllResults += $result
                if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
                
                $result = Run-TestSuite "TestDataIntegrity" "Data Integrity" (Join-Path $ResultsDir "integrity_tests_$Timestamp.txt")
                $AllResults += $result
                if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
                
                $result = Run-TestSuite "TestLoadDuringRecovery" "Load Testing" (Join-Path $ResultsDir "load_tests_$Timestamp.txt")
                $AllResults += $result
                if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
            }
        }
        "unit" {
            $result = Run-TestSuite "TestBadgerQueue" "Unit Tests" (Join-Path $ResultsDir "unit_tests_$Timestamp.txt")
            $AllResults += $result
            if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
        }
        "chaos" {
            $result = Run-TestSuite "TestChaosEngineering|TestAllChaosScenarios" "Chaos Engineering" (Join-Path $ResultsDir "chaos_tests_$Timestamp.txt")
            $AllResults += $result
            if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
        }
        "integrity" {
            $result = Run-TestSuite "TestDataIntegrity" "Data Integrity" (Join-Path $ResultsDir "integrity_tests_$Timestamp.txt")
            $AllResults += $result
            if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
        }
        "performance" {
            if (!$BenchmarkOnly) {
                $result = Run-TestSuite "BenchmarkRecovery|BenchmarkBadgerQueue" "Performance Tests" (Join-Path $ResultsDir "performance_tests_$Timestamp.txt")
                $AllResults += $result
                if ($result.ExitCode -ne 0) { $OverallSuccess = $false }
            }
        }
        default {
            Write-Host "❌ Invalid test type: $TestType" -ForegroundColor Red
            Write-Host "Valid types: all, unit, chaos, integrity, performance" -ForegroundColor Yellow
            exit 1
        }
    }
    
    # Run benchmarks if requested
    if ($BenchmarkOnly -or $TestType -eq "all" -or $TestType -eq "performance") {
        $BenchmarkSuccess = Run-Benchmarks
        if (!$BenchmarkSuccess) { $OverallSuccess = $false }
    }
    
    # Generate final report
    Write-TestHeader "FINAL SYSTEM VALIDATION" "Magenta"
    
    $TotalPassed = ($AllResults | Measure-Object -Property Passed -Sum).Sum
    $TotalFailed = ($AllResults | Measure-Object -Property Failed -Sum).Sum
    $TotalTests = $TotalPassed + $TotalFailed
    
    Write-Host ""
    Write-Host "OVERALL TEST SUMMARY" -ForegroundColor White
    Write-Host "Total Tests Run: $TotalTests" -ForegroundColor White
    Write-Host "Total Passed: $TotalPassed" -ForegroundColor Green
    Write-Host "Total Failed: $TotalFailed" -ForegroundColor $(if ($TotalFailed -gt 0) { "Red" } else { "Green" })
    
    if ($TotalTests -gt 0) {
        $PassRate = [math]::Round(($TotalPassed / $TotalTests) * 100, 1)
        Write-Host "Pass Rate: $PassRate%" -ForegroundColor $(if ($PassRate -ge 100) { "Green" } elseif ($PassRate -ge 95) { "Yellow" } else { "Red" })
    }
    
    # Validate requirements
    $RequirementsMet = Validate-Requirements $AllResults
    
    Write-Host ""
    Write-Host "SYSTEM CERTIFICATION" -ForegroundColor White
    if ($RequirementsMet -and $OverallSuccess) {
        Write-Host "🎉 SYSTEM CERTIFIED FOR PRODUCTION" -ForegroundColor Green
        Write-Host "   ✅ Zero data loss guarantee validated" -ForegroundColor Green
        Write-Host "   ✅ Sub-30-second recovery verified" -ForegroundColor Green
        Write-Host "   ✅ All critical tests passed" -ForegroundColor Green
        $ExitCode = 0
    } else {
        Write-Host "❌ SYSTEM NOT READY FOR PRODUCTION" -ForegroundColor Red
        if (!$RequirementsMet) {
            Write-Host "   ❌ Requirements validation failed" -ForegroundColor Red
        }
        if (!$OverallSuccess) {
            Write-Host "   ❌ Test failures detected" -ForegroundColor Red
        }
        $ExitCode = 1
    }
    
    Write-Host ""
    Write-Host "Test results saved to: $ResultsDir" -ForegroundColor Gray
    Write-Host "=" * 80 -ForegroundColor Green
    
    exit $ExitCode
}
catch {
    Write-Host "❌ Test runner failed: $_" -ForegroundColor Red
    Write-Host $_.ScriptStackTrace -ForegroundColor Red
    exit 1
}
