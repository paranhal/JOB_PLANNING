package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListTasksBetweenUsesWorkDateOnly(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "between.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	for _, tsk := range []model.WorkTask{
		{WorkType: model.WBWorkAdmin, Title: "오늘", WorkDate: "2026-10-19", Status: model.WBTaskWaiting},
		{WorkType: model.WBWorkAdmin, Title: "어제", WorkDate: "2026-10-18", Status: model.WBTaskWaiting},
		{WorkType: model.WBWorkAdmin, Title: "미배정", DueDate: "2026-10-19", Status: model.WBTaskWaiting},
		{WorkType: model.WBWorkAdmin, Title: "상위", WorkDate: "2026-10-19", Status: model.WBTaskWaiting, RecurrenceRole: model.RecurrenceRoleParent},
	} {
		if err := wb.CreateTask(&tsk); err != nil {
			t.Fatal(err)
		}
	}
	got, err := wb.ListTasksBetween("2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "오늘" {
		t.Fatalf("got=%+v", got)
	}
}

func TestListTasksNeedingAutoPlaceAndPlacedKeys(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "auto.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	need := &model.WorkTask{
		WorkType: model.WBWorkAS, Title: "올릴것", WorkDate: "2026-10-19",
		Status: model.WBTaskWaiting, SourceType: model.WBSourceAS, SourceID: "A1",
	}
	placed := &model.WorkTask{
		WorkType: model.WBWorkAS, Title: "이미", WorkDate: "2026-10-19",
		StartTime: "09:00", EndTime: "09:30", Status: model.WBTaskWaiting,
		SourceType: model.WBSourceAS, SourceID: "A2",
	}
	done := &model.WorkTask{
		WorkType: model.WBWorkAS, Title: "완료", WorkDate: "2026-10-19",
		Status: model.WBTaskComplete, SourceType: model.WBSourceAS, SourceID: "A3",
	}
	otherDay := &model.WorkTask{
		WorkType: model.WBWorkAS, Title: "다른날", WorkDate: "2026-10-20",
		Status: model.WBTaskWaiting, SourceType: model.WBSourceAS, SourceID: "A4",
	}
	for _, tsk := range []*model.WorkTask{need, placed, done, otherDay} {
		if err := wb.CreateTask(tsk); err != nil {
			t.Fatal(err)
		}
	}
	auto, err := wb.ListTasksNeedingAutoPlace("2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatal(err)
	}
	if len(auto) != 1 || auto[0].Title != "올릴것" {
		t.Fatalf("auto=%+v", auto)
	}
	keys, err := wb.PlacedSourceKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !keys[model.WBSourceAS+":A1"] || !keys[model.WBSourceAS+":A2"] {
		t.Fatalf("keys=%v", keys)
	}
	if keys[model.WBSourceAS+":missing"] {
		t.Fatal("unexpected key")
	}
}

func TestEnsureMaintenanceTasksBetweenSkipsOutside(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "ensure_range.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','나성동도서관','나성동도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES
		('v-in','MP1','2026-10-19','C1',0,'normal','최혜영','KLAS',0),
		('v-out','MP1','2026-01-01','C1',1,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	repo := NewWBRepo(db)
	n, err := repo.EnsureMaintenanceTasksBetween("2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("created=%d", n)
	}
	in, _ := repo.GetTaskBySource(model.WBSourceMaintenance, "v-in")
	out, _ := repo.GetTaskBySource(model.WBSourceMaintenance, "v-out")
	if in == nil {
		t.Fatal("in-range visit missing")
	}
	if out != nil {
		t.Fatal("out-of-range visit should wait")
	}
}
