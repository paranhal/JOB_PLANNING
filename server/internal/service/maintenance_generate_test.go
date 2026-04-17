package service

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestAutoGenerateMaintenance(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "gen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, id := range []string{"c1", "c2", "c3"} {
		_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
			id, "Org "+id, "Org "+id)
		if err != nil {
			t.Fatal(err)
		}
	}

	repo := repository.NewMaintenanceRepo(db)
	for i, id := range []string{"c1", "c2", "c3"} {
		cat := "normal"
		if i == 2 {
			cat = "fixed"
		}
		fr := ""
		if i == 2 {
			fr = fixedRuleLastMonday
		}
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, Region: "R1",
			HasKlas: true, EntryCategory: cat, FixedRule: fr,
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := repo.CreatePlan(2026, "자동배정 테스트")
	if err != nil {
		t.Fatal(err)
	}

	if err := AutoGenerateMaintenance(repo, p.PlanID); err != nil {
		t.Fatal(err)
	}

	visits, err := repo.ListVisits(p.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	// 월별: 고정 1 + flex 2 → 보통 36건. 마지막 월요일이 공휴일과 겹치는 달은 고정 건을 건너뛸 수 있어 35건 등이 될 수 있다.
	if len(visits) < 34 || len(visits) > 36 {
		t.Fatalf("expected about 36 visits, got %d", len(visits))
	}

	if err := repo.DeleteAutoVisits(p.PlanID); err != nil {
		t.Fatal(err)
	}
	visits, _ = repo.ListVisits(p.PlanID)
	if len(visits) != 0 {
		t.Fatalf("after DeleteAutoVisits: len=%d", len(visits))
	}
}

func TestWorkdaysForMonth(t *testing.T) {
	days := workdaysForMonth(2026, 3, time.Local)
	if len(days) < 10 {
		t.Fatalf("expected enough workdays, got %d", len(days))
	}
}
