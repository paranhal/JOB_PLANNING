package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_165D_BackfillDryRunApplyIdempotent(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "a165bf.db"))
	insert165Asset(t, wb, "A1", "자가대출기")
	insert165Asset(t, wb, "A2", "사서대출기")

	linked := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "이미연결", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	missing := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "누락", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	none := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "자산없음", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	orphan := &model.ASReceipt{
		ASNumber: "AS-ORPHAN", CustomerID: "C1", AssetID: "A2", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "업무없음", Urgency: "normal", Priority: "normal",
		AssignedTo: "김담당", ReceiptDatetime: time.Date(2026, 3, 2, 9, 0, 0, 0, time.Local), Status: "received",
	}
	for _, as := range []*model.ASReceipt{linked, missing, none, orphan} {
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := wb.db.Exec(`INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, status) VALUES ('W1','W-1',?,'visit','open')`, orphan.ASID); err != nil {
		t.Fatal(err)
	}
	for _, as := range []*model.ASReceipt{linked, missing, none} {
		if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
			t.Fatal(err)
		}
	}
	missTask, _ := wb.GetTaskBySource(model.WBSourceAS, missing.ASID)
	if missTask == nil {
		t.Fatal("누락 업무 없음")
	}
	if _, err := wb.db.Exec(`DELETE FROM work_task_assets WHERE task_id=?`, missTask.TaskID); err != nil {
		t.Fatal(err)
	}
	copy := &model.WorkTask{
		WorkType: model.WBWorkAS, Title: "분리", Status: model.WBTaskWaiting,
		SourceType: model.WBSourceAS, SourceID: missing.ASID, SourceRole: "asset:A2",
		Assignee: "양기헌", CustomerID: "C1",
	}
	if err := wb.CreateTask(copy); err != nil {
		t.Fatal(err)
	}

	dry, err := wb.BackfillASTaskAssets(true)
	if err != nil || dry == nil || !dry.DryRun || dry.Missing != 1 || dry.Linked != 0 || dry.AlreadyLinked != 1 {
		t.Fatalf("dry %+v err=%v", dry, err)
	}
	if ids := assetIDsOf(t, wb, missTask.TaskID); len(ids) != 0 {
		t.Fatalf("dry-run이 썼다: %v", ids)
	}
	if len(dry.NoTask) != 1 || dry.NoTask[0].ASID != orphan.ASID || !dry.NoTask[0].HasWorkItem || dry.NoTask[0].Assignee != "김담당" {
		t.Fatalf("업무 없는 접수 %+v", dry.NoTask)
	}

	apply, err := wb.BackfillASTaskAssets(false)
	if err != nil || apply == nil || apply.Linked != 1 {
		t.Fatalf("apply %+v err=%v", apply, err)
	}
	if ids := assetIDsOf(t, wb, missTask.TaskID); len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("원 업무 연결=%v", ids)
	}
	if ids := assetIDsOf(t, wb, copy.TaskID); len(ids) != 0 {
		t.Fatalf("분리 업무에 접수 자산을 붙이면 안 된다: %v", ids)
	}
	noneTask, _ := wb.GetTaskBySource(model.WBSourceAS, none.ASID)
	if noneTask == nil {
		t.Fatal("자산 없는 접수 업무")
	}
	if ids := assetIDsOf(t, wb, noneTask.TaskID); len(ids) != 0 {
		t.Fatalf("자산 없는 접수 영향: %v", ids)
	}
	var orphanTasks int
	if err := wb.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`, model.WBSourceAS, orphan.ASID).Scan(&orphanTasks); err != nil {
		t.Fatal(err)
	}
	if orphanTasks != 0 {
		t.Fatalf("업무 없는 접수에 업무를 만들었다: %d", orphanTasks)
	}

	again, err := wb.BackfillASTaskAssets(false)
	if err != nil || again == nil || again.Missing != 0 || again.Linked != 0 || again.AlreadyLinked != 2 {
		t.Fatalf("재실행 %+v err=%v", again, err)
	}
}
