package repository

import (
	"database/sql"
	"log"
)

// applyQueryPlanIndexes 마이그레이션 049. 화면 조회가 큰 표를 훑거나 임시 인덱스를 만들지 않게 한다. §44.7
func applyQueryPlanIndexes(db *sql.DB) {
	if db == nil {
		return
	}
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_as_processes_as_id ON as_processes(as_id)`,
		`CREATE INDEX IF NOT EXISTS idx_as_receipts_customer_id ON as_receipts(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_customer_id ON assets(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_rooms_floor_id ON customer_rooms(floor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_floors_bld_id ON customer_floors(building_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_buildings_cust ON customer_buildings(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_ref ON attachments(ref_type, ref_id)`,
		`CREATE INDEX IF NOT EXISTS idx_as_receipts_asset_id ON as_receipts(asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_as_receipts_receipt_dt ON as_receipts(receipt_datetime)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_customer_id ON contacts(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_history_contact ON contact_history(contact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_maint_visits_customer ON maintenance_visits(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_as_work_items_as_id ON as_work_items(as_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customers_parent ON customers(parent_customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_work_tasks_project_id ON work_tasks(project_id)`,
		// status 는 99.2% 가 completed 라 통짜 인덱스는 무의미하다. 미완료만 담는다. §44.7
		`CREATE INDEX IF NOT EXISTS idx_as_receipts_open
			ON as_receipts(status, receipt_datetime)
			WHERE status NOT IN ('completed','closed','cancelled')`,
		// ux_mnt_visit_unique 와 정의가 같다. 쓰기마다 두 번 갱신한다. §44.7
		`DROP INDEX IF EXISTS idx_maintenance_visit_dedup2`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			log.Printf("049 query plan index: %v", err)
		}
	}
	dropDuplicateBackupFolderIndex(db)
	if _, err := db.Exec(`ANALYZE`); err != nil {
		log.Printf("049 ANALYZE: %v", err)
	}
}

// dropDuplicateBackupFolderIndex folder_name UNIQUE 제약이 이미 인덱스를 만들면 중복 인덱스를 뗀다.
// 옛 DB는 CREATE TABLE IF NOT EXISTS 가 UNIQUE 를 안 붙이므로, 그때는 인덱스를 남긴다.
func dropDuplicateBackupFolderIndex(db *sql.DB) {
	rows, err := db.Query(`PRAGMA index_list('data_backups')`)
	if err != nil {
		log.Printf("049 data_backups index_list: %v", err)
		return
	}
	defer rows.Close()
	hasTableUnique := false
	for rows.Next() {
		var seq, uniq, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &uniq, &origin, &partial); err != nil {
			log.Printf("049 data_backups index_list scan: %v", err)
			return
		}
		if uniq == 1 && origin == "u" && name != "idx_data_backups_folder" {
			hasTableUnique = true
		}
	}
	if !hasTableUnique {
		return
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_data_backups_folder`); err != nil {
		log.Printf("049 drop idx_data_backups_folder: %v", err)
	}
}
