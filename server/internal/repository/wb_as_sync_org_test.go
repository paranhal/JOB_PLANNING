package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSyncASDailyTaskOrgIDFromReceipt(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('c-si','정안초등학교','정안초등학교',1,'O03'),
		('c-media','미디어고객','미디어고객',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local)
	si := &model.ASReceipt{
		CustomerID: "c-si", ReceiptChannel: "phone", Requester: "학교", Symptom: "정안초",
		AssignedTo: "박창훈", ReceiptDatetime: at, VisitScheduledDate: "2026-10-25", OrgID: "O03",
	}
	if err := asRepo.Create(si); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(si); err != nil {
		t.Fatal(err)
	}
	tk, err := wb.GetTaskBySource(model.WBSourceAS, si.ASID)
	if err != nil || tk == nil {
		t.Fatalf("SI 동기화: %v", err)
	}
	if tk.OrgID != "O03" {
		t.Fatalf("SI work_tasks.org_id=%q (O03 이어야 함, 예전 기본값 O01)", tk.OrgID)
	}
	media := &model.ASReceipt{
		CustomerID: "c-media", ReceiptChannel: "phone", Requester: "담당", Symptom: "미디어",
		AssignedTo: "박창훈", ReceiptDatetime: at, VisitScheduledDate: "2026-10-25", OrgID: "O02",
	}
	if err := asRepo.Create(media); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(media); err != nil {
		t.Fatal(err)
	}
	mt, err := wb.GetTaskBySource(model.WBSourceAS, media.ASID)
	if err != nil || mt == nil || mt.OrgID != "O02" {
		t.Fatalf("미디어 org=%v err=%v", mt, err)
	}
}

func TestCorrectWorkTaskOrgFromSourceIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_org_fix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES ('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-si','정안초등학교','정안초등학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c-si", ReceiptChannel: "phone", Requester: "학교", Symptom: "오염",
		AssignedTo: "박창훈", ReceiptDatetime: time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local),
		OrgID: "O03",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	tk, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if tk == nil {
		t.Fatal("task")
	}
	if _, err := db.Exec(`UPDATE work_tasks SET org_id=? WHERE task_id=?`, model.OrgIDLibrary, tk.TaskID); err != nil {
		t.Fatal(err)
	}
	n, byOrg, err := CorrectWorkTaskOrgFromSource(db)
	if err != nil || n != 1 || byOrg["O03"] != 1 {
		t.Fatalf("1차 n=%d by=%v err=%v", n, byOrg, err)
	}
	got, _ := wb.GetTaskBySource(model.WBSourceAS, as.ASID)
	if got == nil || got.OrgID != "O03" {
		t.Fatalf("보정 후 %v", got)
	}
	n2, _, err := CorrectWorkTaskOrgFromSource(db)
	if err != nil || n2 != 0 {
		t.Fatalf("멱등 n2=%d err=%v", n2, err)
	}
}

func TestCreateTaskSourcedRequiresOrgID(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ct_org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	err = wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAS, Title: "출처없음", Status: model.WBTaskWaiting,
		SourceType: model.WBSourceAS, SourceID: "no-such",
	})
	if err == nil {
		t.Fatal("AS 출처인데 org_id 없이 생성되면 안 된다")
	}
}
