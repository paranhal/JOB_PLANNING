#!/bin/bash
set -e
cd "$(dirname "$0")"
echo "========================================"
echo "  고객지원시스템 (업무일지) 설치/실행"
echo "========================================"
echo ""

if ! docker info >/dev/null 2>&1; then
  echo "[오류] Docker에 연결할 수 없습니다."
  echo "  Docker Desktop(또는 docker 데몬)을 실행한 뒤 이 스크립트를 다시 실행하세요."
  echo "  이 스크립트는 deploy 폴더에서 ./start.sh 로 실행해야 합니다."
  echo "========================================"
  exit 1
fi

# Docker 이미지 로드 (최초 1회만)
echo "[1/3] Docker 이미지 로드 중..."
docker load -i server-app.tar
echo ""

# data 폴더 생성
mkdir -p data

# 서버 실행
echo "[2/3] 서버 시작 중..."
docker compose up -d
echo ""

echo "[3/3] 완료!"
echo ""
echo "  접속 주소: http://localhost:8080"
echo "  관리자 계정: admin / admin"
echo ""
echo "  중지: ./stop.sh"
echo "  로그: docker logs server-app-1  (또는: docker compose logs -f app)"
echo "========================================"
