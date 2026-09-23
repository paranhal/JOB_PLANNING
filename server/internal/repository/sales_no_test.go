package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesNoCreateAndSearch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_no.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p1 := &model.SalesProject{Name: "9월 1건"}
	if err := repo.Create(p1); err != nil {
		t.Fatal(err)
	}
	if p1.SalesID == "" || p1.SalesNo == "" {
		t.Fatalf("내부키·사업번호 둘 다 필요: id=%s no=%s", p1.SalesID, p1.SalesNo)
	}
	if p1.SalesID[:3] != "SP-" {
		t.Fatalf("sales_id 유지: %s", p1.SalesID)
	}
	got, err := repo.Get(p1.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SalesNo != p1.SalesNo {
		t.Fatalf("Get sales_no=%s want %s", got.SalesNo, p1.SalesNo)
	}
	list, err := repo.ListFilter(SalesListFilter{Search: p1.SalesNo, IncludeClosed: true})
	if err != nil || len(list) != 1 || list[0].SalesID != p1.SalesID {
		t.Fatalf("검색 %s: n=%d err=%v", p1.SalesNo, len(list), err)
	}
}

func TestSalesNoBackfillThenCreateDoesNotCollide(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_no_bf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	a := &model.SalesProject{Name: "기존 A"}
	b := &model.SalesProject{Name: "기존 B"}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(b); err != nil {
		t.Fatal(err)
	}
	sep := time.Date(2026, 9, 3, 10, 0, 0, 0, time.Local)
	oct := time.Date(2026, 10, 1, 10, 0, 0, 0, time.Local)
	_, _ = db.Exec(`UPDATE sales_projects SET sales_no='', created_at=? WHERE sales_id=?`, sep.Format("2006-01-02 15:04:05"), a.SalesID)
	_, _ = db.Exec(`UPDATE sales_projects SET sales_no='', created_at=? WHERE sales_id=?`, sep.Add(time.Hour).Format("2006-01-02 15:04:05"), b.SalesID)
	c := &model.SalesProject{Name: "10월"}
	if err := repo.Create(c); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE sales_projects SET sales_no='', created_at=? WHERE sales_id=?`, oct.Format("2006-01-02 15:04:05"), c.SalesID)
	_, _ = db.Exec(`DELETE FROM id_sequences WHERE seq_key LIKE 'sales_no:%'`)

	applySalesNoV1(db)
	ga, _ := repo.Get(a.SalesID)
	gb, _ := repo.Get(b.SalesID)
	gc, _ := repo.Get(c.SalesID)
	if ga.SalesNo != "S2609-001" || gb.SalesNo != "S2609-002" || gc.SalesNo != "S2610-001" {
		t.Fatalf("backfill: %s %s %s", ga.SalesNo, gb.SalesNo, gc.SalesNo)
	}
	snapA, snapB := ga.SalesNo, gb.SalesNo
	applySalesNoV1(db)
	ga2, _ := repo.Get(a.SalesID)
	gb2, _ := repo.Get(b.SalesID)
	if ga2.SalesNo != snapA || gb2.SalesNo != snapB {
		t.Fatalf("두 번째 실행이 번호를 바꿈: %s %s", ga2.SalesNo, gb2.SalesNo)
	}

	n := &model.SalesProject{Name: "새 9월"}
	n.SalesNo, err = NextSalesNo(db, sep)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(n); err != nil {
		t.Fatal(err)
	}
	if n.SalesNo != "S2609-003" {
		t.Fatalf("채우기 뒤 새 번호 겹침: %s", n.SalesNo)
	}
	if n.SalesID == a.SalesID || n.SalesID == b.SalesID {
		t.Fatalf("sales_id 충돌")
	}
}
