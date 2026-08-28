package repository

import (
	"database/sql"
	"log"
)

const workRecurrenceMetaKey = "__meta:work_recurrence_v1"

const workRecurrenceSchema = `
CREATE TABLE IF NOT EXISTS work_recurrence (
  task_id                  TEXT PRIMARY KEY,
  start_date               TEXT NOT NULL DEFAULT '',
  end_date                 TEXT NOT NULL DEFAULT '',
  rule_type                TEXT NOT NULL DEFAULT 'none',
  interval_n               INTEGER NOT NULL DEFAULT 1,
  weekdays                 TEXT NOT NULL DEFAULT '',
  holiday_policy           TEXT NOT NULL DEFAULT 'as_is',
  complete_policy          TEXT NOT NULL DEFAULT 'manual',
  progress_include_future  INTEGER NOT NULL DEFAULT 0,
  final_result             TEXT NOT NULL DEFAULT '',
  created_at               DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at               DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// applyWorkRecurrence 마이그레이션 017. §13.15 — 실행 작업은 work_tasks 하위행이다.
func applyWorkRecurrence(db *sql.DB) {
	if db == nil {
		return
	}
	for _, col := range []string{"recurrence_role", "occurrence_seq", "occurrence_status", "not_done_reason"} {
		if workTaskHasColumn(db, col) {
			continue
		}
		typ := "TEXT"
		if col == "occurrence_seq" {
			typ = "INTEGER"
		}
		if _, err := db.Exec(`ALTER TABLE work_tasks ADD COLUMN ` + col + ` ` + typ); err != nil {
			log.Printf("017 work_tasks.%s: %v", col, err)
		}
	}
	if _, err := db.Exec(workRecurrenceSchema); err != nil {
		log.Printf("017 work_recurrence: %v", err)
		return
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_tasks_recurrence_parent
		ON work_tasks(parent_task_id, recurrence_role)`); err != nil {
		log.Printf("017 idx_work_tasks_recurrence_parent: %v", err)
	}
	if !workRecurrenceHasColumn(db, "archived") {
		if _, err := db.Exec(`ALTER TABLE work_recurrence ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`); err != nil {
			log.Printf("018 work_recurrence.archived: %v", err)
		}
	}
	for _, col := range []struct {
		name string
		def  string
	}{
		{"month_day", "INTEGER NOT NULL DEFAULT 0"},
		{"month_n", "INTEGER NOT NULL DEFAULT 0"},
		{"last_workday", "INTEGER NOT NULL DEFAULT 0"},
		{"manual_dates", "TEXT NOT NULL DEFAULT ''"},
	} {
		if workRecurrenceHasColumn(db, col.name) {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE work_recurrence ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
			log.Printf("023 work_recurrence.%s: %v", col.name, err)
		}
	}
	markMetaDone(db, workRecurrenceMetaKey)
}

func workRecurrenceHasColumn(db *sql.DB, name string) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_recurrence') WHERE name=?`, name).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func workTaskHasColumn(db *sql.DB, name string) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name=?`, name).Scan(&n); err != nil {
		return false
	}
	return n > 0
}
