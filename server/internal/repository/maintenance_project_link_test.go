package repository

import (
	"path/filepath"
	"testing"
)

func TestSetVisitProjectOnlyTouchesProject(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "mntproj.db"))
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
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES ('MP1', 2026, '계획', 'approved', 'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, completed_date, notes)
		VALUES ('V1','MP1','2026-08-12','C1',0,'normal','김기술','KLAS',1,'2026-08-12','비고')`); err != nil {
		t.Fatal(err)
	}

	repo := NewMaintenanceRepo(db)
	before, after, err := repo.SetVisitProject("O03", "V1", "WP1")
	if err != nil {
		t.Fatal(err)
	}
	if before != "" || after != "사업A" {
		t.Fatalf("before=%q after=%q", before, after)
	}
	var pid, date, cust, asg, notes, doneDate string
	var done int
	if err := db.QueryRow(`SELECT COALESCE(project_id,''), visit_date, customer_id, COALESCE(assignee,''),
		COALESCE(completed,0), COALESCE(completed_date,''), COALESCE(notes,'')
		FROM maintenance_visits WHERE visit_id='V1'`).Scan(&pid, &date, &cust, &asg, &done, &doneDate, &notes); err != nil {
		t.Fatal(err)
	}
	if pid != "WP1" {
		t.Fatalf("project_id=%s", pid)
	}
	if date != "2026-08-12" || cust != "C1" || asg != "김기술" || done != 1 || doneDate != "2026-08-12" || notes != "비고" {
		t.Fatalf("다른 칸이 바뀜 date=%s cust=%s asg=%s done=%d doneDate=%s notes=%s", date, cust, asg, done, doneDate, notes)
	}
	if _, _, err := repo.SetVisitProject("O01", "V1", "WP1"); err == nil {
		t.Fatal("타 조직은 실패해야 한다")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='maintenance_visits' AND entity_id='V1' AND action='update'`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("이력 n=%d %v", n, err)
	}
	var tasks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='maintenance' AND source_id='V1'`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("일일업무를 건드렸다 n=%d", tasks)
	}
}
