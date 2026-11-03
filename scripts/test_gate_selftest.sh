#!/usr/bin/env bash
# 95-93 §5.1: test_gate.sh 자체 검증
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GATE="$ROOT/scripts/test_gate.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

json_fail() {
  local pkg="$1" name="$2"
  printf '{"Action":"fail","Package":"customer-support/%s","Test":"%s"}\n' "$pkg" "$name"
}
json_pass() {
  local pkg="$1" name="$2"
  printf '{"Action":"pass","Package":"customer-support/%s","Test":"%s"}\n' "$pkg" "$name"
}

# ① known 에 없는 실패 → 종료 1
{
  json_fail internal/handler TestBrandNewFail
  json_pass internal/handler TestOk
} >"$TMP/new.json"
echo "# empty" >"$TMP/known.txt"
set +e
out="$("$GATE" --json "$TMP/new.json" --known "$TMP/known.txt" 2>&1)"
code=$?
set -e
echo "$out" | grep -q '신규 실패' || { echo "① 신규 안내 없음"; echo "$out"; exit 1; }
echo "$out" | grep -q '통과 1 / 실패 1 (기존 0 · 신규 1 · 해결 0)' || { echo "① 요약 줄 불일치: $out"; exit 1; }
[[ $code -eq 1 ]] || { echo "① 종료 코드 $code"; exit 1; }

# ② known 항목이 통과하면 해결됨 안내 (종료 0)
{
  json_pass internal/handler TestOldFail
} >"$TMP/resolved.json"
echo "internal/handler::TestOldFail  # leftover / 95-93 / 2026-12-31" >"$TMP/known2.txt"
set +e
out="$("$GATE" --json "$TMP/resolved.json" --known "$TMP/known2.txt" 2>&1)"
code=$?
set -e
echo "$out" | grep -q '해결됨' || { echo "② 해결 안내 없음"; echo "$out"; exit 1; }
echo "$out" | grep -q 'known_failures.txt 에서 지우세요' || { echo "② 지우세요 안내 없음"; echo "$out"; exit 1; }
echo "$out" | grep -q '통과 1 / 실패 0 (기존 0 · 신규 0 · 해결 1)' || { echo "② 요약 줄 불일치: $out"; exit 1; }
[[ $code -eq 0 ]] || { echo "② 종료 코드 $code"; exit 1; }

# ③ 만료일 지난 항목은 실패
{
  json_fail internal/handler TestExpired
} >"$TMP/exp.json"
echo "internal/handler::TestExpired  # leftover / 95-93 / 2020-01-01" >"$TMP/known3.txt"
set +e
out="$(TEST_GATE_TODAY=2026-11-03 "$GATE" --json "$TMP/exp.json" --known "$TMP/known3.txt" 2>&1)"
code=$?
set -e
echo "$out" | grep -q '만료' || { echo "③ 만료 안내 없음"; echo "$out"; exit 1; }
echo "$out" | grep -q '통과 0 / 실패 1 (기존 1 · 신규 0 · 해결 0)' || { echo "③ 요약 줄 불일치: $out"; exit 1; }
[[ $code -eq 1 ]] || { echo "③ 종료 코드 $code"; exit 1; }

# ④ 요약 줄 형식
echo "$out" | grep -Eq '^통과 [0-9]+ / 실패 [0-9]+ \(기존 [0-9]+ · 신규 [0-9]+ · 해결 [0-9]+\)$' \
  || { echo "④ 요약 줄 형식 불일치"; exit 1; }

echo "test_gate selftest ok"
