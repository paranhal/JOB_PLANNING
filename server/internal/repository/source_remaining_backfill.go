package repository

import (
	"database/sql"
	"log"
	"time"
)

const settingMigration121Done = "migration_121_done"

// applySourceRemainingBackfillV121 95-231: 남은 사업 미연결 접수 정리(금산·당진 WPSEED01 연결, 사업 무관 고객 15건 업무 구분 지정). 한 번만 실행한다.
func applySourceRemainingBackfillV121(db *sql.DB) {
	if db == nil {
		return
	}
	var done string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingMigration121Done).Scan(&done)
	if done != "" {
		return
	}
	up, err := leaveLedgerSQL("121_remaining_source_projects.sql")
	if err != nil {
		log.Printf("121 remaining_source_projects: %v", err)
		return
	}
	if _, err := db.Exec(up); err != nil {
		log.Printf("121 remaining_source_projects apply: %v", err)
		return
	}
	var rp, tp, ta int
	_ = db.QueryRow(`SELECT COALESCE(SUM(kind='receipt_project'),0), COALESCE(SUM(kind='task_project'),0), COALESCE(SUM(kind='task_activity'),0) FROM source_remaining_backfill_121`).Scan(&rp, &tp, &ta)
	now := time.Now().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT OR REPLACE INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)`, settingMigration121Done, now, now)
	log.Printf("121 remaining_source_projects: 접수 사업 %d · 업무 사업 %d · 업무 구분 %d건", rp, tp, ta)
}
