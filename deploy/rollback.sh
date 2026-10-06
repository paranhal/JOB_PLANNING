#!/bin/sh
# 직전 이미지(server-app:prev)로 앱만 되돌린다. data/ 는 건드리지 않는다.
set -e
cd "$(dirname "$0")"

echo "== 지금 버전 =========================================="
curl -s localhost:8888/version || echo "  (응답 없음)"
echo ""

if ! docker image inspect server-app:prev >/dev/null 2>&1; then
  if [ -f server-app.tar.prev ]; then
    echo "== tar.prev 에서 이미지 올리기 ======================="
    docker load -i server-app.tar.prev
    docker tag server-app:latest server-app:prev
  else
    echo "되돌릴 이미지가 없습니다."
    echo "  배포 때 update.sh 가 server-app:prev 를 남깁니다."
    echo "  또는 배포 전에  cp server-app.tar server-app.tar.prev"
    exit 1
  fi
fi

echo "== 내리기 ============================================="
docker compose down

echo "== 직전 이미지를 latest 로 ============================"
docker tag server-app:prev server-app:latest

echo "== 띄우기 ============================================="
docker compose up -d

echo "== 기동 대기 =========================================="
for i in 1 2 3 4 5 6 7 8 9 10; do
  sleep 2
  if curl -sf localhost:8888/version >/dev/null 2>&1; then break; fi
  echo "  기다리는 중... ($i)"
done

echo ""
echo "== 되돌린 버전 ========================================"
curl -s localhost:8888/version
echo ""
echo "  data/ 는 그대로입니다. DB 까지 예전 날짜로 돌리려면"
echo "  backup-db.sh 로 받아 둔 tar.gz 를 앱을 멈춘 뒤 푸세요."
echo "  그때 이후 입력은 사라집니다."
