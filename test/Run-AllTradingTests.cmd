@echo off
echo Running All Trading Tests...
powershell -ExecutionPolicy Bypass -File "%~dp0Run-AllTradingTests.ps1"
pause
