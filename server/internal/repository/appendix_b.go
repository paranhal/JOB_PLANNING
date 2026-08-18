package repository

import (
	"database/sql"
	"log"
	"strings"
)

const (
	appendixBB1Expected = 1124
	appendixBB2Expected = 1128
	appendixBB3Expected = 1124
)

// applyAppendixB 기획서 부록 B 전체. SQL 문법은 부록 그대로이며
// 보정 순서는 B-3 → B-2 → B-1 → 트리거 → UNIQUE 이다.
func applyAppendixB(db *sql.DB) {
	// B.1 컬럼 추가 (이미 있으면 SQLite가 오류 → 무시)
	for _, q := range []string{
		`ALTER TABLE as_receipts        ADD COLUMN data_origin TEXT NOT NULL DEFAULT 'app'`,
		`ALTER TABLE as_receipts        ADD COLUMN schedule_no_date_reason TEXT`,
		`ALTER TABLE as_receipts        ADD COLUMN asset_unlinked_reason   TEXT`,
		`ALTER TABLE maintenance_visits ADD COLUMN project_id  TEXT`,
		`ALTER TABLE maintenance_visits ADD COLUMN data_origin TEXT NOT NULL DEFAULT 'app'`,
	} {
		if _, err := db.Exec(q); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("appendix B.1 skip: %v", err)
		}
	}

	// B.2 휴무일 테이블
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS holidays (
  holiday_date TEXT PRIMARY KEY,
  holiday_year INTEGER NOT NULL,
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'public',
  source       TEXT,
  synced_at    DATETIME,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		log.Printf("appendix B.2 holidays: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_holidays_year ON holidays(holiday_year)`); err != nil {
		log.Printf("appendix B.2 holidays index: %v", err)
	}

	// B.5 보정: B-3 → B-2 → B-1
	n3 := execRows(db, `
UPDATE as_receipts SET data_origin = 'import'
 WHERE TRIM(COALESCE(import_key,'')) <> ''`)
	log.Printf("appendix B-3 data_origin=import: expected=%d actual=%d match=%v",
		appendixBB3Expected, n3, n3 == appendixBB3Expected)

	n2 := execRows(db, `
UPDATE as_receipts SET start_datetime = (
        SELECT MIN(p.process_datetime) FROM as_processes p WHERE p.as_id = as_receipts.as_id)
 WHERE TRIM(COALESCE(start_datetime,'')) = ''
   AND EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = as_receipts.as_id)`)
	log.Printf("appendix B-2 start_datetime backfill: expected=%d actual=%d match=%v",
		appendixBB2Expected, n2, n2 == appendixBB2Expected)

	n1 := execRows(db, `
UPDATE as_receipts SET schedule_confirmed = 0
 WHERE schedule_confirmed = 1
   AND TRIM(COALESCE(visit_scheduled_date,'')) = ''`)
	log.Printf("appendix B-1 unconfirm without date: expected=%d actual=%d match=%v",
		appendixBB1Expected, n1, n1 == appendixBB1Expected)

	// B.4 트리거 4종 (문법 변경 없음)
	for _, q := range []string{
		`DROP TRIGGER IF EXISTS trg_as_sched_confirm_ins`,
		`CREATE TRIGGER trg_as_sched_confirm_ins BEFORE INSERT ON as_receipts
FOR EACH ROW WHEN NEW.schedule_confirmed = 1
                 AND TRIM(COALESCE(NEW.visit_scheduled_date,'')) = ''
BEGIN SELECT RAISE(ABORT, '일정확정에는 예정업무일이 필요합니다'); END`,
		`DROP TRIGGER IF EXISTS trg_as_sched_confirm_upd`,
		`CREATE TRIGGER trg_as_sched_confirm_upd BEFORE UPDATE ON as_receipts
FOR EACH ROW WHEN NEW.schedule_confirmed = 1
                 AND TRIM(COALESCE(NEW.visit_scheduled_date,'')) = ''
BEGIN SELECT RAISE(ABORT, '일정확정에는 예정업무일이 필요합니다'); END`,
		`DROP TRIGGER IF EXISTS trg_proc_time_spent_ins`,
		`CREATE TRIGGER trg_proc_time_spent_ins BEFORE INSERT ON as_processes
FOR EACH ROW WHEN COALESCE(NEW.time_spent, 0) <= 0
BEGIN SELECT RAISE(ABORT, '소요시간(분)은 0보다 커야 합니다'); END`,
		`DROP TRIGGER IF EXISTS trg_as_work_place_upd`,
		`CREATE TRIGGER trg_as_work_place_upd BEFORE UPDATE ON as_receipts
FOR EACH ROW WHEN TRIM(COALESCE(NEW.work_place,'')) <> ''
                 AND NEW.work_place NOT IN ('office','field')
BEGIN SELECT RAISE(ABORT, '근무구분은 office 또는 field 여야 합니다'); END`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("appendix B.4 trigger: %v", err)
		}
	}

	// B.3 UNIQUE 인덱스
	if _, err := db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS ux_mnt_visit_unique
  ON maintenance_visits(plan_id, visit_date, customer_id, COALESCE(product_type,''))`); err != nil {
		log.Printf("appendix B.3 unique index: %v", err)
	}
}

func execRows(db *sql.DB, q string) int64 {
	res, err := db.Exec(q)
	if err != nil {
		log.Printf("appendix B update: %v", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}
