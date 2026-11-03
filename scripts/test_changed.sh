#!/usr/bin/env bash
# 95-98 T1/T2: git diff 로 바뀐 패키지 테스트만 실행.
# 대응 표:
#   server/internal/handler/**     → go test ./internal/handler (-run 는 T1에서 파일 stem)
#   server/internal/repository/**  → go test ./internal/repository
#   server/internal/model/**       → go test ./internal/model
#   server/web/templates/**        → go test ./internal/handler -run 'TestUIContract|TestAllTemplatesParse'
#   scripts/**, Makefile           → 스크립트 자체는 go test 없음
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="$ROOT/server"
MODE="${1:-t1}"

files=()
while IFS= read -r line; do
  [[ -n "$line" ]] && files+=("$line")
done < <(cd "$ROOT" && git diff --name-only HEAD && git diff --name-only --cached && git ls-files --others --exclude-standard)

pkgs=()
run_handler_contract=0
add_pkg() {
  local p="$1"
  for x in "${pkgs[@]+"${pkgs[@]}"}"; do
    [[ "$x" == "$p" ]] && return
  done
  pkgs+=("$p")
}

for f in "${files[@]}"; do
  case "$f" in
    server/internal/handler/*) add_pkg "./internal/handler" ;;
    server/internal/repository/*) add_pkg "./internal/repository" ;;
    server/internal/model/*) add_pkg "./internal/model" ;;
    server/internal/service/leaveapp/*) add_pkg "./internal/service/leaveapp" ;;
    server/internal/service/approval/*) add_pkg "./internal/service/approval" ;;
    server/internal/service/*) add_pkg "./internal/service" ;;
    server/web/templates/*|server/web/static/js/*)
      add_pkg "./internal/handler"
      run_handler_contract=1
      ;;
    server/cmd/server/*) add_pkg "./cmd/server" ;;
  esac
done

if [[ ${#pkgs[@]} -eq 0 ]]; then
  echo "변경 패키지 없음"
  exit 0
fi

cd "$SERVER"
if [[ "$MODE" == "t1" ]]; then
  echo "T1 packages: ${pkgs[*]}"
  go vet "${pkgs[@]}"
  for p in "${pkgs[@]}"; do
    case "$p" in
      ./internal/handler)
        go test ./internal/handler -count=1 -timeout 30s -run 'TestUIContract|TestAllTemplatesParse|TestNavTree|TestRenderSmokeParse'
        ;;
      *)
        go test "$p" -count=1 -timeout 30s
        ;;
    esac
  done
else
  echo "T2 packages: ${pkgs[*]}"
  go test "${pkgs[@]}" -timeout 300s
fi
