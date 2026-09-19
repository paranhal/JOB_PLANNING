package repository

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// applySalesTaskType 영업활동 업무 구분·완료 이관, 한 활동이 업무 둘을 만들 수 있게 색인을 연다. 42-C
func applySalesTaskType(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`ALTER TABLE work_tasks ADD COLUMN source_role TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("052 work_tasks.source_role: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_work_tasks_source`); err != nil {
		log.Printf("052 drop idx_work_tasks_source: %v", err)
	}
	if _, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_work_tasks_source_role
			ON work_tasks(source_type, source_id, source_role)
			WHERE source_type IS NOT NULL AND source_type != ''`); err != nil {
		log.Printf("052 idx_work_tasks_source_role: %v", err)
	}

	res1, err := db.Exec(`
		UPDATE work_tasks
		   SET work_type = 'sales'
		 WHERE source_type = 'sales_activity'
		   AND work_type = 'admin'`)
	if err != nil {
		log.Printf("052 영업활동 구분 이관: %v", err)
		return
	}
	n1, _ := res1.RowsAffected()

	if _, err := db.Exec(`
		UPDATE work_tasks
		   SET source_role = 'done'
		 WHERE source_type = 'sales_activity'
		   AND TRIM(COALESCE(source_role,'')) = ''`); err != nil {
		log.Printf("052 source_role done: %v", err)
	}

	res2, err := db.Exec(`
		UPDATE work_tasks
		   SET status = 'complete',
		       progress = 100,
		       complete_date = work_date
		 WHERE source_type = 'sales_activity'
		   AND status = 'waiting'
		   AND progress = 0
		   AND IFNULL(work_date,'') <> ''
		   AND work_date <= date('now','localtime')`)
	if err != nil {
		log.Printf("052 영업활동 완료 이관: %v", err)
		return
	}
	n2, _ := res2.RowsAffected()
	log.Printf("영업활동 업무 이관: 구분 %d건 · 완료 %d건", n1, n2)
	if n1 > 0 || n2 > 0 {
		recordSalesTaskMigrateNote(db, n1, n2)
	}
}

func recordSalesTaskMigrateNote(db *sql.DB, n1, n2 int64) {
	if db == nil {
		return
	}
	note := time.Now().Format("2006-01-02") +
		": 영업활동 업무 이관 구분 " + strconv.FormatInt(n1, 10) + "건 · 완료 " + strconv.FormatInt(n2, 10) + "건"
	id := "AV-" + uuid.New().String()[:12]
	now := time.Now().UTC().Format("2006-01-02 15:04")
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO app_versions(
			version_id, version, "commit", built_at, first_started_at, last_started_at,
			start_count, schema_version, host, note)
		VALUES (?,?,?,?,?,?,1,?,?,?)`,
		id, "42-C", "sales-task-migrate", "42-C", now, now, AppSchemaVersion, "", note); err != nil {
		log.Printf("052 배포 이력: %v", err)
	}
}
