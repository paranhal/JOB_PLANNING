package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyASReceiptsCauseReport 마이그레이션 007. 장애원인·결론 서술. §12.10.4
// cause_type 은 건드리지 않는다.
func applyASReceiptsCauseReport(db *sql.DB) {
	for _, col := range []string{"cause_detail", "conclusion"} {
		q := `ALTER TABLE as_receipts ADD COLUMN ` + col + ` TEXT`
		if _, err := db.Exec(q); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("007 as_receipts.%s: %v", col, err)
		}
	}
}
