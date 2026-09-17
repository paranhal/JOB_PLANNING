@echo off
REM JOB_PLANNING 빌드 — build.ps1 을 부른다. 더블클릭용.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1"
echo.
pause
