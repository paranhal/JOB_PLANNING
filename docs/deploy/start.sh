#!/bin/bash
set -e
cd "$(dirname "$0")"
echo "========================================"
echo "  고객지원시스템 (업무일지) 설치/실행"
echo "========================================"
echo ""

if ! docker info >/dev/null 2>&1; then
  echo "[오류] Docker에 연결할 수 없습니다."
  echo "  Docker를 설치·실행한 뒤 이 스크립트를 다시 실행하세요."
  echo "========================================"
  exit 1
fi

# Windows에서 올린 경우 CRLF 제거
sed -i 's/\r$//' "$0" 2>/dev/null || true
sed -i 's/\r$//' stop.sh 2>/dev/null || true

echo "[1/3] Docker 이미지 로드 중..."
docker load -i server-app.tar
echo ""

mkdir -p data

echo "[2/3] 서버 시작 중..."
# 이전 컨테이너가 있으면 최신 이미지로 교체
docker compose down 2>/dev/null || true
docker compose up -d
echo ""

echo "[3/3] 완료!"
echo ""
echo "  접속 주소: http://서버IP:8888"
echo "  관리자 계정: admin / admin  (기존 DB면 기존 계정 사용)"
echo ""
echo "  중지: ./stop.sh"
echo "  로그: docker logs server-app-1"
echo "========================================"
