package repository

import (
	"fmt"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test95_165_SplitByTargetsOneTransaction(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "split.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','C1','스위치'),('A2','C1','발급기')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES('P1','시험사업','active',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	src := &model.WorkTask{
		Title: "원제목", WorkType: model.WBWorkAdmin, WorkDate: "2026-11-20",
		Assignee: "테스터", CustomerID: "C1", OrgID: model.OrgIDLibrary, ProjectID: "P1",
	}
	targets := []SplitTarget{{Kind: "asset", ID: "A1", Name: "스위치"}, {Kind: "asset", ID: "A2", Name: "발급기"}}
	made, err := wb.SplitByTargets(src, targets, nil, SplitLinks{})
	if err != nil {
		t.Fatal(err)
	}
	if len(made) != 2 {
		t.Fatalf("made %d", len(made))
	}
	if made[0].Title != "원제목 - 스위치" || made[1].Title != "원제목 - 발급기" {
		t.Fatalf("titles %q %q", made[0].Title, made[1].Title)
	}
	if made[0].Assignee != made[1].Assignee || made[0].WorkDate != made[1].WorkDate || made[0].CustomerID != made[1].CustomerID || made[0].ProjectID != made[1].ProjectID {
		t.Fatalf("inherit %+v %+v", made[0], made[1])
	}
	if made[1].SourceType != "split" || made[1].SourceRole != "asset:A2" || made[1].ParentTaskID != "" {
		t.Fatalf("copy source %+v", made[1])
	}
	for i, id := range []string{"A1", "A2"} {
		got, err := wb.ListTaskAssets(made[i].TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].AssetID != id {
			t.Fatalf("link %d %+v", i, got)
		}
	}
	again, err := wb.SplitByTargets(made[0], targets, nil, SplitLinks{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 2 || again[1].TaskID != made[1].TaskID {
		t.Fatalf("dup %+v", again)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows %d", n)
	}
}

func Test95_165_SplitByTargetsRollsBack(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "split.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	prev := splitBeforeInsert
	splitBeforeInsert = func(i int) error {
		if i == 1 {
			return fmt.Errorf("중단")
		}
		return nil
	}
	t.Cleanup(func() { splitBeforeInsert = prev })
	src := &model.WorkTask{Title: "원제목", WorkType: model.WBWorkSales, WorkDate: "2026-11-20", Assignee: "테스터", OrgID: model.OrgIDLibrary}
	_, err = wb.SplitByTargets(src, []SplitTarget{{Kind: "asset", ID: "A1", Name: "스위치"}, {Kind: "asset", ID: "A2", Name: "발급기"}}, nil, SplitLinks{})
	if err == nil {
		t.Fatal("want rollback")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rolled back rows=%d", n)
	}
}

func Test95_165_SplitRefusesASAndMaintenance(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "split.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	targets := []SplitTarget{{Kind: "asset", ID: "A1", Name: "스위치"}, {Kind: "asset", ID: "A2", Name: "발급기"}}
	cases := []*model.WorkTask{
		{Title: "접수", WorkType: model.WBWorkAS, WorkDate: "2026-11-20"},
		{Title: "점검", WorkType: model.WBWorkMaintenance, WorkDate: "2026-11-20"},
		{Title: "원본", WorkType: model.WBWorkAdmin, SourceType: model.WBSourceAS, WorkDate: "2026-11-20"},
		{Title: "완료", WorkType: model.WBWorkAdmin, Status: model.WBTaskComplete, WorkDate: "2026-11-20"},
		{Title: "반복", WorkType: model.WBWorkAdmin, RecurrenceRole: model.RecurrenceRoleParent, WorkDate: "2026-11-20"},
	}
	for _, src := range cases {
		_, err := wb.SplitByTargets(src, targets, nil, SplitLinks{})
		if err == nil {
			t.Fatalf("%s: 거절되지 않음", src.Title)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rows %d", n)
	}
}
