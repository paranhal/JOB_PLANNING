package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applySalesMemos(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sales_memos (
  memo_id     TEXT PRIMARY KEY,
  sales_id    TEXT NOT NULL,
  content     TEXT NOT NULL,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  author_id   TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		log.Printf("054 sales_memos: %v", err)
		return
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sales_memos_sales ON sales_memos(sales_id, sort_order)`); err != nil {
		log.Printf("054 sales_memos index: %v", err)
	}
}

func applyWorkProjectContract(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`ALTER TABLE work_projects ADD COLUMN contract_amount INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE work_projects ADD COLUMN contract_no TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(q); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("055 work_projects contract: %v", err)
		}
	}
}
