#!/bin/sh
# 운영 DB 백업. app.db · app.db-wal · app.db-shm 를 한 묶음으로 뜬다.
# 쓰는 법:  ./backup-db.sh              평소 백업
#           ./backup-db.sh 배포전        이름에 꼬리표를 붙인다
#
# 바꾸고 싶으면 환경변수로:
#   DATA_DIR=/home/sys2/deploy/data  BACKUP_DIR=/home/sys2/backup  KEEP=14
set -e

DATA_DIR=${DATA_DIR:-/home/sys2/deploy/data}
BACKUP_DIR=${BACKUP_DIR:-/home/sys2/backup}
KEEP=${KEEP:-14}
TAG=$1

DB="$DATA_DIR/app.db"
STAMP=$(date '+%Y%m%d-%H%M')
NAME="app-$STAMP"
[ -n "$TAG" ] && NAME="app-$STAMP-$TAG"

echo "== 1/5 확인 =="
if [ ! -f "$DB" ]; then
  echo "!! $DB 가 없습니다. DATA_DIR 를 확인하세요."
  echo "   예: DATA_DIR=/경로/data ./backup-db.sh"
  exit 1
fi
mkdir -p "$BACKUP_DIR"

# 남은 디스크가 DB 크기의 3배는 돼야 한다
DB_KB=$(du -sk "$DATA_DIR" | cut -f1)
FREE_KB=$(df -Pk "$BACKUP_DIR" | awk 'NR==2 {print $4}')
echo "   데이터 $((DB_KB / 1024))MB · 백업 폴더 여유 $((FREE_KB / 1024))MB"
if [ "$FREE_KB" -lt $((DB_KB * 3)) ]; then
  echo "!! 디스크 여유가 부족합니다. 오래된 백업을 지우고 다시 하세요."
  exit 1
fi

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM

echo "== 2/5 복사 =="
# 세 파일을 그대로 뜬다. 셋이 한 세트다.
cp -p "$DB" "$WORK/app.db"
[ -f "$DB-wal" ] && cp -p "$DB-wal" "$WORK/app.db-wal"
[ -f "$DB-shm" ] && cp -p "$DB-shm" "$WORK/app.db-shm"
ls -l "$WORK" | sed 's/^/   /'

echo "== 3/5 점검 =="
if command -v sqlite3 >/dev/null 2>&1; then
  # 앱이 켜져 있어도 안전한 방법으로 한 파일짜리 사본을 하나 더 뜬다(WAL 내용 포함).
  if sqlite3 "$DB" ".backup '$WORK/app-consistent.db'" 2>/dev/null; then
    echo "   정합 사본 만들었습니다 (app-consistent.db)"
  else
    echo "   !! 정합 사본 실패 — 파일 복사본만 담습니다"
  fi
  CHK=$(sqlite3 "$WORK/app.db" "PRAGMA integrity_check;" 2>/dev/null | head -1)
  echo "   무결성: ${CHK:-확인 못함}"
  if [ "$CHK" != "ok" ] && [ -n "$CHK" ]; then
    echo "   !! 무결성 경고. 백업은 남기되 원인을 확인하세요."
  fi
  sqlite3 "$WORK/app.db" "
    SELECT 'AS 접수 ' || (SELECT COUNT(*) FROM as_receipts)
        || ' · 영업 사업 ' || (SELECT COUNT(*) FROM sales_projects)
        || ' · 견적 ' || (SELECT COUNT(*) FROM sales_quotes)
        || ' · 업무 ' || (SELECT COUNT(*) FROM work_tasks);" 2>/dev/null | sed 's/^/   /'
else
  echo "   sqlite3 가 없어 점검을 건너뜁니다 (sudo apt install -y sqlite3)"
fi

echo "== 4/5 묶기 =="
tar czf "$BACKUP_DIR/$NAME.tar.gz" -C "$WORK" .
SIZE=$(du -h "$BACKUP_DIR/$NAME.tar.gz" | cut -f1)
echo "   $BACKUP_DIR/$NAME.tar.gz ($SIZE)"

echo "== 5/5 오래된 백업 정리 (최근 $KEEP개만 남김) =="
ls -1t "$BACKUP_DIR"/app-*.tar.gz 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r OLD; do
  echo "   지움: $(basename "$OLD")"
  rm -f "$OLD"
done

echo
echo "끝났습니다. 남아 있는 백업:"
ls -lht "$BACKUP_DIR"/app-*.tar.gz 2>/dev/null | head -5 | sed 's/^/   /'
echo
echo "되돌리는 법 (앱을 멈춘 뒤에):"
echo "   cd $(dirname "$DATA_DIR") && docker compose down"
echo "   tar xzf $BACKUP_DIR/$NAME.tar.gz -C $DATA_DIR"
echo "   docker compose up -d"
