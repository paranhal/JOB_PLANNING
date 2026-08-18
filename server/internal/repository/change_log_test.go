package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func TestChangeLogOnCustomerUpdate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "log.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	pop := audit.Push(audit.Actor{Name: "홍길동", Username: "hong"})
	defer pop()

	c := &model.Customer{OrgName: "테스트도서관", OfficialName: "테스트도서관", IsActive: true}
	if err := NewCustomerRepo(db).Create(c); err != nil {
		t.Fatal(err)
	}
	c.OrgName = "수정도서관"
	if err := NewCustomerRepo(db).Update(c); err != nil {
		t.Fatal(err)
	}
	logs, err := audit.ListLogs(20)
	if err != nil || len(logs) < 2 {
		t.Fatalf("이력 건수: %d err=%v", len(logs), err)
	}
	var upd *audit.ChangeLog
	for i := range logs {
		if logs[i].Action == audit.ActionUpdate && logs[i].EntityID == c.CustomerID {
			upd = &logs[i]
			break
		}
	}
	if upd == nil || upd.Who() != "홍길동" {
		t.Fatalf("수정 이력 없음: %+v", logs)
	}
	if err := audit.Rollback(upd.LogID); err != nil {
		t.Fatal(err)
	}
	got, err := NewCustomerRepo(db).GetByID(c.CustomerID)
	if err != nil || got.OrgName != "테스트도서관" {
		t.Fatalf("롤백 후: %+v err=%v", got, err)
	}
}

func countTableLogs(t *testing.T, table, action string) int {
	t.Helper()
	logs, err := audit.ListLogs(200)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range logs {
		if l.TableName == table && (action == "" || l.Action == action) {
			n++
		}
	}
	return n
}

// §25.2 이력 기록 대상: 접수·조치 등록은 ○, 접수 수정·조치 삭제·방문 완료는 ●, 시간표 배치는 ○.
func TestChangeLogScopePerSpec252(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	pop := audit.Push(audit.Actor{Name: "테크", Username: "tech"})
	defer pop()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','테스트도서관','테스트도서관',1)`); err != nil {
		t.Fatal(err)
	}

	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트 오류", Urgency: "중", Priority: "normal",
	}
	if err := NewASRepo(db).Create(as); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "as_receipts", audit.ActionCreate); n != 0 {
		t.Fatalf("접수 등록은 미기록이어야 함: %d", n)
	}

	as.Status = "in_progress"
	as.ActionTaken = "점검"
	as.ProcessType = "현장"
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "as_receipts", audit.ActionUpdate); n < 1 {
		t.Fatalf("접수 수정은 기록해야 함: %d", n)
	}

	p := &model.ASProcess{ASID: as.ASID, Worker: "테크", WorkContent: "조치", TimeSpent: 30}
	if err := NewASProcessRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "as_processes", audit.ActionCreate); n != 0 {
		t.Fatalf("조치 등록은 미기록이어야 함: %d", n)
	}
	if err := NewASProcessRepo(db).DeleteByASAndID(as.ASID, p.ProcessID); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "as_processes", audit.ActionDelete); n != 1 {
		t.Fatalf("조치 삭제는 기록해야 함: %d", n)
	}

	plan, err := NewMaintenanceRepo(db).CreatePlan(2026, "2026 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewMaintenanceRepo(db).InsertVisit(plan.PlanID, "2026-08-14", "C1", 0, 0, "normal", ""); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "maintenance_visits", audit.ActionCreate); n != 0 {
		t.Fatalf("방문 추가는 미기록이어야 함: %d", n)
	}
	var visitID string
	if err := db.QueryRow(`SELECT visit_id FROM maintenance_visits WHERE plan_id=?`, plan.PlanID).Scan(&visitID); err != nil {
		t.Fatal(err)
	}
	if err := NewMaintenanceRepo(db).SetVisitCompleted(visitID, true, "2026-08-14"); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "maintenance_visits", audit.ActionUpdate); n < 1 {
		t.Fatalf("방문 완료는 기록해야 함: %d", n)
	}

	// 당월·완료 방문은 계획 삭제가 거부되므로, 이력 확인 후 미래 미완료로 바꿔 삭제한다.
	if _, err := db.Exec(`UPDATE maintenance_visits SET completed=0, completed_date='', visit_date='2026-11-14' WHERE visit_id=?`, visitID); err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	task := &model.WorkTask{
		Title: "배치 업무", WorkType: model.WBWorkAdmin,
		WorkDate: "2026-08-14", StartTime: "09:00", EndTime: "10:00", DurationMin: 60,
		Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "work_tasks", ""); n != 0 {
		t.Fatalf("시간표 배치는 미기록이어야 함: %d", n)
	}
	if err := wb.PlaceTask(task.TaskID, "2026-08-15", "10:00", "11:00"); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "work_tasks", ""); n != 0 {
		t.Fatalf("PlaceTask도 미기록이어야 함: %d", n)
	}

	if err := NewSettingsRepo(db).Set("notice", "안내"); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "app_settings", ""); n < 1 {
		t.Fatalf("설정 변경은 기록해야 함: %d", n)
	}

	if err := NewMaintenanceRepo(db).DeletePlan(plan.PlanID); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "maintenance_visits", audit.ActionDelete); n < 1 {
		t.Fatalf("계획 삭제 시 방문 삭제는 기록해야 함: %d", n)
	}
}

func TestChangeLogBindsRepoDBWithoutInit(t *testing.T) {
	audit.Init(nil)
	db, err := InitDB(filepath.Join(t.TempDir(), "bind.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	c := &model.Customer{OrgName: "바인딩도서관", OfficialName: "바인딩도서관", IsActive: true}
	if err := NewCustomerRepo(db).Create(c); err != nil {
		t.Fatal(err)
	}
	if n := countTableLogs(t, "customers", audit.ActionCreate); n != 1 {
		t.Fatalf("Init 없이도 고객 등록이 기록돼야 함: %d", n)
	}
}

