package repository

import (
	"database/sql"
	"log"
)

// applyStaffLeaves 마이그레이션 009. 연차 테이블. §23.13.3
func applyStaffLeaves(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS staff_leaves (
  leave_id    TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  leave_date  TEXT NOT NULL,
  leave_kind  TEXT NOT NULL DEFAULT 'annual',
  note        TEXT,
  created_by  TEXT,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, leave_date)
)`); err != nil {
		log.Printf("009 staff_leaves: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_staff_leaves_date ON staff_leaves(leave_date)`); err != nil {
		log.Printf("009 staff_leaves date index: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_staff_leaves_user ON staff_leaves(user_id)`); err != nil {
		log.Printf("009 staff_leaves user index: %v", err)
	}
}
