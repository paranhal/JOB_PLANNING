package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

// 95-227: 접수·방문의 사업을 업무로 복사 — 생성 시 · 원본 변경 시 · 업무에서 바꿀 때 · 기존 건 보정.

type env227 struct {
	db *sql.DB
	wb *WBRepo
	as *ASRepo
	mn *MaintenanceRepo
}

func setup227(t *testing.T) env227 {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "p227.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O03','O03','SI',1)`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O03'),('WP2','사업B','active','O03')`,
		`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, org_id, created_at, updated_at) VALUES
			('AS1','AS1','C1','2026-08-12 10:00:00','in_progress','O03',datetime('now'),datetime('now')),
			('AS2','AS2','C1','2026-08-12 11:00:00','in_progress','O03',datetime('now'),datetime('now')),
			('AS3','AS3','C1','2026-08-12 12:00:00','in_progress','O03',datetime('now'),datetime('now'))`,
		`INSERT INTO as_work_items(work_id,work_number,as_id,work_kind,scheduled_date,status) VALUES ('W1','AS1-W01','AS1','revisit','2026-08-13','open')`,
		`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id) VALUES ('MP1', 2026, '계획', 'approved', 'O03')`,
		`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, notes)
			VALUES ('V1','MP1','2026-08-12','C1',0,'normal','김기술','KLAS',0,'')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	return env227{db: db, wb: NewWBRepo(db), as: NewASRepo(db), mn: NewMaintenanceRepo(db)}
}

func (e env227) task(t *testing.T, title, srcType, srcID, project string) string {
	t.Helper()
	tk := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: title, Status: model.WBTaskWaiting, Priority: model.WBPriorityNormal,
		DueDate: "2026-08-12", WorkDate: "2026-08-12", OrgID: "O03", SourceType: srcType, SourceID: srcID, ProjectID: project}
	if err := e.wb.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	return tk.TaskID
}

func (e env227) project(t *testing.T, taskID string) string {
	t.Helper()
	var p string
	_ = e.db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&p)
	return p
}

func (e env227) exec(t *testing.T, q string, a ...interface{}) {
	t.Helper()
	if _, err := e.db.Exec(q, a...); err != nil {
		t.Fatal(q, err)
	}
}

// 생성: 접수·하부업무·방문에서 만든 업무가 원본의 사업을 가진다. 원본에 사업이 없으면 비어 있고, 직접 준 사업은 그대로다.
func TestWorkTaskCreateInheritsProject227(t *testing.T) {
	e := setup227(t)
	e.exec(t, `UPDATE as_receipts SET project_id='WP1' WHERE as_id='AS1'`)
	e.exec(t, `UPDATE maintenance_visits SET project_id='WP1' WHERE visit_id='V1'`)
	if got := e.project(t, e.task(t, "접수업무", model.WBSourceAS, "AS1", "")); got != "WP1" {
		t.Fatalf("접수 업무 사업 %q", got)
	}
	if got := e.project(t, e.task(t, "하부업무", model.WBSourceASWork, "W1", "")); got != "WP1" {
		t.Fatalf("하부업무 사업 %q", got)
	}
	if got := e.project(t, e.task(t, "점검업무", model.WBSourceMaintenance, "V1", "")); got != "WP1" {
		t.Fatalf("점검 업무 사업 %q", got)
	}
	if got := e.project(t, e.task(t, "사업없는접수", model.WBSourceAS, "AS2", "")); got != "" {
		t.Fatalf("원본에 사업이 없는데 %q", got)
	}
	if got := e.project(t, e.task(t, "직접지정", model.WBSourceAS, "AS3", "WP2")); got != "WP2" {
		t.Fatalf("직접 준 사업이 바뀌었다 %q", got)
	}
}

// 변경: 접수에서 사업을 지정·변경·해제하면 연결된 업무(하부업무 포함)에 반영, 다른 접수의 업무는 그대로, 활동 분류는 비운다, 멱등.
func TestWorkTaskFollowsReceiptProject227(t *testing.T) {
	e := setup227(t)
	a := e.task(t, "접수업무", model.WBSourceAS, "AS1", "")
	w := e.task(t, "하부업무", model.WBSourceASWork, "W1", "")
	other := e.task(t, "다른접수", model.WBSourceAS, "AS2", "")
	if _, _, err := e.wb.SetTaskActivity("O03", a, "mgmt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.as.SetASReceiptProject("O03", "AS1", "WP1"); err != nil {
		t.Fatal(err)
	}
	if e.project(t, a) != "WP1" || e.project(t, w) != "WP1" || e.project(t, other) != "" {
		t.Fatalf("지정 반영 a=%q w=%q other=%q", e.project(t, a), e.project(t, w), e.project(t, other))
	}
	if k := e.wb.TaskActivityKind(a); k != "" {
		t.Fatalf("사업이 붙었는데 활동 분류가 남았다 %q", k)
	}
	if _, _, err := e.as.SetASReceiptProject("O03", "AS1", "WP2"); err != nil {
		t.Fatal(err)
	}
	if e.project(t, a) != "WP2" || e.project(t, w) != "WP2" {
		t.Fatalf("변경 반영 a=%q w=%q", e.project(t, a), e.project(t, w))
	}
	// 같은 값을 다시 맞추면 바뀐 건수 0
	if n, err := SyncTaskProjectsOfReceipt(e.db, "AS1", false); err != nil || n != 0 {
		t.Fatalf("멱등 n=%d err=%v", n, err)
	}
	var logs int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='work_tasks' AND entity_id=? AND reason LIKE '원본(접수 AS1)%'`, a).Scan(&logs)
	if logs < 1 {
		t.Fatal("복사 이력이 남지 않았다")
	}
	// 해제: 원본에서 사업을 해제하면 업무도 비워진다
	if _, _, err := e.as.SetASReceiptProject("O03", "AS1", ""); err != nil {
		t.Fatal(err)
	}
	if e.project(t, a) != "" || e.project(t, w) != "" {
		t.Fatalf("해제 반영 a=%q w=%q", e.project(t, a), e.project(t, w))
	}
	// 원본이 비어 있어도(해제 동작이 아닌 동기화) 업무에 직접 둔 사업은 유지한다
	e.exec(t, `UPDATE work_tasks SET project_id='WP2' WHERE task_id=?`, a)
	if n, _ := SyncTaskProjectsOfReceipt(e.db, "AS1", false); n != 0 || e.project(t, a) != "WP2" {
		t.Fatalf("원본이 비었는데 업무 사업이 바뀌었다 n=%d %q", n, e.project(t, a))
	}
}

// 점검 방문: 방문에서 사업을 지정·해제하면 점검 업무에 반영된다.
func TestWorkTaskFollowsVisitProject227(t *testing.T) {
	e := setup227(t)
	m := e.task(t, "점검업무", model.WBSourceMaintenance, "V1", "")
	if _, _, err := e.mn.SetVisitProject("O03", "V1", "WP1"); err != nil {
		t.Fatal(err)
	}
	if e.project(t, m) != "WP1" {
		t.Fatalf("점검 업무 %q", e.project(t, m))
	}
	if _, _, err := e.mn.SetVisitProject("O03", "V1", ""); err != nil {
		t.Fatal(err)
	}
	if e.project(t, m) != "" {
		t.Fatalf("해제 후 점검 업무 %q", e.project(t, m))
	}
}

// 업무에서 직접 바꾸면 원본에 먼저 쓰고 같은 원본의 다른 업무에도 복사한다(원본이 두 곳이 되지 않는다).
func TestSetTaskProjectWritesThroughToSource227(t *testing.T) {
	e := setup227(t)
	a := e.task(t, "접수업무", model.WBSourceAS, "AS1", "")
	w := e.task(t, "하부업무", model.WBSourceASWork, "W1", "")
	if _, _, err := e.wb.SetTaskProject("O03", a, "WP2"); err != nil {
		t.Fatal(err)
	}
	var rp string
	_ = e.db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='AS1'`).Scan(&rp)
	if rp != "WP2" || e.project(t, a) != "WP2" || e.project(t, w) != "WP2" {
		t.Fatalf("접수=%q 접수업무=%q 하부업무=%q", rp, e.project(t, a), e.project(t, w))
	}
	// 원본이 없는 일반 업무는 종전대로 업무에만 쓴다
	g := e.task(t, "일반", "", "", "")
	if _, _, err := e.wb.SetTaskProject("O03", g, "WP1"); err != nil || e.project(t, g) != "WP1" {
		t.Fatalf("일반 업무 %q %v", e.project(t, g), err)
	}
}

// 기존 건 보정(117): 비어 있는 업무만 채우고 덮어쓰지 않으며, 두 번 돌려도 같고, 전후가 남는다.
func TestMigration117CopiesProject227(t *testing.T) {
	e := setup227(t)
	a := e.task(t, "접수업무", model.WBSourceAS, "AS1", "")
	w := e.task(t, "하부업무", model.WBSourceASWork, "W1", "")
	m := e.task(t, "점검업무", model.WBSourceMaintenance, "V1", "")
	keep := e.task(t, "이미사업있음", model.WBSourceAS, "AS2", "WP2")
	// 원본에만 사업을 심는다(업무 쪽은 비어 있는 옛 상태)
	e.exec(t, `UPDATE as_receipts SET project_id='WP1' WHERE as_id='AS1'`)
	e.exec(t, `UPDATE as_receipts SET project_id='WP1' WHERE as_id='AS2'`)
	e.exec(t, `UPDATE maintenance_visits SET project_id='WP1' WHERE visit_id='V1'`)
	e.exec(t, `DELETE FROM app_settings WHERE setting_key=?`, settingMigration117Done)
	e.exec(t, `DROP TABLE IF EXISTS work_task_copy_117`)
	applyWorkTaskProjectCopyV117(e.db)
	if e.project(t, a) != "WP1" || e.project(t, w) != "WP1" || e.project(t, m) != "WP1" {
		t.Fatalf("보정 a=%q w=%q m=%q", e.project(t, a), e.project(t, w), e.project(t, m))
	}
	if e.project(t, keep) != "WP2" {
		t.Fatalf("이미 사업이 있는 업무를 덮어썼다 %q", e.project(t, keep))
	}
	var n int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM work_task_copy_117`).Scan(&n)
	if n != 3 {
		t.Fatalf("전후 기록 %d건 want 3", n)
	}
	// 사용자가 비운 뒤 재기동해도 되살리지 않는다(1회 실행)
	e.exec(t, `UPDATE work_tasks SET project_id=NULL WHERE task_id=?`, a)
	applyWorkTaskProjectCopyV117(e.db)
	if e.project(t, a) != "" {
		t.Fatal("두 번째 실행이 다시 덮어썼다")
	}
}
