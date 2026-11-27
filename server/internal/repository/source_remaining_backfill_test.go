package repository

import (
	"path/filepath"
	"testing"
)

// 95-231 마이그레이션 121: 금산·당진 → WPSEED01, 사업 무관 고객 15건 업무 → 사업 부문. 비어 있는 것만, 한 번만, 전후 기록.
func TestSourceRemainingBackfill231(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "r231.db"))
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
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O01','O01','도서관','1')`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','금산도서관','금산도서관',1,'O01'),('C2','비젼아이티','비젼아이티',1,'O01')`,
		`INSERT OR IGNORE INTO work_projects(project_id,name,status,org_id) VALUES ('WPSEED01','충남 도서관','active','O01'),('WPX','다른','active','O01')`,
		`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, org_id, created_at, updated_at) VALUES
			('G1','R2611-108','C1','2026-11-12 10:00:00','completed','O01',datetime('now'),datetime('now')),
			('G2','R2611-109','C1','2026-11-12 11:00:00','completed','O01',datetime('now'),datetime('now')),
			('B1','R2607-062','C2','2026-07-12 10:00:00','completed','O01',datetime('now'),datetime('now')),
			('B2','R2607-063','C2','2026-07-12 11:00:00','completed','O01',datetime('now'),datetime('now')),
			('B3','R2607-064','C2','2026-07-12 12:00:00','completed','O01',datetime('now'),datetime('now'))`,
		`UPDATE as_receipts SET project_id='WPX' WHERE as_id='G2'`,
	} {
		ex(q)
	}
	task := func(id, src string, activity string) {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,source_type,source_id,activity_kind) VALUES (?,'admin',?, 'complete','O01','as',?,NULLIF(?,''))`, id, "업무 "+id, src, activity)
	}
	task("TG1", "G1", "")
	task("TG2", "G2", "")
	task("TB1", "B1", "")
	task("TB2", "B2", "mgmt") // 이미 구분이 있음 → 덮어쓰지 않음
	task("TB3", "B3", "")     // 대상 번호가 아님(R2607-064)
	ex(`DELETE FROM app_settings WHERE setting_key=?`, settingMigration121Done)
	applySourceRemainingBackfillV121(db)
	get := func(q string) string {
		var s string
		_ = db.QueryRow(q).Scan(&s)
		return s
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='G1'`); got != "WPSEED01" {
		t.Fatalf("금산 접수 %q", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id='TG1'`); got != "WPSEED01" {
		t.Fatalf("금산 업무 %q", got)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='G2'`); got != "WPX" {
		t.Fatalf("이미 사업이 있는 접수를 덮어썼다 %q", got)
	}
	if got := get(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id='TB1'`); got != "service" {
		t.Fatalf("비젼아이티 업무 구분 %q", got)
	}
	if got := get(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id='TB2'`); got != "mgmt" {
		t.Fatalf("이미 구분이 있는 업무를 덮어썼다 %q", got)
	}
	if got := get(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id='TB3'`); got != "" {
		t.Fatalf("대상이 아닌 접수의 업무를 바꿨다 %q", got)
	}
	if got := get(`SELECT COUNT(*) FROM source_remaining_backfill_121`); got != "3" {
		t.Fatalf("전후 기록 %s건 want 3(접수 사업 1 · 업무 사업 1 · 업무 구분 1)", got)
	}
	// 한 번만
	ex(`UPDATE work_tasks SET activity_kind=NULL WHERE task_id='TB1'`)
	applySourceRemainingBackfillV121(db)
	if got := get(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id='TB1'`); got != "" {
		t.Fatalf("두 번째 실행이 다시 채웠다 %q", got)
	}
}
