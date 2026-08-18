package repository

import (
	"database/sql"
	"log"
	"strings"
)

// 행정/지원 완료일·접수일 SQL. 별칭 t. (§13.4 · §13.10 · §15.3)
// complete_date 가 비면 종전 대리지표 updated_at. receipt_date 가 비면 created_at.
const (
	adminTaskCompleteDateSQL = `date(COALESCE(NULLIF(TRIM(t.complete_date),''), t.updated_at))`
	adminTaskReceiptDateSQL  = `date(COALESCE(NULLIF(TRIM(t.receipt_date),''), t.created_at))`
)

// applyS11WorkTaskDates 마이그레이션 004. 컬럼 추가·기존 행 보정.
func applyS11WorkTaskDates(db *sql.DB) {
	for _, q := range []string{
		`ALTER TABLE work_tasks ADD COLUMN receipt_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN complete_date TEXT`,
	} {
		if _, err := db.Exec(q); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("S-11 column: %v", err)
		}
	}
	if _, err := db.Exec(`
UPDATE work_tasks
   SET receipt_date = date(created_at)
 WHERE TRIM(COALESCE(receipt_date,'')) = ''`); err != nil {
		log.Printf("S-11 receipt_date backfill: %v", err)
	}
	if _, err := db.Exec(`
UPDATE work_tasks
   SET complete_date = date(updated_at)
 WHERE status = 'complete'
   AND TRIM(COALESCE(complete_date,'')) = ''`); err != nil {
		log.Printf("S-11 complete_date backfill: %v", err)
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_work_tasks_receipt_date ON work_tasks(receipt_date)`,
		`CREATE INDEX IF NOT EXISTS idx_work_tasks_complete_date ON work_tasks(complete_date)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("S-11 index: %v", err)
		}
	}
}
