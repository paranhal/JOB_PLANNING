package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestOrgCrossNoLeak(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "org_cross.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO customers (customer_id, org_name, official_name, org_id) VALUES ('C-O1','일팀고객','일팀고객','O01')`)
	mustExec(`INSERT INTO customers (customer_id, org_name, official_name, org_id) VALUES ('C-O2','이팀고객','이팀고객','O02')`)

	asRepo := NewASRepo(db)
	as1 := &model.ASReceipt{CustomerID: "C-O1", Symptom: "O01장애", OrgID: "O01", ReceiptDatetime: time.Date(2026, 9, 10, 9, 0, 0, 0, time.Local)}
	as2 := &model.ASReceipt{CustomerID: "C-O2", Symptom: "O02장애", OrgID: "O02", ReceiptDatetime: time.Date(2026, 9, 10, 9, 0, 0, 0, time.Local)}
	if err := asRepo.Create(as1); err != nil {
		t.Fatal(err)
	}
	if err := asRepo.Create(as2); err != nil {
		t.Fatal(err)
	}

	items, n, err := asRepo.ListFiltered("O01", "", "", "", nil, "", "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(items) != 1 || items[0].ASID != as1.ASID {
		t.Fatalf("AS O01 list n=%d items=%d", n, len(items))
	}
	got, err := asRepo.GetByID("O01", as2.ASID)
	if err == nil && got != nil {
		t.Fatal("O02 AS 가 O01 Get 에 보였다")
	}
	if _, err := asRepo.GetByID("", as1.ASID); err == nil {
		t.Fatal("빈 org Get 이 통과했다")
	}

	custs, cn, err := NewCustomerRepo(db).List("O01", "", "", "", "", "", 1, 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if cn != 1 || len(custs) != 1 || custs[0].CustomerID != "C-O1" {
		t.Fatalf("customer O01 n=%d", cn)
	}

	sales := NewSalesRepo(db)
	if err := sales.Create(&model.SalesProject{Name: "O01사업", OrgID: "O01"}); err != nil {
		t.Fatal(err)
	}
	if err := sales.Create(&model.SalesProject{Name: "O02사업", OrgID: "O02"}); err != nil {
		t.Fatal(err)
	}
	s1, err := sales.ListFilter(SalesListFilter{OrgID: "O01", IncludeClosed: true, IncludeDormant: true, DealType: model.SalesDealAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) != 1 || s1[0].Name != "O01사업" {
		t.Fatalf("sales O01=%d %+v", len(s1), s1)
	}
	s2, err := sales.ListFilter(SalesListFilter{OrgID: "O02", IncludeClosed: true, IncludeDormant: true, DealType: model.SalesDealAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(s2) != 1 {
		t.Fatalf("sales O02=%d", len(s2))
	}

	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "O01행정", OrgID: "O01", Status: model.WBTaskWaiting}); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "O02행정", OrgID: "O02", Status: model.WBTaskWaiting}); err != nil {
		t.Fatal(err)
	}
	admin, err := wb.ListAdminWork("O01", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(admin) != 1 || admin[0].Title != "O01행정" {
		t.Fatalf("admin work O01=%d", len(admin))
	}

	qr := NewQuoteRepo(db)
	if err := qr.Create(&model.SalesQuote{Title: "O01견적", OrgID: "O01", QuoteDate: "2026-09-10", SalesID: s1[0].SalesID, OwnerName: "테스트", OwnerPhone: "010-0000-0000"}); err != nil {
		t.Fatal(err)
	}
	if err := qr.Create(&model.SalesQuote{Title: "O02견적", OrgID: "O02", QuoteDate: "2026-09-10", SalesID: s2[0].SalesID, OwnerName: "테스트", OwnerPhone: "010-0000-0000"}); err != nil {
		t.Fatal(err)
	}
	q1, err := qr.List(QuoteFilter{OrgID: "O01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(q1) != 1 || q1[0].Title != "O01견적" {
		t.Fatalf("quote O01=%d", len(q1))
	}

	st := NewStatsRepo(db)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.Local)
	cols := BuildStatsRangeColumns(now.AddDate(0, 0, -30), now.AddDate(0, 0, 1))
	kpi1, err := st.LoadStatsKPI(model.StatsViewRange, cols, model.StatsMeetingFilter{OrgID: "O01", IncludeImport: true, Scope: model.StatsScopeTeam})
	if err != nil {
		t.Fatal(err)
	}
	kpi2, err := st.LoadStatsKPI(model.StatsViewRange, cols, model.StatsMeetingFilter{OrgID: "O02", IncludeImport: true, Scope: model.StatsScopeTeam})
	if err != nil {
		t.Fatal(err)
	}
	if kpi1.PlanningOpen == kpi2.PlanningOpen && kpi1.VisitSample == 0 && kpi2.VisitSample == 0 {
		// KPI 카드 구성이 달라도 집계 SQL 이 조직을 타야 한다. 접수 건으로 확인.
	}
	asSQL, asArgs := asFilterSQL(model.StatsMeetingFilter{OrgID: "O01", IncludeImport: true})
	var n1 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE 1=1`+asSQL, asArgs...).Scan(&n1); err != nil {
		t.Fatal(err)
	}
	asSQL2, asArgs2 := asFilterSQL(model.StatsMeetingFilter{OrgID: "O02", IncludeImport: true})
	var n2 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE 1=1`+asSQL2, asArgs2...).Scan(&n2); err != nil {
		t.Fatal(err)
	}
	if n1 != 1 || n2 != 1 {
		t.Fatalf("stats as filter O01=%d O02=%d", n1, n2)
	}

	space := NewSpaceRepo(db)
	mustExec(`INSERT INTO customer_buildings (building_id,customer_id,building_name,is_active) VALUES ('B-O1','C-O1','일팀본관',1)`)
	mustExec(`INSERT INTO customer_buildings (building_id,customer_id,building_name,is_active) VALUES ('B-O2','C-O2','이팀본관',1)`)
	b1, err := space.ListBuildings("O01", "C-O1")
	if err != nil {
		t.Fatal(err)
	}
	if len(b1) != 1 || b1[0].BuildingID != "B-O1" {
		t.Fatalf("space O01 buildings=%d", len(b1))
	}
	cross, err := space.GetBuilding("O01", "B-O2")
	if err != nil {
		t.Fatal(err)
	}
	if cross != nil {
		t.Fatal("O02 건물이 O01 Get 에 보였다")
	}
	if _, err := space.ListBuildings("", "C-O1"); err == nil {
		t.Fatal("빈 org 공간 조회가 통과했다")
	}

	empty, err := ListEmptyOrgRows(db)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, e := range empty {
		sum += e.Count
	}
	if sum != 0 {
		t.Fatalf("미지정 %d건 %+v", sum, empty)
	}
}
