package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyAS34ActionUX 마이그레이션 027. §34.3.1~§34.3.4
func applyAS34ActionUX(db *sql.DB) {
	for _, col := range []string{"cause_cat1", "cause_cat2", "cause_cat3"} {
		q := `ALTER TABLE as_receipts ADD COLUMN ` + col + ` TEXT`
		if _, err := db.Exec(q); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("027 as_receipts.%s: %v", col, err)
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS as_cause_categories (
			code        TEXT PRIMARY KEY,
			level       INTEGER NOT NULL,
			parent_code TEXT,
			label       TEXT NOT NULL,
			sort_order  INTEGER DEFAULT 0,
			is_active   INTEGER DEFAULT 1
		)`); err != nil {
		log.Printf("027 as_cause_categories: %v", err)
	}
	db.Exec(`INSERT OR IGNORE INTO as_cause_categories (code, level, parent_code, label, sort_order, is_active) VALUES
		('rfid',1,NULL,'RFID',1,1),
		('klas',1,NULL,'KLAS',2,1),
		('web',1,NULL,'홈페이지',3,1),
		('server',1,NULL,'서버',4,1)`)

	db.Exec(`UPDATE as_receipts SET process_type='visit' WHERE process_type IN ('replace','config')`)
	db.Exec(`UPDATE as_processes SET work_type='visit' WHERE work_type IN ('replace','config')`)
	db.Exec(`UPDATE codes SET is_active=0 WHERE code_group='process_type' AND code_value IN ('replace','config')`)
	db.Exec(`UPDATE codes SET is_active=0 WHERE code_group='result_code' AND code_value IN ('revisit_needed','temporary','escalation')`)
}
