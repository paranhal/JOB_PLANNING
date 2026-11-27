package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

// 95-229 ③: 정기점검 업무의 사업을 바꾸면 방문에 먼저 쓰고, 다음 점검 동기화가 빈 사업으로 되돌리지 않는다.
func TestMntTaskProjectWriteThrough229(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m229.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O03','O03','SI',1)`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O03')`,
		`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id) VALUES ('MP1', 2026, '계획', 'approved', 'O03')`,
		`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, notes) VALUES ('V1','MP1','2026-12-02','C1',0,'normal','김기술','KLAS',0,'')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	wb := NewWBRepo(db)
	taskID, err := wb.EnsureMaintenanceTaskForVisit("V1")
	if err != nil || taskID == "" {
		t.Fatalf("점검 업무 %q %v", taskID, err)
	}
	get := func(q string, a ...interface{}) string {
		var s string
		_ = db.QueryRow(q, a...).Scan(&s)
		return s
	}
	if ok, err := wb.WriteThroughSourceProject(taskID, "WP1"); err != nil || !ok {
		t.Fatalf("write-through %v %v", ok, err)
	}
	if get(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V1'`) != "WP1" || get(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID) != "WP1" {
		t.Fatal("방문과 업무에 사업이 같이 들어가지 않았다")
	}
	// 다음 동기화가 와도 유지된다(방문이 원본)
	if _, err := wb.EnsureMaintenanceTasks(); err != nil {
		t.Fatal(err)
	}
	if got := get(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID); got != "WP1" {
		t.Fatalf("동기화 뒤 업무 사업 %q", got)
	}
	// 해제도 방문부터
	if _, err := wb.WriteThroughSourceProject(taskID, ""); err != nil {
		t.Fatal(err)
	}
	if get(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V1'`) != "" || get(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID) != "" {
		t.Fatal("해제가 방문·업무에 같이 반영되지 않았다")
	}
	_ = model.WBSourceMaintenance
}
