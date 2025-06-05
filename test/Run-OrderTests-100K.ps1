# 100K Order Test Script

Set-Location -Path "c:\Orbit CEX\Finalex"

Write-Host "Running 100,000 order test to measure performance..." -ForegroundColor Cyan
go test .\test\high_volume_matching_test.go -tags=trading -run=TestHighVolumeMatching -v

Write-Host "`nRunning order matching benchmark..." -ForegroundColor Cyan
go test .\test\high_volume_matching_test.go -tags=trading -bench=BenchmarkHighVolumeOrderMatching -v

Write-Host "`nTesting heap vs B-tree data structures..." -ForegroundColor Cyan
go test .\test\high_volume_matching_test.go -tags=trading -bench=BenchmarkHeapVsBTree -v

Write-Host "`nTest complete!" -ForegroundColor Green
