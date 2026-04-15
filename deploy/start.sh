#!/bin/bash
echo "========================================"
echo "  고객지원시스템 (업무일지) 설치/실행"
echo "========================================"
echo ""

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
echo "  로그: docker logs server-app-1"
echo "========================================"
