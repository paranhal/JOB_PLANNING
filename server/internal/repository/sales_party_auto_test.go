package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestEnsureCustomerPartiesFromCounterparts(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "party_auto.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "세종 RFID", ProspectName: "세종시립도서관", IsTentativeName: true}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}

	act := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: "2026-08-19", ActivityType: "visit",
		Title: "첫 미팅", Counterparts: "김철수, 이영희", OurMembers: "관리자",
	}
	if err := repo.CreateActivity(act, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	parties, err := repo.ListParties(p.SalesID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.CustomerPartyHintNames(parties)) != 2 {
		t.Fatalf("두 명이 안 생겼다: %+v", parties)
	}
	for _, pt := range parties {
		if pt.PartyType != model.SalesPartyCustomer {
			continue
		}
		if !pt.IsAuto || !pt.NeedsSupplement() {
			t.Fatalf("is_auto/보완 필요 아님: %+v", pt)
		}
		if pt.OrgName != "세종시립도서관" {
			t.Fatalf("org_name=%q", pt.OrgName)
		}
	}

	act2 := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: "2026-08-20", ActivityType: "call",
		Title: "재통화", Counterparts: "김철수", OurMembers: "관리자",
	}
	if err := repo.CreateActivity(act2, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	parties, _ = repo.ListParties(p.SalesID, true)
	if n := len(model.CustomerPartyHintNames(parties)); n != 2 {
		t.Fatalf("같은 이름인데 행이 늘었다 n=%d %+v", n, parties)
	}

	var kim *model.SalesParty
	for i := range parties {
		if parties[i].PersonName == "김철수" && parties[i].IsActive {
			kim = &parties[i]
			break
		}
	}
	if kim == nil {
		t.Fatal("김철수 활성 행이 없다")
	}
	neu := &model.SalesParty{PartyType: model.SalesPartyCustomer, OrgName: "세종시립도서관", PersonName: "이후임", PartyRole: "working"}
	if err := repo.ReplaceParty(kim.PartyID, neu, "전배", "관리자"); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListParties(p.SalesID, true)
	var oldKim, newPerson bool
	for _, pt := range after {
		if pt.PersonName == "김철수" && !pt.IsActive {
			oldKim = true
		}
		if pt.PersonName == "이후임" && pt.IsActive {
			newPerson = true
		}
	}
	if !oldKim || !newPerson {
		t.Fatalf("교체 후 옛 사람이 사라졌다: %+v", after)
	}
}

func TestBackfillActivityCounterpartsToParties(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "party_backfill.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "옛 글자만 상대", ProspectName: "고객사", IsTentativeName: true}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO sales_activities (
			activity_id, sales_id, activity_date, activity_type, title, counterparts, created_by)
		VALUES ('SA-OLD1', ?, '2026-07-01', 'visit', '예전 미팅', '박민수, 최은정', 'migration')`, p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.BackfillActivityCounterpartsToParties("migration"); err != nil {
		t.Fatal(err)
	}
	parties, err := repo.ListParties(p.SalesID, true)
	if err != nil {
		t.Fatal(err)
	}
	names := model.CustomerPartyHintNames(parties)
	if len(names) != 2 {
		t.Fatalf("이관 결과 %v", names)
	}
	if err := repo.BackfillActivityCounterpartsToParties("migration"); err != nil {
		t.Fatal(err)
	}
	parties, _ = repo.ListParties(p.SalesID, true)
	if n := len(model.CustomerPartyHintNames(parties)); n != 2 {
		t.Fatalf("이관을 두 번 돌렸더니 늘었다 n=%d", n)
	}
}
