@echo off
cd /d "%~dp0"
docker compose down
echo 서버를 중지했습니다.
pause
