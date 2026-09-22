#!/bin/sh
# 빌드 → 전송 → 교체 → 확인. server-app.tar 한 파일만 올린다. data/ 는 절대 안 올린다.
# 쓰는 법:  ./deploy/push.sh          (빌드까지 다 함)
#           ./deploy/push.sh --no-build  (이미 만든 tar 를 올리기만)
set -e
cd "$(dirname "$0")/.."
ROOT=$(pwd)

HOST=${DEPLOY_HOST:-visionit}         # ~/.ssh/config 의 Host 이름 (Tailscale 주소·계정은 거기에)
PORT=${DEPLOY_PORT:-11800}            # SSH. 앱 HTTP는 8888. 호스트에 :11800 을 붙이지 말 것
REMOTE=${DEPLOY_DIR:-/home/sys2/deploy}

if [ "$1" != "--no-build" ]; then
  echo "== 1/4 빌드 =="
  sh deploy/build.sh
else
  echo "== 1/4 빌드 건너뜀 =="
fi

TAR="$ROOT/deploy/server-app.tar"
[ -f "$TAR" ] || { echo "없음: $TAR"; exit 1; }
SIZE=$(wc -c < "$TAR" | tr -d ' ')
echo "로컬 tar: $SIZE 바이트"

echo "== 2/4 전송 (tar 한 파일만) =="
scp -P "$PORT" "$TAR" "$HOST:$REMOTE/server-app.tar"

echo "== 3/4 서버에서 교체 =="
ssh -p "$PORT" "$HOST" "
  set -e
  cd '$REMOTE'
  REMOTE_SIZE=\$(wc -c < server-app.tar | tr -d ' ')
  echo \"서버 tar: \$REMOTE_SIZE 바이트\"
  if [ \"\$REMOTE_SIZE\" != '$SIZE' ]; then
    echo '전송이 잘렸다. 중단한다.'; exit 1
  fi
  docker compose down
  docker load -i server-app.tar
  docker compose up -d --force-recreate
"

echo "== 4/4 확인 =="
sleep 6
ssh -p "$PORT" "$HOST" "curl -s localhost:8888/version" ; echo
echo
echo "version 이 VERSION 파일($(cat VERSION 2>/dev/null))과 같고,"
echo "index.as_receipts 가 1175 근처면 운영 DB가 그대로인 것이다."
