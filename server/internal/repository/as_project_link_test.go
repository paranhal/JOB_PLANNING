package repository

import (
	"path/filepath"
	"testing"
)

func TestSetASReceiptProjectOrgAndHistory(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "asproj.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O03','O03','SI',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C1','학교','학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, org_id, created_at, updated_at)
		VALUES ('AS1','AS1','C1','2026-08-12 10:00:00','in_progress','O03', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	as := NewASRepo(db)
	before, after, err := as.SetASReceiptProject("O03", "AS1", "WP1")
	if err != nil {
		t.Fatal(err)
	}
	if before != "" || after != "사업A" {
		t.Fatalf("before=%q after=%q", before, after)
	}
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='AS1'`).Scan(&pid); err != nil || pid != "WP1" {
		t.Fatalf("pid=%s %v", pid, err)
	}
	if _, _, err := as.SetASReceiptProject("O03", "AS1", "WP1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := as.SetASReceiptProject("O01", "AS1", "WP1"); err == nil {
		t.Fatal("타 조직은 실패해야 한다")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id='AS1' AND action='update'`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("이력 n=%d %v", n, err)
	}
}
