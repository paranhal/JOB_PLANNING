package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestWorkTaskAssetsAndASMove(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "45d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','해미도서관','해미도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, model_name, operation_status) VALUES
		('A1','C1','발급기','M1','operating'),
		('A2','C1','반납기','M2','operating')`); err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "IP 수정", Status: model.WBTaskWaiting,
		DueDate: "2026-09-26", WorkDate: "2026-09-26", DurationMin: 30,
		CustomerID: "C1", ReceiptDate: "2026-09-20",
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := wb.ReplaceTaskAssets(task.TaskID, []string{"A1", "A2"}); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetTask(task.TaskID)
	if err != nil || got == nil || len(got.LinkedAssets) != 2 {
		t.Fatalf("linked assets=%v err=%v", got, err)
	}
	hist, err := wb.ListAssetSupportHistory("A1", 20)
	if err != nil || len(hist) != 1 || hist[0].Kind != "admin" {
		t.Fatalf("support hist %+v err=%v", hist, err)
	}

	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, asset_id, receipt_datetime, symptom, status,
			assigned_to, start_datetime, complete_datetime, visit_date, process_type, data_origin)
		VALUES
		('R1','R2609-001','C1','A1','2026-09-10 09:00:00','고장','completed','김기술','2026-09-11 10:00:00','2026-09-12 11:00:00','2026-09-11','visit','app'),
		('R2','R2609-002','C1','A2','2026-09-10 09:00:00','태그 테스트','completed','김기술','2026-09-11 10:00:00','2026-09-12 11:00:00','2026-09-11','visit','app')`); err != nil {
		t.Fatal(err)
	}

	st := NewStatsRepo(db)
	f := model.StatsMeetingFilter{IncludeImport: true}
	f.MetricsBaseDate = ""
	_, _, n, err := st.avgASLeadTimes("2026-09-01", "2026-10-01", f)
	if err != nil || n != 2 {
		t.Fatalf("before move n=%d err=%v", n, err)
	}

	asRepo := NewASRepo(db)
	if _, err := asRepo.MoveToAdminWork("R2", ""); err != ErrASMoveReason {
		t.Fatalf("empty reason: %v", err)
	}
	moved, err := asRepo.MoveToAdminWork("R2", "행정 요청이라 AS가 아님")
	if err != nil || moved == nil || moved.SourceType != "" {
		t.Fatalf("move %+v err=%v", moved, err)
	}
	as2, _ := asRepo.GetByID("R2")
	if as2.Status != model.StatusAdminWork || as2.MovedTaskID != moved.TaskID {
		t.Fatalf("as after move %+v", as2)
	}
	assets, _ := wb.ListTaskAssets(moved.TaskID)
	if len(assets) != 1 || assets[0].AssetID != "A2" {
		t.Fatalf("moved assets %+v", assets)
	}
	src, _ := asRepo.GetByMovedTaskID(moved.TaskID)
	if src == nil || src.ASNumber != "R2609-002" {
		t.Fatalf("reverse link %+v", src)
	}

	_, _, n, err = st.avgASLeadTimes("2026-09-01", "2026-10-01", f)
	if err != nil || n != 1 {
		t.Fatalf("after move n=%d err=%v", n, err)
	}
	cnt, err := st.CountASAdminMoved("2026-09-01", "2026-10-01", f)
	if err != nil || cnt != 1 {
		t.Fatalf("moved count=%d err=%v", cnt, err)
	}

	var reason string
	err = db.QueryRow(`SELECT reason FROM data_change_logs WHERE table_name='as_receipts' AND entity_id='R2' ORDER BY occurred_at DESC LIMIT 1`).Scan(&reason)
	if err != nil || !strings.Contains(reason, "행정 요청") {
		t.Fatalf("log reason=%q err=%v", reason, err)
	}

	codes := NewCodeRepo(db)
	items, _ := codes.ActiveByGroup(model.CodeGroupClassifyHint)
	if len(items) == 0 || items[0].CodeName == "" {
		t.Fatal("classify hint code missing")
	}
}
