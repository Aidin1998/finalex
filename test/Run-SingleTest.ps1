param(
    [Parameter(Mandatory=$true)]
    [string]$TestFile
)

# Get the absolute path of the test file
$fullPath = Join-Path -Path $PSScriptRoot -ChildPath $TestFile

Write-Host "Running test: $fullPath"
go test -v -tags=trading $fullPath ./common_test_types.go
