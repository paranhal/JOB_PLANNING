@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo ========================================
echo   고객지원시스템 (업무일지) 설치/실행
echo ========================================
echo.

docker info >nul 2>&1
if errorlevel 1 (
  echo [오류] Docker에 연결할 수 없습니다.
  echo   Docker Desktop을 실행한 뒤 다시 시도하세요.
  pause
  exit /b 1
)

echo [1/3] Docker 이미지 로드 중...
docker load -i server-app.tar
echo.

if not exist data mkdir data

echo [2/3] 서버 시작 중...
docker compose down 2>nul
docker compose up -d
echo.

echo [3/3] 완료!
echo.
echo   접속 주소: http://localhost:8888
echo   관리자 계정: admin / admin
echo.
echo   중지: stop.bat
pause
