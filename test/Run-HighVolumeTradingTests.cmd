@echo off
echo Running High Volume Trading Tests...
powershell -ExecutionPolicy Bypass -File "%~dp0Run-HighVolumeTradingTests.ps1"
pause
