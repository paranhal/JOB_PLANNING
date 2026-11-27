package repository

import (
	"database/sql"
	"log"
	"time"
)

const settingMigration120Done = "migration_120_done"

// applySourceProjectBackfillV120 95-230: 업무에만 있는 사업을 접수·방문(원본)에 복사하는 보정. 딱 한 번만 실행한다.
// 사용자가 이후 사업을 비우거나 바꿔도 기동할 때마다 다시 덮어쓰지 않도록 app_settings 에 완료를 기록한다.
func applySourceProjectBackfillV120(db *sql.DB) {
	if db == nil {
		return
	}
	var done string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingMigration120Done).Scan(&done)
	if done != "" {
		return
	}
	up, err := leaveLedgerSQL("120_source_project_backfill.sql")
	if err != nil {
		log.Printf("120 source_project_backfill: %v", err)
		return
	}
	if _, err := db.Exec(up); err != nil {
		log.Printf("120 source_project_backfill apply: %v", err)
		return
	}
	var as, mnt int
	_ = db.QueryRow(`SELECT COALESCE(SUM(source_type='as'),0), COALESCE(SUM(source_type='maintenance'),0) FROM source_project_backfill_120`).Scan(&as, &mnt)
	now := time.Now().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT OR REPLACE INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)`, settingMigration120Done, now, now)
	log.Printf("120 source_project_backfill: 접수 %d건 · 방문 %d건에 업무의 사업을 복사", as, mnt)
}
