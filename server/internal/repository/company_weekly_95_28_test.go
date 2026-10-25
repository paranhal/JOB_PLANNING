package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func seedUnassignedNoRowFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP-P1','미행사업','active','O03'),
		('WP-P2','행있음사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date) VALUES
		('T-P1A','admin','행정1','complete','WP-P1','O03','2026-08-12','2026-08-12'),
		('T-P1B','project_work','작업1','complete','WP-P1','O03','2026-08-13','2026-08-13'),
		('T-P1C','admin','행정2','complete','WP-P1','O03','2026-08-14','2026-08-14'),
		('T-P2','project_work','작업P2','complete','WP-P2','O03','2026-08-12','2026-08-12'),
		('T-NP','admin','사업없음','complete',NULL,'O03','2026-08-11','2026-08-11')`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "행있음사업", Kind: "project", ProjectID: "WP-P2",
		NoLabel: "1", SortOrder: 0, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCompanyWeeklyUnassignedNoRowVsNoProject(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw28.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedUnassignedNoRowFixture(t, db)

	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.CompanyWeeklyUnassigned{}
	for _, u := range draft.Unassigned {
		byID[u.ID] = u
	}
	if _, ok := byID["T-P2"]; ok {
		t.Fatalf("행 있는 P2 업무가 미배정: %+v", draft.Unassigned)
	}
	for _, id := range []string{"T-P1A", "T-P1B", "T-P1C", "T-NP"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("미배정 누락 %s n=%d %+v", id, len(draft.Unassigned), draft.Unassigned)
		}
	}
	if len(draft.Unassigned) != 4 {
		t.Fatalf("미배정=%d want 4 %+v", len(draft.Unassigned), draft.Unassigned)
	}
	for _, id := range []string{"T-P1A", "T-P1B", "T-P1C"} {
		u := byID[id]
		if u.ProjectID != "WP-P1" || u.Reason != model.UnassignedReasonNoRow {
			t.Fatalf("%s Reason=%q pid=%q want no_row", id, u.Reason, u.ProjectID)
		}
	}
	if byID["T-NP"].ProjectID != "" || byID["T-NP"].Reason != model.UnassignedReasonNoProject {
		t.Fatalf("T-NP Reason=%q pid=%q want no_project", byID["T-NP"].Reason, byID["T-NP"].ProjectID)
	}
	if len(draft.UnassignedNoRow) != 1 || draft.UnassignedNoRow[0].ProjectID != "WP-P1" || draft.UnassignedNoRowTaskN != 3 {
		t.Fatalf("no_row groups %+v n=%d", draft.UnassignedNoRow, draft.UnassignedNoRowTaskN)
	}
	if len(draft.UnassignedNoProject) != 1 || draft.UnassignedNoProject[0].ID != "T-NP" {
		t.Fatalf("no_project %+v", draft.UnassignedNoProject)
	}
	if draft.UnassignedNoRowTaskN+len(draft.UnassignedNoProject)+draft.UnassignedForeignTaskN != len(draft.Unassigned) {
		t.Fatalf("구역 합 %d+%d+%d != %d", draft.UnassignedNoRowTaskN, len(draft.UnassignedNoProject), draft.UnassignedForeignTaskN, len(draft.Unassigned))
	}
}

func TestAddWeeklyReportRowsFromProjectsAndReagg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw28add.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedUnassignedNoRowFixture(t, db)
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP-A','추가A','active','O03'),('WP-B','추가B','active','O03'),('WP-X','비활성','active','O03')`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "비활성", Kind: "project", ProjectID: "WP-X", SortOrder: 5, Active: false,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-A", "WP-B"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Action != "inserted" || got[1].Action != "inserted" {
		t.Fatalf("%+v", got)
	}
	rows, err := repo.ListWeeklyReportRowsOrg("O03", true)
	if err != nil {
		t.Fatal(err)
	}
	var sawA, sawB bool
	for _, r := range rows {
		if r.ProjectID == "WP-A" {
			sawA = true
			if r.NoLabel != "2" || r.OrgID != "O03" {
				t.Fatalf("A no=%s org=%s", r.NoLabel, r.OrgID)
			}
		}
		if r.ProjectID == "WP-B" {
			sawB = true
			if r.NoLabel != "3" {
				t.Fatalf("B no=%s", r.NoLabel)
			}
		}
	}
	if !sawA || !sawB {
		t.Fatal("행 미생성")
	}
	again, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-A"}, false)
	if err != nil || len(again) != 1 || again[0].Action != "exists" {
		t.Fatalf("재요청 %+v %v", again, err)
	}
	act, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-X"}, false)
	if err != nil || len(act) != 1 || act[0].Action != "activated" {
		t.Fatalf("활성화 %+v %v", act, err)
	}
	if _, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-NOPE"}, false); err == nil {
		t.Fatal("목록 밖 사업")
	}
	if _, err := db.Exec(`UPDATE work_projects SET org_id=? WHERE project_id='WPSEED01'`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	foreign, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WPSEED01"}, false)
	if err != nil || len(foreign) != 1 || foreign[0].Err == "" {
		t.Fatalf("타 조직 확인 %+v %v", foreign, err)
	}
	if _, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WPSEED01"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-P1"}, false); err != nil {
		t.Fatal(err)
	}
	draft, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Unassigned) != 1 || draft.Unassigned[0].ID != "T-NP" {
		t.Fatalf("재집계 미배정 %+v", draft.Unassigned)
	}
	var onCard bool
	for _, row := range draft.Rows {
		if row.ProjectID == "WP-P1" && strings.Contains(row.PrevText, "행정") {
			onCard = true
		}
	}
	if !onCard {
		t.Fatalf("사업 카드 미이동 %+v", draft.Rows)
	}
	anchor := CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)).Anchor
	if err := repo.UpsertCompanyWeeklyReport("O03", anchor, "sheet", "{}", "t", CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-B"}, false); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.GetCompanyWeeklyReport("O03", anchor)
	if err != nil || saved.Status != CompanyWeeklyStatusConfirmed {
		t.Fatalf("확정 상태 %s %v", saved.Status, err)
	}
}

func TestSetTaskProjectWeeklyUnassigned(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw28link.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedUnassignedNoRowFixture(t, db)
	wb := NewWBRepo(db)
	before, after, err := wb.SetTaskProject("O03", "T-NP", "WP-P1")
	if err != nil {
		t.Fatal(err)
	}
	if before != "" || after == "" {
		t.Fatalf("before=%q after=%q", before, after)
	}
	t2, err := wb.GetTask("T-NP")
	if err != nil || t2.ProjectID != "WP-P1" {
		t.Fatalf("%+v %v", t2, err)
	}
}
