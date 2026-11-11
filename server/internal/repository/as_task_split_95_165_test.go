package repository

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_165D_SplitASKeepsOneReceipt(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "d1.db"))
	insert165Asset(t, wb, "A1", "스위치")
	insert165Asset(t, wb, "A2", "발급기")
	insert165Asset(t, wb, "A3", "반납기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	primary, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	base := primary.Title
	made, err := wb.SplitASReceipt(as.ASID, base, []ASSplitTarget{
		{AssetID: "A1", Name: "스위치"},
		{AssetID: "A2", Name: "발급기"},
		{AssetID: "A3", Name: "반납기"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(made) != 3 {
		t.Fatalf("made %d", len(made))
	}
	if made[0].TaskID != primary.TaskID || made[0].SourceRole != "" {
		t.Fatalf("원 업무 %+v", made[0])
	}
	if made[0].Title != base+" - 스위치" || made[1].Title != base+" - 발급기" || made[2].Title != base+" - 반납기" {
		t.Fatalf("titles %q %q %q", made[0].Title, made[1].Title, made[2].Title)
	}
	if made[1].SourceType != model.WBSourceAS || made[1].SourceID != as.ASID || made[1].SourceRole != "asset:A2" {
		t.Fatalf("copy %+v", made[1])
	}
	if ids := assetIDsOf(t, wb, made[0].TaskID); len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("primary assets %v", ids)
	}
	if ids := assetIDsOf(t, wb, made[2].TaskID); len(ids) != 1 || ids[0] != "A3" {
		t.Fatalf("third assets %v", ids)
	}
	var receipts int
	if err := wb.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("receipts %d", receipts)
	}
	again, err := wb.SplitASReceipt(as.ASID, base, []ASSplitTarget{
		{AssetID: "A1"}, {AssetID: "A2"}, {AssetID: "A3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 || again[1].TaskID != made[1].TaskID {
		t.Fatal("재전송으로 업무가 늘었다")
	}
	var n int
	if err := wb.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("rows %d", n)
	}
}

func Test95_165D_SplitASRollsBack(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "d1b.db"))
	insert165Asset(t, wb, "A1", "스위치")
	insert165Asset(t, wb, "A2", "발급기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	before, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	prev := asSplitBeforeInsert
	asSplitBeforeInsert = func(i int) error {
		if i == 1 {
			return fmt.Errorf("중단")
		}
		return nil
	}
	t.Cleanup(func() { asSplitBeforeInsert = prev })
	_, err := wb.SplitASReceipt(as.ASID, before.Title, []ASSplitTarget{{AssetID: "A1", Name: "스위치"}, {AssetID: "A2", Name: "발급기"}})
	if err == nil {
		t.Fatal("want rollback")
	}
	var n int
	if err := wb.db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("rows %d", n)
	}
	got, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if got == nil || got.Title != before.Title {
		t.Fatalf("title %v", got)
	}
}

func Test95_165D_SyncKeepsSplitTasks(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "d1c.db"))
	insert165Asset(t, wb, "A1", "스위치")
	insert165Asset(t, wb, "A2", "발급기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	primary, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	made, err := wb.SplitASReceipt(as.ASID, primary.Title, []ASSplitTarget{{AssetID: "A1", Name: "스위치"}, {AssetID: "A2", Name: "발급기"}})
	if err != nil {
		t.Fatal(err)
	}
	as.AssignedTo = "다른사람"
	as.Status = "in_progress"
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileASDailyTasks(wb.db); err != nil {
		t.Fatal(err)
	}
	still, _ := wb.GetTask(made[1].TaskID)
	if still == nil || still.SourceRole != "asset:A2" || still.Title != made[1].Title {
		t.Fatalf("분리 업무가 바뀜 %+v", still)
	}
	var n int
	if err := wb.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_role LIKE 'asset:%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("split rows %d", n)
	}
	kept, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if kept == nil || kept.Title != made[0].Title {
		t.Fatalf("원 제목이 덮임 %+v", kept)
	}
}

func Test95_165D_ReceiptCompletesWhenAllSplitsDone(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "d1d.db"))
	insert165Asset(t, wb, "A1", "스위치")
	insert165Asset(t, wb, "A2", "발급기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	primary, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	made, err := wb.SplitASReceipt(as.ASID, primary.Title, []ASSplitTarget{{AssetID: "A1", Name: "스위치"}, {AssetID: "A2", Name: "발급기"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wb.db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, as.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.MarkPrimaryASTaskComplete(as.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASReceiptStatusFromSplits(as.ASID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := wb.db.QueryRow(`SELECT status FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" {
		t.Fatalf("일부 완료인데 %s", status)
	}
	if _, err := wb.db.Exec(`UPDATE work_tasks SET status=? WHERE task_id=?`, model.WBTaskComplete, made[1].TaskID); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASReceiptStatusFromSplits(as.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.db.QueryRow(`SELECT status FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("전부 완료인데 %s", status)
	}

	plain := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "단독", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "completed", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(plain); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, plain.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASReceiptStatusFromSplits(plain.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.db.QueryRow(`SELECT status FROM as_receipts WHERE as_id=?`, plain.ASID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("분리 없는 접수 상태 %s", status)
	}
}

func Test95_165D_RefusesCompletedAndMaintenanceStillBlocked(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "d1e.db"))
	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "끝", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "completed", OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, as.ASID); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	_, err := wb.SplitASReceipt(as.ASID, "끝", []ASSplitTarget{{AssetID: "A1", Name: "스위치"}, {AssetID: "A2", Name: "발급기"}})
	if err == nil {
		t.Fatal("완료 접수가 나뉨")
	}
	src := &model.WorkTask{Title: "점검", WorkType: model.WBWorkMaintenance, WorkDate: "2026-11-20"}
	if _, err := wb.SplitByTargets(src, []SplitTarget{{Kind: "asset", ID: "A1", Name: "스위치"}, {Kind: "asset", ID: "A2", Name: "발급기"}}, nil, SplitLinks{}); err == nil {
		t.Fatal("정기점검이 나뉨")
	}
}
