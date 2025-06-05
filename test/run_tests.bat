@echo off
REM Ultra-high concurrency database layer test runner for Accounts module (Windows)
REM Supports batch execution of all test categories with detailed reporting

setlocal enabledelayedexpansion

REM Test configuration
set TEST_TIMEOUT=30m
set BENCHMARK_DURATION=60s
set INTEGRATION_TIMEOUT=10m
set UNIT_TIMEOUT=5m

echo Ultra-High Concurrency Database Layer Test Suite
echo =================================================
echo.

REM Function to print section headers
:print_section
echo %~1
for /L %%i in (1,1,50) do echo -
echo.
goto :eof

REM Parse command line arguments
set "command=%~1"
if "%command%"=="" set "command=all"

if "%command%"=="unit" (
    echo Running Unit Tests Only
    echo.
    call :print_section "Unit Tests"
    go test -v -tags=unit -timeout=%UNIT_TIMEOUT% ./...
    if !errorlevel! neq 0 (
        echo Unit tests failed
        exit /b 1
    )
    echo Unit tests passed
    goto :end
)

if "%command%"=="integration" (
    echo Running Integration Tests Only
    echo.
    call :print_section "Integration Tests"
    go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% ./...
    if !errorlevel! neq 0 (
        echo Integration tests failed
        exit /b 1
    )
    echo Integration tests passed
    goto :end
)

if "%command%"=="benchmark" (
    echo Running Benchmark Tests Only
    echo.
    call :print_section "Benchmark Tests"
    go test -v -tags=benchmark -timeout=%TEST_TIMEOUT% -benchmem -run=XXX -bench=. ./...
    if !errorlevel! neq 0 (
        echo Benchmark tests failed
        exit /b 1
    )
    echo Benchmark tests passed
    goto :end
)

if "%command%"=="models" (
    echo Running Models Tests Only
    echo.
    call :print_section "Models Tests"
    go test -v -tags=unit -timeout=%UNIT_TIMEOUT% ./accounts_models_test.go
    if !errorlevel! neq 0 (
        echo Models tests failed
        exit /b 1
    )
    echo Models tests passed
    goto :end
)

if "%command%"=="cache" (
    echo Running Cache Tests Only
    echo.
    call :print_section "Cache Tests"
    go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% ./accounts_cache_test.go
    if !errorlevel! neq 0 (
        echo Cache tests failed
        exit /b 1
    )
    echo Cache tests passed
    goto :end
)

if "%command%"=="repository" (
    echo Running Repository Tests Only
    echo.
    call :print_section "Repository Tests"
    go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% ./accounts_repository_test.go
    if !errorlevel! neq 0 (
        echo Repository tests failed
        exit /b 1
    )
    echo Repository tests passed
    goto :end
)

if "%command%"=="performance" (
    echo Running Performance Benchmarks Only
    echo.
    call :print_section "Performance Benchmarks"
    go test -v -tags=benchmark -timeout=%TEST_TIMEOUT% -benchmem -run=XXX -bench=. ./accounts_benchmark_test.go
    if !errorlevel! neq 0 (
        echo Performance benchmarks failed
        exit /b 1
    )
    echo Performance benchmarks passed
    goto :end
)

if "%command%"=="quick" (
    echo Running Quick Test Suite (Unit + Basic Integration^)
    echo.
    call :print_section "Unit Tests"
    go test -v -tags=unit -timeout=%UNIT_TIMEOUT% ./...
    if !errorlevel! neq 0 (
        echo Unit tests failed
        exit /b 1
    )
    echo Unit tests passed
    echo.
    echo Running basic integration tests...
    go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% -run="TestCache.*Basic|TestRepository.*Basic" ./...
    if !errorlevel! neq 0 (
        echo Basic integration tests failed
        exit /b 1
    )
    echo Basic integration tests passed
    goto :end
)

if "%command%"=="ci" (
    echo Running CI Test Suite (Unit + Integration, no benchmarks^)
    echo.
    call :print_section "Unit Tests"
    go test -v -tags=unit -timeout=%UNIT_TIMEOUT% ./...
    if !errorlevel! neq 0 (
        echo Unit tests failed
        exit /b 1
    )
    echo Unit tests passed
    echo.
    call :print_section "Integration Tests"
    go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% ./...
    if !errorlevel! neq 0 (
        echo Integration tests failed
        exit /b 1
    )
    echo Integration tests passed
    goto :end
)

REM Default: run all tests
echo Running Complete Test Suite
echo.

call :print_section "Unit Tests"
go test -v -tags=unit -timeout=%UNIT_TIMEOUT% ./...
if !errorlevel! neq 0 (
    echo Unit tests failed
    exit /b 1
)
echo Unit tests passed
echo.

call :print_section "Integration Tests"
go test -v -tags=integration -timeout=%INTEGRATION_TIMEOUT% ./...
if !errorlevel! neq 0 (
    echo Integration tests failed
    exit /b 1
)
echo Integration tests passed
echo.

echo Running performance benchmarks (this may take several minutes^)...
call :print_section "Benchmark Tests"
go test -v -tags=benchmark -timeout=%TEST_TIMEOUT% -benchmem -run=XXX -bench=. ./...
if !errorlevel! neq 0 (
    echo Benchmark tests failed
    exit /b 1
)
echo Benchmark tests passed
echo.

echo All test categories completed successfully!

:end
echo Test execution completed.
