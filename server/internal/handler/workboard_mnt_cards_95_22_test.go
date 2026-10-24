package handler

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestNewWorkMntCardsUsesPlanOrg(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mntcards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('c-o02','타조직고','타조직고',1,'O02','customer'),
		       ('c-o01','도서관','도서관',1,'O01','customer')`); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	p01, err := mnt.CreatePlanForOrg("O01", 2026, "도서관계획")
	if err != nil {
		t.Fatal(err)
	}
	p02, err := mnt.CreatePlanForOrg("O02", 2026, "미디어계획")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, completed)
		VALUES ('v-o01', ?, ?, 'c-o02', 1, 0, 0),
		       ('v-o02', ?, ?, 'c-o01', 1, 0, 0)`, p01.PlanID, day, p02.PlanID, day); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	cards, err := h.Workboard.newWorkMntCards("O01")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range cards {
		got[c.RefID] = true
	}
	if !got["v-o01"] {
		t.Fatalf("계획 조직 O01 방문 없음 %+v", got)
	}
	if got["v-o02"] {
		t.Fatalf("다른 계획 조직 방문이 있다 %+v", got)
	}
	as, mntCards, _, _, _, err := h.Workboard.registerPalette("O01", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = as
	pal := map[string]bool{}
	for _, c := range mntCards {
		pal[c.RefID] = true
	}
	if pal["v-o01"] != got["v-o01"] || pal["v-o02"] != got["v-o02"] {
		t.Fatalf("예전 화면과 새 모달 불일치 palette=%+v new=%+v", pal, got)
	}
	_ = model.WBSourceMaintenance
}
