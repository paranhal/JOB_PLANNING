package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListActivitiesFilterOrgAndPerson(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "vis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	repo := NewSalesRepo(db)
	a := &model.SalesProject{Name: "A사업", OrgID: "O01", SalesOwner: "영업하나", Stage: model.SalesStage4Discover, Status: model.SalesStatusActive}
	b := &model.SalesProject{Name: "B사업", OrgID: "O02", SalesOwner: "다른영업", Stage: model.SalesStage4Discover, Status: model.SalesStatusActive}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(b); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateActivity(&model.SalesActivity{SalesID: a.SalesID, ActivityDate: "2026-10-10", ActivityType: "other", ActivityMethod: "visit", Title: "A방문"}, "영업하나", nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateActivity(&model.SalesActivity{SalesID: b.SalesID, ActivityDate: "2026-10-10", ActivityType: "other", ActivityMethod: "visit", Title: "B방문"}, "다른영업", nil); err != nil {
		t.Fatal(err)
	}

	orgA, err := repo.ListActivitiesFilter(SalesActivityFilter{OrgID: "O01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orgA) != 1 || orgA[0].Title != "A방문" {
		t.Fatalf("O01 acts=%d", len(orgA))
	}
	orgB, err := repo.ListActivitiesFilter(SalesActivityFilter{OrgID: "O02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orgB) != 1 || orgB[0].Title != "B방문" {
		t.Fatalf("O02 acts=%d", len(orgB))
	}
	mine, err := repo.ListActivitiesFilter(SalesActivityFilter{OrgID: "O01", PersonName: "영업하나"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 {
		t.Fatalf("person=%d", len(mine))
	}
	none, err := repo.ListActivitiesFilter(SalesActivityFilter{OrgID: "O01", PersonName: "없는사람"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("없는사람=%d", len(none))
	}

	listA, err := repo.ListFilter(SalesListFilter{OrgID: "O01"})
	if err != nil || len(listA) != 1 || listA[0].Name != "A사업" {
		t.Fatalf("projects O01 n=%d err=%v", len(listA), err)
	}
}
