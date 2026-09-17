package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSyncASDailyTaskAssignedWithoutVisitDate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "장서점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, err := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil || task == nil {
		t.Fatalf("담당자 배정만으로 일일업무가 없다: %+v err=%v", task, err)
	}
	if strings.TrimSpace(task.WorkDate) != "" || strings.TrimSpace(task.DueDate) != "" {
		t.Fatalf("예정 없으면 날짜는 비워야 한다 due=%s work=%s", task.DueDate, task.WorkDate)
	}
	if task.Assignee != "양기헌" || task.AssigneeSource != model.WBAssigneeSourceAS {
		t.Fatalf("담당자=%s source=%s", task.Assignee, task.AssigneeSource)
	}
	undated, err := wb.ListUndatedASTasks()
	if err != nil || len(undated) != 1 || undated[0].TaskID != task.TaskID {
		t.Fatalf("날짜 미정 목록: %+v err=%v", undated, err)
	}
}

func TestSyncASDailyTaskFollowsAssigneeUnlessManual(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_assign.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}

	as.AssignedTo = "태자운"
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if task == nil || task.Assignee != "태자운" {
		t.Fatalf("AS 담당자 변경이 안 따라감: %+v", task)
	}

	if err := wb.SetTaskAssignee(task.TaskID, "최혜영"); err != nil {
		t.Fatal(err)
	}
	as.AssignedTo = "양기헌"
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, _ = wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if task == nil || task.Assignee != "최혜영" || task.AssigneeSource != model.WBAssigneeSourceManual {
		t.Fatalf("manual 담당자가 AS에 덮였다: %+v", task)
	}
}

func TestSyncASDailyTaskUnassignedRemovesRow(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_unassign.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "미배정", Urgency: "normal", Priority: "normal",
		ReceiptDatetime: time.Now(), Status: "received",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, err := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil || task != nil {
		t.Fatalf("미배정은 일일업무에 올리면 안 된다: %+v", task)
	}

	as.AssignedTo = "양기헌"
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	as.AssignedTo = ""
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, _ = wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if task != nil {
		t.Fatalf("담당자 빼도 일일업무가 남았다: %+v", task)
	}
}

func TestReconcileASDailyTasksDoesNotNeedGET(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_hk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, visit_scheduled_date)
		VALUES ('R-HK','R-HK','C1','2026-09-01 10:00:00','정리','assigned','태자운','')`); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	if t0, _ := wb.GetTaskBySource(model.WBSourceAS, "R-HK"); t0 != nil {
		t.Fatal("정리 전에 행이 있으면 안 된다")
	}
	n, err := ReconcileASDailyTasks(db)
	if err != nil || n < 1 {
		t.Fatalf("정리 n=%d err=%v", n, err)
	}
	task, _ := wb.GetTaskBySource(model.WBSourceAS, "R-HK")
	if task == nil || task.Assignee != "태자운" {
		t.Fatalf("정리 후 일일업무 없음: %+v", task)
	}
}

func TestReconcileASDailyTasksSkipsBadDateAndContinues(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_hk_bad.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, visit_scheduled_date)
		VALUES
		 ('R-BAD','R-BAD','C1','2026-09-01 10:00:00','잘못된날짜','assigned','양기헌','0206-08-12'),
		 ('R-OK','R-OK','C1','2026-09-01 10:00:00','정상','assigned','태자운','2026-09-10')`); err != nil {
		t.Fatal(err)
	}
	n, err := ReconcileASDailyTasks(db)
	if err != nil {
		t.Fatalf("한 건 때문에 정리가 멈추면 안 된다: n=%d err=%v", n, err)
	}
	if n < 2 {
		t.Fatalf("두 건 모두 맞춰야 한다 n=%d", n)
	}
	wb := NewWBRepo(db)
	bad, _ := wb.GetTaskBySource(model.WBSourceAS, "R-BAD")
	if bad == nil {
		t.Fatal("잘못된 예정일이어도 일일업무는 있어야 한다")
	}
	if strings.TrimSpace(bad.DueDate) != "" || strings.TrimSpace(bad.WorkDate) != "" {
		t.Fatalf("잘못된 예정일은 비워야 한다 due=%s work=%s", bad.DueDate, bad.WorkDate)
	}
	ok, _ := wb.GetTaskBySource(model.WBSourceAS, "R-OK")
	if ok == nil || ok.Assignee != "태자운" || ok.DueDate != "2026-09-10" {
		t.Fatalf("정상 건이 안 맞춰졌다: %+v", ok)
	}
}
