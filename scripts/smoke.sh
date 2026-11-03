#!/usr/bin/env bash
# 95-98 화면 스모크 S1~S5. 실패 시 docs/리뷰/screenshots/95-98-smoke/ 에 스크린샷·콘솔.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="$ROOT/server"
PORT="${SMOKE_PORT:-18098}"
DB="${SMOKE_DB:-$SERVER/tmp/smoke98.db}"
SHOT="$ROOT/docs/리뷰/screenshots/95-98-smoke"
mkdir -p "$SHOT" "$SERVER/tmp"

if ! node -e "require('playwright')" 2>/dev/null && ! node -e "require('playwright-core')" 2>/dev/null; then
  if [[ -f "$SERVER/web/static/js/95_92_as_action_browser.mjs" ]]; then
    echo "playwright 모듈을 서버 JS 경로에서 찾습니다"
  fi
fi
if ! node -e "try{require('playwright');}catch(e){process.exit(1)}" 2>/dev/null; then
  echo "Playwright가 없습니다. 설치: cd $SERVER && npm i -D playwright && npx playwright install chromium" >&2
  echo "또는 저장소 루트에서 npm i playwright" >&2
  exit 2
fi

rm -f "$DB"
(cd "$SERVER" && go run ./cmd/devseed958 "$DB") >/tmp/smoke98-seed.txt 2>&1 || {
  echo "시드 실패, EnsureAdmin 만으로 기동합니다" >&2
}

export PORT DB_PATH="$DB" JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production}"
cd "$SERVER"
go run ./cmd/server >"$SHOT/server.log" 2>&1 &
SPID=$!
cleanup() { kill "$SPID" 2>/dev/null || true; }
trap cleanup EXIT

ok=0
for i in $(seq 1 40); do
  if curl -sf "http://127.0.0.1:$PORT/login" >/dev/null; then ok=1; break; fi
  sleep 0.25
done
if [[ "$ok" -ne 1 ]]; then
  echo "서버가 뜨지 않았습니다" >&2
  tail -40 "$SHOT/server.log" >&2
  exit 1
fi

export SMOKE_BASE="http://127.0.0.1:$PORT"
export SMOKE_USER="${SMOKE_USER:-admin}"
export SMOKE_PASS="${SMOKE_PASS:-admin}"
export NODE_PATH="$SERVER/node_modules${NODE_PATH:+:$NODE_PATH}"
node "$ROOT/server/e2e/smoke/run.mjs"
echo "스모크 통과"
