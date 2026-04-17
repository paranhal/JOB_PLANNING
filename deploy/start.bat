@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo ========================================
echo   고객지원시스템 업무일지 설치/실행
echo ========================================
echo.

docker info >nul 2>&1
if errorlevel 1 (
  echo [오류] Docker에 연결할 수 없습니다.
  echo   Docker Desktop을 켠 뒤 엔진이 준비될 때까지 기다렸다가
  echo   이 파일을 다시 실행하세요.
  echo.
  echo   배포 폴더:
  echo   %~dp0 
  echo   위 폴더의 start.bat 을 실행해야 합니다. 프로젝트 루트에서 짧게 치는 명령과는 다릅니다.
  echo ========================================
  pause
  exit /b 1
)

REM Docker 이미지 로드 (최초 1회만)
echo [1/3] Docker 이미지 로드 중...
docker load -i server-app.tar
echo.

REM data 폴더 생성
if not exist data mkdir data

REM 서버 실행
echo [2/3] 서버 시작 중...
docker compose up -d
echo.

echo [3/3] 완료!
echo.
echo   접속 주소: http://localhost:8080
echo   관리자 계정: admin / admin
echo.
echo   중지: docker compose down
echo   로그: docker logs server-app-1
echo   또는 deploy 폴더에서: docker compose logs -f app
echo ========================================
pause
