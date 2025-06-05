@echo off
REM 100K Order Test Script

cd "c:\Orbit CEX\Finalex"

echo Running 100,000 order test to measure performance...
go test .\test\high_volume_matching_test.go -tags=trading -run=TestHighVolumeMatching -v
echo.
echo.
echo Running order matching benchmark...
go test .\test\high_volume_matching_test.go -tags=trading -bench=BenchmarkHighVolumeOrderMatching -v
echo.
echo.
echo Testing heap vs B-tree data structures...
go test .\test\high_volume_matching_test.go -tags=trading -bench=BenchmarkHeapVsBTree -v
echo.
echo Test complete!
