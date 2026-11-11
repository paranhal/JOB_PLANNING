package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_170_BackfillDryRunWritesNothingAndApplyIsIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "bf170.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c170','KDI','KDI',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c170", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "백필", AssignedTo: "관리자", Status: model.StatusPartialComplete,
		ReceiptDatetime: time.Now(), OrgID: "O01", VisitScheduledDate: "2026-11-20",
	}
	if err := NewASRepo(db).Create(as); err != nil {
		t.Fatal(err)
	}
	w := &model.ASWorkItem{
		ASID: as.ASID, WorkKind: model.WorkKindRevisit, ScheduledDate: "2026-11-20",
		ScheduleConfirmed: true, AssignedTo: "관리자", Status: "open",
	}
	if err := NewASWorkRepo(db).Create(w); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=?`, model.WBSourceASWork).Scan(&before); err != nil {
		t.Fatal(err)
	}
	dry, err := wb.BackfillASWorkTasks(true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Missing != 1 || dry.Created != 0 {
		t.Fatalf("dry-run %+v", dry)
	}
	var afterDry int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=?`, model.WBSourceASWork).Scan(&afterDry); err != nil {
		t.Fatal(err)
	}
	if afterDry != before {
		t.Fatalf("dry-run이 썼다 %d → %d", before, afterDry)
	}
	applied, err := wb.BackfillASWorkTasks(false)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Created != 1 {
		t.Fatalf("apply %+v", applied)
	}
	again, err := wb.BackfillASWorkTasks(false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 || again.Already < 1 {
		t.Fatalf("재실행 %+v", again)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`, model.WBSourceASWork, w.WorkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("업무 %d건", n)
	}
}

func Test95_170_ReconcileKeepsASWorkAndDoesNotDoubleCount(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rc170.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c170b','KDI','KDI',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c170b", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "정리", AssignedTo: "관리자", Status: model.StatusPartialComplete,
		ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	if err := NewASRepo(db).Create(as); err != nil {
		t.Fatal(err)
	}
	if err := NewWBRepo(db).SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	w := &model.ASWorkItem{
		ASID: as.ASID, WorkKind: model.WorkKindConfirm, ScheduledDate: "2026-11-21",
		ScheduleConfirmed: false, AssignedTo: "관리자", Status: "open", Notes: "회신",
	}
	if err := NewASWorkRepo(db).Create(w); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	if err := wb.SyncASWorkDailyTask(w); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetTaskBySource(model.WBSourceASWork, w.WorkID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	id := got.TaskID
	if _, err := ReconcileASDailyTasks(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileASWorkDailyTasks(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileASWorkDailyTasks(db); err != nil {
		t.Fatal(err)
	}
	again, _ := wb.GetTaskBySource(model.WBSourceASWork, w.WorkID)
	if again == nil || again.TaskID != id {
		t.Fatalf("정리 후 업무가 바뀜 %+v", again)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`, model.WBSourceASWork, w.WorkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("하위업무 업무 %d", n)
	}
	parent, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if parent == nil {
		t.Fatal("접수 본체 업무가 지워졌다")
	}
	stats := NewStatsRepo(db)
	f := model.StatsMeetingFilter{}
	openN, err := stats.countWeeklyInProgress("2099-01-01", f)
	if err != nil {
		t.Fatal(err)
	}
	adminN, err := stats.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.source_type=?`, []interface{}{model.WBSourceASWork}...)
	if err != nil {
		t.Fatal(err)
	}
	if adminN != 0 {
		t.Fatalf("행정 통계에 하위업무가 섞임 %d", adminN)
	}
	if err := wb.SyncASWorkDailyTask(w); err != nil {
		t.Fatal(err)
	}
	openN2, err := stats.countWeeklyInProgress("2099-01-01", f)
	if err != nil {
		t.Fatal(err)
	}
	if openN2 != openN {
		t.Fatalf("동기화 후 진행중 집계가 늘었다 %d → %d", openN, openN2)
	}
	var workRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_work_items WHERE as_id=?`, as.ASID).Scan(&workRows); err != nil {
		t.Fatal(err)
	}
	if workRows != 1 {
		t.Fatalf("하위업무 원본 %d", workRows)
	}
}

func Test95_170_DeleteReceiptRemovesWorkTask(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "del170.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c170c','KDI','KDI',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c170c", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "삭제", AssignedTo: "관리자", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	repo := NewASRepo(db)
	if err := repo.Create(as); err != nil {
		t.Fatal(err)
	}
	w := &model.ASWorkItem{ASID: as.ASID, WorkKind: model.WorkKindSupport, ScheduledDate: "2026-11-22", AssignedTo: "관리자", Status: "open"}
	if err := NewASWorkRepo(db).Create(w); err != nil {
		t.Fatal(err)
	}
	if err := NewWBRepo(db).SyncASWorkDailyTask(w); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(as.ASID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`, model.WBSourceASWork, w.WorkID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("접수 삭제 후 하위업무 업무가 남음 %d", n)
	}
}
