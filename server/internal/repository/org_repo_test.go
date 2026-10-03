package repository

import (
	"path/filepath"
	"testing"
)

func TestOrgRepoCreateUpdateAndCounts(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "orgs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewOrgRepo(db)
	list, err := r.List()
	if err != nil || len(list) != 1 || list[0].OrgID != "O01" {
		t.Fatalf("seed list=%v err=%v", list, err)
	}
	if list[0].OrgName != "도서관사업팀" {
		t.Fatalf("O01 name=%q", list[0].OrgName)
	}
	got, err := r.Create("다른팀", "이팀", "메모", "테스터")
	if err != nil {
		t.Fatal(err)
	}
	if got.OrgID != "O02" || got.OrgNo != "O02" || got.OrgName != "다른팀" {
		t.Fatalf("create %+v", got)
	}
	if err := r.Update("O02", "새이름", "짧", "n", true); err != nil {
		t.Fatal(err)
	}
	after, err := r.Get("O02")
	if err != nil || after == nil || after.OrgName != "새이름" || after.OrgID != "O02" {
		t.Fatalf("update %+v err=%v", after, err)
	}
	if err := r.Update("O02", "숨김팀", "", "", false); err != nil {
		t.Fatal(err)
	}
	active, err := r.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].OrgID != "O01" {
		t.Fatalf("active %+v", active)
	}
}
