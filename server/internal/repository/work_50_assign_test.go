package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func TestCollectUnassignedFourKindsAndOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "unassigned50.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewWorkBoardRepo(db)
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C1','기관A','기관A',1,'O1'),('C2','기관B','기관B',1,'O2')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, assigned_user_id, org_id, created_at, updated_at)
		VALUES ('A1','AS-1',?,'C1','증상','received','','','O1',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('A2','AS-2',?,'C2','다른조직','received','','O2',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P1',2026,'계획','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, assignee)
		VALUES ('V1','P1',?,'C1',0,'')`, time.Now().Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	if err := NewWBRepo(db).CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "행정미배정", Status: model.WBTaskWaiting, OrgID: "O1"}); err != nil {
		t.Fatal(err)
	}
	if err := NewWBRepo(db).CreateTask(&model.WorkTask{WorkType: model.WBWorkSales, Title: "영업미배정", Status: model.WBTaskWaiting, OrgID: "O1"}); err != nil {
		t.Fatal(err)
	}

	items, err := r.CollectUnassigned("O1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("card/panel count=%d %+v", len(items), items)
	}
	seen := map[string]int{}
	for _, it := range items {
		seen[it.Prefix]++
	}
	if seen[model.WorkPrefixAS] < 1 || seen[model.WorkPrefixMaintenance] < 1 || seen[model.WorkPrefixGeneral] < 1 || seen[model.WorkPrefixSales] < 1 {
		t.Fatalf("prefixes %+v", seen)
	}
	other, err := r.CollectUnassigned("O2")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range other {
		if it.RefID == "A1" || it.Prefix == model.WorkPrefixSales {
			t.Fatalf("other org leaked %+v", it)
		}
	}
}

func TestStampAssignedOnlyOnChange(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "stamp50.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','기관','기관',1,'O1')`); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, org_id, created_at, updated_at)
		VALUES ('A1','AS-1',?,'C1','증상','received','O1',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	audit.Push(audit.Actor{Name: "테스터"})
	stampAssignedIfChanged(db, "as_receipts", "as_id", "A1", "", "", "홍길동", "")
	var at1 string
	_ = db.QueryRow(`SELECT COALESCE(assigned_at,'') FROM as_receipts WHERE as_id='A1'`).Scan(&at1)
	if at1 == "" {
		t.Fatal("assigned_at empty")
	}
	time.Sleep(20 * time.Millisecond)
	stampAssignedIfChanged(db, "as_receipts", "as_id", "A1", "홍길동", "", "홍길동", "")
	var at2 string
	_ = db.QueryRow(`SELECT COALESCE(assigned_at,'') FROM as_receipts WHERE as_id='A1'`).Scan(&at2)
	if at1 != at2 {
		t.Fatalf("same person restamp %q -> %q", at1, at2)
	}
}

func TestListUnplacedTasksKeepsSales(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "unplaced50.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkSales, Title: "영업활동", Status: model.WBTaskWaiting, SourceType: model.WBSourceSalesActivity, SourceID: "act1", OrgID: model.OrgIDLibrary}); err != nil {
		t.Fatal(err)
	}
	got, err := wb.ListUnplacedTasks(OrgAll, []string{model.WBWorkSales})
	if err != nil || len(got) != 1 {
		t.Fatalf("sales unplaced %d err=%v", len(got), err)
	}
}
