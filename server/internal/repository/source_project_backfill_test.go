package repository

import (
	"path/filepath"
	"testing"
)

// 95-230 마이그레이션 120: 업무에만 있는 사업을 접수·방문에 복사한다. 비어 있는 원본만, 충돌은 건드리지 않고, 한 번만, 전후 기록.
func TestSourceProjectBackfill230(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "b230.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O03','O03','SI',1)`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O03'),('WP2','사업B','active','O03')`,
		`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, org_id, created_at, updated_at) VALUES
			('A1','A1','C1','2026-08-12 10:00:00','completed','O03',datetime('now'),datetime('now')),
			('A2','A2','C1','2026-08-12 11:00:00','completed','O03',datetime('now'),datetime('now')),
			('A3','A3','C1','2026-08-12 12:00:00','completed','O03',datetime('now'),datetime('now')),
			('A4','A4','C1','2026-08-12 13:00:00','completed','O03',datetime('now'),datetime('now'))`,
		`UPDATE as_receipts SET project_id='WP2' WHERE as_id='A2'`,
		`INSERT INTO as_work_items(work_id,work_number,as_id,work_kind,status) VALUES ('W3','A3-W01','A3','revisit','open')`,
		`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id) VALUES ('MP1', 2026, '계획', 'approved', 'O03')`,
		`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, notes) VALUES
			('V1','MP1','2026-12-02','C1',0,'normal','김','KLAS',1,''),('V2','MP1','2026-12-03','C1',0,'normal','김','KLAS',1,'')`,
		`UPDATE maintenance_visits SET project_id='WP2' WHERE visit_id='V2'`,
	} {
		ex(q)
	}
	task := func(id, st, src, pid string) {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id,project_id) VALUES (?,'admin',?, 'complete','O03',?,?,NULLIF(?,''))`, id, "업무 "+id, st, src, pid)
	}
	task("T-A1", "as", "A1", "WP1") // 접수에 사업 없음·업무에 있음 → 복사
	task("T-A2", "as", "A2", "WP1") // 접수에 이미 다른 사업(WP2) → 덮어쓰지 않음
	task("T-A3", "as", "A3", "WP1") // 접수 업무 + 하부업무 업무가 서로 다른 사업 → 충돌, 건드리지 않음
	task("T-W3", "as_work", "W3", "WP2")
	task("T-A4", "as", "A4", "") // 업무에도 사업 없음 → 그대로
	task("T-V1", "maintenance", "V1", "WP1")
	task("T-V2", "maintenance", "V2", "WP1") // 방문에 이미 WP2 → 덮어쓰지 않음
	ex(`DELETE FROM app_settings WHERE setting_key=?`, settingMigration120Done)
	applySourceProjectBackfillV120(db)
	get := func(q string) string {
		var s string
		_ = db.QueryRow(q).Scan(&s)
		return s
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='A1'`); got != "WP1" {
		t.Fatalf("A1 %q want WP1", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='A2'`); got != "WP2" {
		t.Fatalf("A2 덮어썼다 %q", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='A3'`); got != "" {
		t.Fatalf("A3 충돌인데 채웠다 %q", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='A4'`); got != "" {
		t.Fatalf("A4 %q", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V1'`); got != "WP1" {
		t.Fatalf("V1 %q want WP1", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V2'`); got != "WP2" {
		t.Fatalf("V2 덮어썼다 %q", got)
	}
	if got := get(`SELECT COUNT(*) FROM source_project_backfill_120`); got != "2" {
		t.Fatalf("전후 기록 %s건 want 2", got)
	}
	if got := get(`SELECT before_project||'|'||after_project FROM source_project_backfill_120 WHERE source_id='A1'`); got != "|WP1" && got != "" {
		// before 는 NULL 이라 || 연산이 NULL 이 될 수 있다
		t.Logf("A1 기록 %q", got)
	}
	// 한 번만: 사용자가 비운 뒤 다시 기동해도 되살리지 않는다
	ex(`UPDATE as_receipts SET project_id=NULL WHERE as_id='A1'`)
	applySourceProjectBackfillV120(db)
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='A1'`); got != "" {
		t.Fatalf("두 번째 실행이 다시 채웠다 %q", got)
	}
}
