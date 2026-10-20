package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMaintenancePlanOrgYearUnique(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "mnt_org.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if n := CountEmptyOrgRows(db, "maintenance_plans"); n != 0 {
		t.Fatalf("empty org plans on fresh db: %d", n)
	}

	repo := NewMaintenanceRepo(db)
	lib, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "도서관")
	if err != nil || lib == nil || lib.OrgID != model.OrgIDLibrary {
		t.Fatalf("library plan: %+v err=%v", lib, err)
	}
	if _, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "중복"); err == nil {
		t.Fatal("same org+year must fail")
	}

	orgRepo := NewOrgRepo(db)
	mediaOrg, err := orgRepo.Create("미디어사업팀", "미디어", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	media, err := repo.CreatePlanForOrg(mediaOrg.OrgID, 2026, "미디어")
	if err != nil || media == nil {
		t.Fatalf("media same year: %v", err)
	}

	libList, err := repo.ListPlansInOrg(model.OrgIDLibrary)
	if err != nil || len(libList) != 1 || libList[0].PlanID != lib.PlanID {
		t.Fatalf("library list %+v err=%v", libList, err)
	}
	mediaList, err := repo.ListPlansInOrg(mediaOrg.OrgID)
	if err != nil || len(mediaList) != 1 || mediaList[0].PlanID != media.PlanID {
		t.Fatalf("media list %+v err=%v", mediaList, err)
	}

	got, err := repo.GetPlanByYearInOrg(model.OrgIDLibrary, 2026)
	if err != nil || got == nil || got.PlanID != lib.PlanID {
		t.Fatalf("get library year: %+v %v", got, err)
	}
	if other, _ := repo.GetPlanByYearInOrg(mediaOrg.OrgID, 2026); other == nil || other.PlanID != media.PlanID {
		t.Fatal("media year plan missing")
	}

	copied, err := repo.CopyPlan(lib.PlanID, 2027, "")
	if err != nil || copied.OrgID != model.OrgIDLibrary || copied.PlanYear != 2027 {
		t.Fatalf("copy %+v err=%v", copied, err)
	}
	if _, err := repo.CopyPlan(lib.PlanID, 2026, ""); err == nil {
		t.Fatal("copy onto own year must fail")
	}

	emptyID := "mpl_emptyorg"
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES (?, 2025, '빈조직', 'draft', '')`, emptyID); err != nil {
		t.Fatal(err)
	}
	if CountEmptyOrgRows(db, "maintenance_plans") != 1 {
		t.Fatalf("empty org count want 1 got %d", CountEmptyOrgRows(db, "maintenance_plans"))
	}
	libAll, _ := repo.ListPlansInOrg(model.OrgIDLibrary)
	for _, p := range libAll {
		if p.PlanID == emptyID {
			t.Fatal("empty org plan must not appear as library")
		}
	}
	if p, _ := repo.GetPlan(emptyID); p == nil || p.OrgID != "" {
		t.Fatal("empty org plan row must stay empty")
	}
}

func TestMaintenancePlanEmptyOrgNotBackfilledOnInit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mnt_empty.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES ('mpl_keep', 2024, '빈', 'draft', '')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	var org string
	if err := db2.QueryRow(`SELECT COALESCE(org_id,'') FROM maintenance_plans WHERE plan_id='mpl_keep'`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if org != "" {
		t.Fatalf("InitDB must not backfill empty org, got %q", org)
	}
	if n := CountEmptyOrgRows(db2, "maintenance_plans"); n != 1 {
		t.Fatalf("empty count %d", n)
	}
}
