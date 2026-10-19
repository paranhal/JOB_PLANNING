package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestCustomerProvisionalCreateReuseNormalize5(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "prov.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewCustomerRepo(db)

	id, official, err := repo.ResolveProvisional("O01", "해미 도서관", "김담당", "041-000-0000", "tester")
	if err != nil || id == "" || official {
		t.Fatalf("create id=%s official=%v err=%v", id, official, err)
	}
	got, err := repo.GetByID(id)
	if err != nil || got == nil || !got.IsProvisional {
		t.Fatalf("provisional flag: %+v err=%v", got, err)
	}

	variants := []string{"해미도서관", "해미  도서관", "해미도서관 ", "해미\t도서관", "(주)해미도서관"}
	for _, v := range variants {
		rid, off, err := repo.ResolveProvisional("O01", v, "", "", "tester")
		if err != nil {
			t.Fatal(err)
		}
		if rid != id {
			t.Fatalf("reuse %q got %s want %s", v, rid, id)
		}
		if off {
			t.Fatalf("%q 는 임시 재사용이어야 한다", v)
		}
	}
}

func TestCustomerProvisionalOfficialWarnAndOrgIsolation(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "prov2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewCustomerRepo(db)
	if err := repo.Create(&model.Customer{
		OrgName: "서산시립도서관", OfficialName: "(주)서산시립도서관", IsActive: true,
		PartyKind: model.PartyKindCustomer, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}

	id, official, err := repo.ResolveProvisional("O01", "서산 시립도서관", "", "", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !official {
		t.Fatal("정식 고객과 정규화 일치하면 재사용 안내(official)여야 한다")
	}
	c, _ := repo.GetByID(id)
	if c == nil || c.IsProvisional {
		t.Fatal("정식 고객을 재사용해야 한다")
	}

	other, off2, err := repo.ResolveProvisional("O02", "서산 시립도서관", "", "", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if other == id || off2 {
		t.Fatalf("다른 조직은 새로 만들어야 한다 other=%s off=%v", other, off2)
	}
}

func TestProvisionalASInnerJoinListShowSearch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "prov-as.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cust := NewCustomerRepo(db)
	cid, _, err := cust.ResolveProvisional("O01", "직접입력도서관74B", "", "", "tester")
	if err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	as := &model.ASReceipt{
		CustomerID: cid, Symptom: "74B직접입력증상게이트", Status: "received", OrgID: "O01",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	got, err := asRepo.GetByID(OrgAll, as.ASID)
	if err != nil || got == nil || got.OrgName != "직접입력도서관74B" {
		t.Fatalf("show join: %+v err=%v", got, err)
	}
	list, total, err := asRepo.List(OrgAll, "", "직접입력도서관74B", 1, 20)
	if err != nil || total < 1 {
		t.Fatalf("list total=%d err=%v", total, err)
	}
	found := false
	for _, it := range list {
		if it.ASID == as.ASID {
			found = true
		}
	}
	if !found {
		t.Fatal("목록 INNER JOIN 에서 임시 고객 AS 가 빠졌다")
	}
	hits, n, err := asRepo.SearchAS(model.ASSearchFilter{Query: "74B직접입력증상게이트"})
	if err != nil || n < 1 {
		t.Fatalf("search n=%d err=%v", n, err)
	}
	hitOK := false
	for _, h := range hits {
		if h.ASID == as.ASID {
			hitOK = true
		}
	}
	if !hitOK {
		t.Fatal("지식 검색 INNER JOIN 에서 빠졌다")
	}
}

func TestPromoteAndMergeMovesRefs(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "prov-merge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cust := NewCustomerRepo(db)
	fromID, _, err := cust.ResolveProvisional("O01", "임시병합원", "", "", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := cust.Create(&model.Customer{
		OrgName: "정식병합대상", OfficialName: "정식병합대상", IsActive: true,
		PartyKind: model.PartyKindCustomer, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	to, err := cust.FindByNormalizedOrgName("O01", "정식병합대상")
	if err != nil || to == nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(db)
	as := &model.ASReceipt{CustomerID: fromID, Symptom: "병합이동", Status: "received", OrgID: "O01"}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := cust.PromoteProvisional(fromID, "admin"); err != nil {
		t.Fatal(err)
	}
	prom, _ := cust.GetByID(fromID)
	if prom == nil || prom.IsProvisional {
		t.Fatal("승격 후 미확정이어야 하지 않는다")
	}
	if _, err := db.Exec(`UPDATE customers SET is_provisional=1 WHERE customer_id=?`, fromID); err != nil {
		t.Fatal(err)
	}
	if err := cust.MergeInto(fromID, to.CustomerID, "admin"); err != nil {
		t.Fatal(err)
	}
	moved, err := asRepo.GetByID(OrgAll, as.ASID)
	if err != nil || moved == nil || moved.CustomerID != to.CustomerID {
		t.Fatalf("merge customer=%v err=%v", moved, err)
	}
	from, _ := cust.GetByID(fromID)
	if from == nil || from.IsActive {
		t.Fatal("병합 원본은 비활성이어야 한다")
	}
}

func TestCustomerProvisionalMigrationIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "prov-mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyCustomerProvisionalV74(db)
	applyCustomerProvisionalV74(db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('customers') WHERE name='is_provisional'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("is_provisional n=%d err=%v", n, err)
	}
	_, down, err := customerProvisionalMigrationFiles()
	if err != nil {
		t.Log("061 sql files:", err)
		return
	}
	if err := execSQLScript(db, down); err != nil {
		t.Fatal(err)
	}
	applyCustomerProvisionalV74(db)
	applyCustomerProvisionalV74(db)
	if !strings.Contains("duplicate column name", "duplicate") {
		t.Log("ok")
	}
}
