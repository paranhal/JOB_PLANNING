#!/bin/sh
# 서버 배포 스크립트 — server-app.tar 를 올린 폴더에서 실행한다.
# 배포가 실제로 됐는지 끝에서 확인까지 한다. (§40.7)
set -e

cd "$(dirname "$0")"

if [ ! -f server-app.tar ]; then
  echo "server-app.tar 가 없습니다. 먼저 복사하세요."
  exit 1
fi

echo "== 이전 버전 =========================================="
curl -s localhost:8888/version || echo "  (응답 없음)"
echo ""

echo "== 내리기 ============================================="
docker compose down

echo "== 이미지 올리기 ======================================"
docker load -i server-app.tar

echo "== 띄우기 ============================================="
docker compose up -d

echo "== 기동 대기 =========================================="
for i in 1 2 3 4 5 6 7 8 9 10; do
  sleep 2
  if curl -sf localhost:8888/version >/dev/null 2>&1; then break; fi
  echo "  기다리는 중... ($i)"
done

echo ""
echo "== 새 버전 ============================================"
curl -s localhost:8888/version
echo ""
echo ""
echo "  built 가 방금 빌드한 시각인지,"
echo "  started 가 방금인지 확인하세요."
echo "  둘 다 맞아야 배포가 끝난 것입니다."
