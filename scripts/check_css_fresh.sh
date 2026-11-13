#!/usr/bin/env bash
# 95-187 C: 커밋된 server/web/static/vendor/tailwind.css 가 템플릿·JS·Go 소스와 맞는지 확인한다.
# 임시 위치에 새로 빌드해 비교만 하며 저장소 파일은 바꾸지 않는다. 다르면 종료 코드 1.
# 해결: (cd server && npm run build:css) 후 tailwind.css 를 커밋한다.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/server"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
npx --no-install tailwindcss -i ./web/css/input.css -o "$TMP/tailwind.css" --minify >/dev/null 2>&1
if cmp -s "$TMP/tailwind.css" web/static/vendor/tailwind.css; then
  echo "OK: tailwind.css 가 최신입니다."
else
  echo "STALE: tailwind.css 가 소스와 다릅니다. (cd server && npm run build:css) 로 재생성해 커밋하세요." >&2
  exit 1
fi
