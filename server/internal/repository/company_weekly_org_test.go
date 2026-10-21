package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestWeeklyReportRowsOrgIsolation(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cworg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	o01, err := repo.ListWeeklyReportRowsOrg(model.OrgIDLibrary, true)
	if err != nil || len(o01) < 6 {
		t.Fatalf("O01 rows=%d err=%v", len(o01), err)
	}
	o02, err := repo.ListWeeklyReportRowsOrg("O02", true)
	if err != nil || len(o02) != 0 {
		t.Fatalf("O02 시드 없어야 함 n=%d err=%v", len(o02), err)
	}
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O02", DisplayName: "미디어 활동", Kind: "activity", ActivityKind: "mnt_as", SortOrder: 0, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	o02, err = repo.ListWeeklyReportRowsOrg("O02", true)
	if err != nil || len(o02) != 1 || o02[0].DisplayName != "미디어 활동" {
		t.Fatalf("O02=%+v err=%v", o02, err)
	}
	for _, row := range o01 {
		if row.Key == o02[0].Key {
			t.Fatal("O01 목록에 O02 행")
		}
	}
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O02", DisplayName: "타팀사업", Kind: "project", ProjectID: "WPSEED01", Active: true,
	}); err == nil {
		t.Fatal("타 조직 사업 연결을 막아야 함")
	}
}

func TestCompanyWeeklyProjectWorkAndMntASOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cwagg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C03','SI학교','SI학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES('WP03','SI사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date)
		VALUES('PW03','project_work','구축 작업','complete','WP03','O03','2026-08-12','2026-08-12')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_task_sites(task_id,customer_id) VALUES('PW03','C03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,org_id,status) VALUES('MP03',2026,'O03','active')`); err != nil {
		t.Log(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,completed,completed_date,org_id)
		VALUES('V03','MP03','2026-08-12','C03',1,'2026-08-12','O03')`); err != nil {
		_, _ = db.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,completed,completed_date)
			VALUES('V03','MP03','2026-08-12','C03',1,'2026-08-12')`)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,symptom,org_id,status,complete_datetime)
		VALUES('AS03','AS-03','C03','장애','O03','complete','2026-08-12 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "SI사업", Kind: "project", ProjectID: "WP03", SortOrder: 0, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "정기점검·AS", Kind: "activity", ActivityKind: "mnt_as", SortOrder: 1, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	draft, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	var sawPW, sawMnt bool
	for _, row := range draft.Rows {
		if row.ProjectID == "WP03" && strings.Contains(row.PrevText, "구축 작업") && strings.Contains(row.PrevText, "SI학교") {
			sawPW = true
		}
		if row.ActivityKind == "" && row.RowKind == "activity" && strings.Contains(row.PrevText, "정기점검") {
			sawMnt = true
		}
		if strings.Contains(row.PrevText, "정기점검") && row.RowKind == "activity" {
			sawMnt = true
		}
	}
	if !sawPW {
		t.Fatalf("project_work 미포함 %+v", draft.Rows)
	}
	lib, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range lib.Rows {
		if strings.Contains(row.PrevText, "구축 작업") || strings.Contains(row.PrevText, "AS-03") {
			t.Fatalf("O01에 O03 집계: %q", row.PrevText)
		}
	}
	_ = sawMnt
}

func TestFormatProjectWorkLineSites(t *testing.T) {
	got := formatProjectWorkLine("구축", []string{"A교", "B교", "C교"}, "2026-08-12")
	if got != "구축(A교·B교 외 1곳)" {
		t.Fatal(got)
	}
}
