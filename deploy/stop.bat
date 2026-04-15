@echo off
chcp 65001 >nul
echo 서버 중지 중...
docker compose down
echo 서버가 중지되었습니다.
pause
