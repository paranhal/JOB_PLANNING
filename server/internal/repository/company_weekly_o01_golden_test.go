package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeeklyO01AndOtherOrgSameBuilder(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw-same.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)
	o01, err := repo.BuildCompanyWeeklyDraftOrg(anchor, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if len(o01.Rows) != 7 {
		t.Fatalf("O01 시드 행=%d want 7", len(o01.Rows))
	}
	o03, err := repo.BuildCompanyWeeklyDraftOrg(anchor, "O03")
	if err != nil {
		t.Fatal(err)
	}
	if len(o03.Rows) != 0 {
		t.Fatalf("O03 시드 없이 행=%d", len(o03.Rows))
	}
}

func TestWeeklyViewHasNoOrgHardcode(t *testing.T) {
	files := []string{
		filepath.Join("..", "handler", "company_weekly_view.go"),
		filepath.Join("..", "handler", "company_weekly_sheet.go"),
		filepath.Join("..", "handler", "stats_company_weekly_ui.go"),
		"company_weekly_progress.go",
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, `== "O03"`) || strings.Contains(s, `== "O02"`) || strings.Contains(s, `org == model.OrgIDLibrary`) {
			t.Fatalf("%s 에 팀별 집계 분기가 있다", f)
		}
	}
}
