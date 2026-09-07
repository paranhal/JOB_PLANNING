package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyAS34TransferFollowup 마이그레이션 028. §34.3.5 이관후속 추가 접수 내용.
func applyAS34TransferFollowup(db *sql.DB) {
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN followup_note TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("028 as_receipts.followup_note: %v", err)
	}
}
