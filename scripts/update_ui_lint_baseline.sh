#!/usr/bin/env bash
# 95-93: UI 린트 baseline 은 감소분만 반영. 증가가 있으면 거부.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="$ROOT/server"
BASE="$SERVER/internal/handler/testdata/ui_lint_baseline.json"

cd "$SERVER"
export UPDATE_UI_LINT_BASELINE=1
export UPDATE_UI_LINT_BASELINE_DECREASE_ONLY=1
if ! go test ./internal/handler -count=1 -run TestUILintRatchet -timeout 120s; then
  echo "update_ui_lint_baseline: 증가가 있어 baseline 을 갱신하지 않았습니다." >&2
  exit 1
fi
echo "update_ui_lint_baseline: 감소분만 반영했습니다 ($BASE)"
