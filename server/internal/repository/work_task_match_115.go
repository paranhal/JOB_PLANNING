package repository

import (
	"context"
	"database/sql"
	"log"
	"time"
)

const settingMigration115Done = "migration_115_done"

// applyWorkTaskMatchV115 95-224: 원본 기반 업무의 사업·업무 구분 일괄 매칭. 딱 한 번만 실행한다.
// 사용자가 이후 사업을 비우거나 바꿔도 기동할 때마다 다시 덮어쓰지 않도록 app_settings 에 완료를 기록한다.
func applyWorkTaskMatchV115(db *sql.DB) {
	if db == nil {
		return
	}
	var done string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingMigration115Done).Scan(&done)
	if done != "" {
		return
	}
	up, err := leaveLedgerSQL("115_work_task_project_match.sql")
	if err != nil {
		log.Printf("115 work_task_match: %v", err)
		return
	}
	// 임시 테이블(TEMP)은 연결 단위라 한 연결에서 끝까지 실행한다.
	conn, err := db.Conn(context.Background())
	if err != nil {
		log.Printf("115 work_task_match conn: %v", err)
		return
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), up); err != nil {
		log.Printf("115 work_task_match apply: %v", err)
		return
	}
	var n, proj, act int
	_ = db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(after_project IS NOT NULL),0), COALESCE(SUM(after_activity IS NOT NULL),0) FROM work_task_match_115`).Scan(&n, &proj, &act)
	_, _ = db.Exec(`INSERT OR REPLACE INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)`,
		settingMigration115Done, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	log.Printf("115 work_task_match: 업무 %d건 반영(사업 %d · 업무 구분 %d)", n, proj, act)
}
