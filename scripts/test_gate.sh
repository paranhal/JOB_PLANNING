#!/usr/bin/env bash
# 95-93: go test 실패를 known_failures.txt 와 비교한다.
# 새 실패·만료 항목이 있으면 종료 코드 1.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${TEST_GATE_SERVER:-$ROOT/server}"
KNOWN="${TEST_GATE_KNOWN:-$SERVER/testdata/known_failures.txt}"
JSON_FILE="${TEST_GATE_JSON:-}"
TODAY="${TEST_GATE_TODAY:-$(date +%F)}"

usage() {
  cat <<'EOF'
usage: scripts/test_gate.sh [--json FILE] [--known FILE] [--server DIR]
  기본: cd server && go test ./... -count=1 -json
  --json  이 있으면 go test 를 돌리지 않고 해당 -json 로그를 파싱한다.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json) JSON_FILE="$2"; shift 2 ;;
    --known) KNOWN="$2"; shift 2 ;;
    --server) SERVER="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
json_path="${JSON_FILE}"
if [[ -z "$json_path" ]]; then
  json_path="$tmp/gotest.json"
  (
    cd "$SERVER"
    go test ./... -count=1 -json -timeout 600s
  ) >"$json_path" || true
fi

export TEST_GATE_JSON_PATH="$json_path"
export TEST_GATE_KNOWN_PATH="$KNOWN"
export TEST_GATE_TODAY="$TODAY"
python3 - <<'PY'
import json, os, re, sys

json_path = os.environ["TEST_GATE_JSON_PATH"]
known_path = os.environ["TEST_GATE_KNOWN_PATH"]
today = os.environ["TEST_GATE_TODAY"]

def short_pkg(p):
    if p.startswith("customer-support/"):
        return p[len("customer-support/"):]
    return p

fails = set()
pass_n = 0
with open(json_path, encoding="utf-8", errors="replace") as f:
    for line in f:
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            o = json.loads(line)
        except json.JSONDecodeError:
            continue
        test = (o.get("Test") or "").strip()
        if not test:
            continue
        act = o.get("Action")
        pkg = short_pkg(o.get("Package") or "")
        key = f"{pkg}::{test}"
        if act == "fail":
            fails.add(key)
        elif act == "pass":
            pass_n += 1

known = {}
try:
    lines = open(known_path, encoding="utf-8").read().splitlines()
except FileNotFoundError:
    lines = []
for raw in lines:
    s = raw.strip()
    if not s or s.startswith("#"):
        continue
    key, _, rest = s.partition(" ")
    if "::" not in key:
        continue
    dates = re.findall(r"\d{4}-\d{2}-\d{2}", rest)
    known[key] = {"reason": rest.strip(), "expiry": dates[-1] if dates else ""}

new_keys = sorted(k for k in fails if k not in known)
known_fail = sorted(k for k in fails if k in known)
resolved = sorted(k for k in known if k not in fails)
expired = sorted(k for k in known_fail if known[k]["expiry"] and known[k]["expiry"] < today)

if new_keys:
    print(f"신규 실패 {len(new_keys)}건 (known_failures.txt 에 없음):")
    for k in new_keys:
        print(f"  {k}")
if resolved:
    print(f"해결됨 {len(resolved)}건 — known_failures.txt 에서 지우세요:")
    for k in resolved:
        print(f"  {k}")
if expired:
    print(f"만료 {len(expired)}건 (만료일이 {today} 이전):")
    for k in expired:
        print(f"  {k}  {known[k]['reason']}")

print(f"통과 {pass_n} / 실패 {len(fails)} (기존 {len(known_fail)} · 신규 {len(new_keys)} · 해결 {len(resolved)})")
if new_keys or expired:
    sys.exit(1)
sys.exit(0)
PY
