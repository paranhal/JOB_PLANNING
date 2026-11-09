package leaveimport

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/leaveledger"
)

func testImportDB(t *testing.T) (*leaveledger.Service, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	led := leaveledger.New(db)
	led.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, time.Local) }
	return led, repository.NewUserRepo(db)
}

func addUser(t *testing.T, users *repository.UserRepo, username, name, org string) *model.User {
	t.Helper()
	u := &model.User{Username: username, PasswordHash: "x", FullName: name, Role: model.RoleTech, IsActive: true, OrgID: org, ProfileDone: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func Test95_161D_MatchIdempotentTotalsHireConflict(t *testing.T) {
	led, users := testImportDB(t)
	a := addUser(t, users, "ga", "가상갑", "O01")
	b := addUser(t, users, "na", "가상을", "O01")
	d1 := addUser(t, users, "d1", "동명이", "O01")
	d2 := addUser(t, users, "d2", "동명이", "O01")
	c := addUser(t, users, "cf", "입사충돌", "O01")
	if err := led.SetHireDate(c.UserID, "O01", "2015-06-01"); err != nil {
		t.Fatal(err)
	}
	_ = d1
	_ = d2

	raw, err := BuildFixture(2026)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw, 2026)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := Match(led.DB, parsed)
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]PreviewRow{}
	for _, r := range prev {
		st[r.Name] = r
	}
	if st["가상갑"].Status != "matched" || st["가상갑"].UserID != a.UserID {
		t.Fatalf("%+v", st["가상갑"])
	}
	if st["가상을"].Status != "matched" || st["가상을"].UserID != b.UserID {
		t.Fatalf("%+v", st["가상을"])
	}
	if st["동명이"].Status != "duplicate" || st["동명이"].UserID != "" {
		t.Fatalf("dup %+v", st["동명이"])
	}
	if st["없는사람"].Status != "unmatched" {
		t.Fatalf("%+v", st["없는사람"])
	}
	if st["미등록자"].Status != "unregistered" {
		t.Fatalf("%+v", st["미등록자"])
	}
	if !st["입사충돌"].HireConflict || st["입사충돌"].HireExisting != "2015-06-01" {
		t.Fatalf("hire %+v", st["입사충돌"])
	}
	if st["표기오류"].Status != "error" {
		t.Fatalf("%+v", st["표기오류"])
	}

	prev = ApplyPicks(led.DB, prev, map[int]string{st["동명이"].ExcelRow: d1.UserID})
	rep, err := Apply(led, prev, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Applied != 4 {
		t.Fatalf("applied %d %+v", rep.Applied, rep)
	}
	for _, ck := range rep.Checks {
		if !ck.OK {
			t.Fatalf("total %s excel=%s ledger=%s", ck.UserID, ck.ExcelRemain, ck.LedgerRemain)
		}
	}
	if led.GrantCount(st["없는사람"].UserID) != 0 {
		t.Fatal("unmatched loaded")
	}
	unreg := st["미등록자"]
	if unreg.UserID != "" && led.GrantCount(unreg.UserID) != 0 {
		t.Fatal("empty grant loaded")
	}
	p, _ := led.LoadProfile(led.DB, c.UserID)
	if p.HireDate != "2015-06-01" {
		t.Fatalf("hire overwritten %s", p.HireDate)
	}
	p2, _ := led.LoadProfile(led.DB, a.UserID)
	if p2.HireDate != "2020-03-01" {
		t.Fatalf("hire fill %s", p2.HireDate)
	}

	g1 := led.GrantCount(a.UserID)
	rep2, err := Apply(led, prev, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if led.GrantCount(a.UserID) != g1 {
		t.Fatal("not idempotent grants")
	}
	for _, ck := range rep2.Checks {
		if !ck.OK {
			t.Fatalf("idempotent remain %s", ck.UserID)
		}
	}
	msg := CountsOnly(rep2)
	if msg == "" || strings.Contains(msg, "가상갑") {
		t.Fatalf("audit %s", msg)
	}
}
