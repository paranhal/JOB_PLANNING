package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyAS47VisitDate 마이그레이션 040. §4.7 방문일 · 처리유형 미정+사유.
func applyAS47VisitDate(db *sql.DB) {
	for _, col := range []string{"visit_date", "process_type_reason"} {
		if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN ` + col + ` TEXT`); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("040 as_receipts.%s: %v", col, err)
		}
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
		VALUES ('PRT006','process_type','undetermined','미정',6,1)`); err != nil {
		log.Printf("040 process_type undetermined: %v", err)
	}
}
