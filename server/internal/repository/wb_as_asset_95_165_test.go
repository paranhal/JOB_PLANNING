package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func seed165Customer(t *testing.T, dbPath string) (*WBRepo, *ASRepo) {
	t.Helper()
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	return NewWBRepo(db), NewASRepo(db)
}

func insert165Asset(t *testing.T, wb *WBRepo, id, name string) {
	t.Helper()
	if _, err := wb.db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES (?,?,?)`, id, "C1", name); err != nil {
		t.Fatal(err)
	}
}

func assetIDsOf(t *testing.T, wb *WBRepo, taskID string) []string {
	t.Helper()
	items, err := wb.ListTaskAssets(taskID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range items {
		ids = append(ids, a.AssetID)
	}
	return ids
}

func Test95_165_SyncCopiesReceiptAsset(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "a165.db"))
	insert165Asset(t, wb, "A1", "자가대출기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "카드 인식", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	task, err := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil || task == nil {
		t.Fatalf("업무 없음: %+v err=%v", task, err)
	}
	ids := assetIDsOf(t, wb, task.TaskID)
	if len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("자산 연결=%v", ids)
	}
}

func Test95_165_SyncWithoutAssetLeavesZeroLinks(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "a165b.db"))
	as := &model.ASReceipt{
		CustomerID: "C1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "문의", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
		t.Fatal(err)
	}
	task, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if task == nil {
		t.Fatal("자산 없어도 업무는 만든다")
	}
	if ids := assetIDsOf(t, wb, task.TaskID); len(ids) != 0 {
		t.Fatalf("연결 0건이어야 한다: %v", ids)
	}
}

func Test95_165_TwoEquipmentRowsCopyEachAsset(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "a165c.db"))
	insert165Asset(t, wb, "A1", "자가대출기")
	insert165Asset(t, wb, "A2", "사서대출기")
	for _, id := range []string{"A1", "A2"} {
		as := &model.ASReceipt{
			CustomerID: "C1", AssetID: id, ReceiptChannel: "phone", Requester: "홍길동",
			Symptom: id, Urgency: "normal", Priority: "normal",
			AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
		if err := wb.SyncASDailyTaskFromReceipt(as); err != nil {
			t.Fatal(err)
		}
		task, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
		if task == nil {
			t.Fatalf("%s 업무 없음", id)
		}
		ids := assetIDsOf(t, wb, task.TaskID)
		if len(ids) != 1 || ids[0] != id {
			t.Fatalf("%s 연결=%v", id, ids)
		}
	}
}

func Test95_165_ReconcileDoesNotBackfillAndDryRunDoesNotWrite(t *testing.T) {
	wb, asRepo := seed165Customer(t, filepath.Join(t.TempDir(), "a165d.db"))
	insert165Asset(t, wb, "A1", "자가대출기")
	as := &model.ASReceipt{
		CustomerID: "C1", AssetID: "A1", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "누락", Urgency: "normal", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(), Status: "assigned",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	task, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if task == nil {
		t.Fatal("업무 없음")
	}
	if _, err := wb.db.Exec(`DELETE FROM work_task_assets WHERE task_id=?`, task.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	if ids := assetIDsOf(t, wb, task.TaskID); len(ids) != 0 {
		t.Fatalf("일일 정리가 기존 누락을 채우면 안 된다: %v", ids)
	}
	rep, err := wb.BackfillASTaskAssets(true)
	if err != nil || rep == nil || rep.Missing != 1 || rep.Linked != 0 {
		t.Fatalf("dry-run %+v err=%v", rep, err)
	}
	if ids := assetIDsOf(t, wb, task.TaskID); len(ids) != 0 {
		t.Fatalf("dry-run이 썼다: %v", ids)
	}
	rep, err = wb.BackfillASTaskAssets(false)
	if err != nil || rep == nil || rep.Linked != 1 {
		t.Fatalf("apply %+v err=%v", rep, err)
	}
	ids := assetIDsOf(t, wb, task.TaskID)
	if len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("보정 후=%v", ids)
	}
}
