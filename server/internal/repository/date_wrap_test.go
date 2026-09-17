package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestWorkStatusDoesNotWrapDatetimeColumns(t *testing.T) {
	b, err := os.ReadFile("work_status_repo.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, bad := range []string{
		"date(receipt_datetime) >=",
		"date(w.created_at)",
		"date(updated_at)",
	} {
		if strings.Contains(s, bad) {
			t.Fatalf("열을 date() 로 감싸면 인덱스를 못 쓴다: %s", bad)
		}
	}
}

func TestASSearchDoesNotWrapReceiptDatetime(t *testing.T) {
	b, err := os.ReadFile("as_search.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "date(ar.receipt_datetime) >=") || strings.Contains(s, "date(ar.receipt_datetime) <=") {
		t.Fatal("검색 기간 조건이 receipt_datetime 을 date() 로 감싼다")
	}
}

func TestDayTimeBounds(t *testing.T) {
	if got := dayTimeStart("2026-09-01"); got != "2026-09-01 00:00:00" {
		t.Fatalf("start=%s", got)
	}
	if got := dayTimeStart("2026-09-01 15:04:05"); got != "2026-09-01 00:00:00" {
		t.Fatalf("start from dt=%s", got)
	}
	if got := dayTimeNext("2026-09-01"); got != "2026-09-02 00:00:00" {
		t.Fatalf("next=%s", got)
	}
}

func TestLookupCacheCustomerAndCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lookup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	c := &model.Customer{OrgName: "캐시기관", OfficialName: "캐시기관", IsActive: true}
	if err := NewCustomerRepo(db).Create(c); err != nil {
		t.Fatal(err)
	}
	if CustomerName(db, c.CustomerID) != "캐시기관" {
		t.Fatalf("create 후 이름이 캐시에 없다: %q", CustomerName(db, c.CustomerID))
	}

	cd := &model.Code{CodeGroup: "lookup_test", CodeValue: "x", CodeName: "룩업", SortOrder: 1, IsActive: true}
	if err := NewCodeRepo(db).Create(cd); err != nil {
		t.Fatal(err)
	}
	got, err := NewCodeRepo(db).ActiveByGroup("lookup_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CodeName != "룩업" {
		t.Fatalf("코드 캐시 미갱신: %+v", got)
	}

	u := &model.User{Username: "lookup_u", PasswordHash: "x", FullName: "캐시사용자", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	if UserName(db, u.UserID) != "캐시사용자" {
		t.Fatalf("사용자 캐시 미갱신: %q", UserName(db, u.UserID))
	}
}

func TestASKbDoesNotQueryCustomerPerHit(t *testing.T) {
	b, err := os.ReadFile("as_kb.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "FROM customers WHERE customer_id=?") {
		t.Fatal("KMS 검색이 건수만큼 고객 이름을 조회한다")
	}
}
