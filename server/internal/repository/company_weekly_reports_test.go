package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestCompanyWeeklyReportSaveConfirmEditIsolation(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cwr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	rows := []model.CompanyWeeklyRow{{RowKey: "M1", PrevText: "미디어비밀문구", PlanText: "계획"}}
	content := EncodeCompanyWeeklyContent(rows)
	if err := repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "s", content, "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "s2", content, "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if err != nil || saved.ReportID == "" || saved.SheetName != "s2" || saved.Status != CompanyWeeklyStatusDraft {
		t.Fatalf("이어서 저장 %+v err=%v", saved, err)
	}
	if err := repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "s2", content, "팀장", CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}
	saved, _ = repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if saved.Status != CompanyWeeklyStatusConfirmed {
		t.Fatalf("confirm %s", saved.Status)
	}
	if err := repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "s2", content, "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	saved, _ = repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if saved.Status != CompanyWeeklyStatusConfirmed {
		t.Fatalf("같은 내용 재저장은 확정 유지, got %s", saved.Status)
	}
	changed := EncodeCompanyWeeklyContent([]model.CompanyWeeklyRow{{RowKey: "M1", PrevText: "고침", PlanText: "계획"}})
	if err := repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "s2", changed, "팀장", CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	saved, _ = repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if saved.Status != CompanyWeeklyStatusDraft {
		t.Fatalf("수정 후 draft 복귀 got %s", saved.Status)
	}
	o03, err := repo.GetCompanyWeeklyReport("O03", "2026-08-17")
	if err != nil || o03.ReportID != "" {
		t.Fatalf("O03에 O02 보고서가 보이면 안 됨 %+v err=%v", o03, err)
	}
}
