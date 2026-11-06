package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeeklyUnlinkedRowT1T2T3(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw135.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES
		('C135','미연결사이트','미연결사이트',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP135','있는사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id)
		VALUES ('MP135',2026,'t','approved','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,source_type) VALUES
		('T-ADM','admin','행정없음','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-SUP','support','지원없음','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-WRK','project_work','작업없음','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-SAL','sales','영업없음','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-CAN','admin','취소됨','cancelled',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-SA','admin','영업활동원본','complete',NULL,'O03','2026-08-12','2026-08-12','sales_activity'),
		('T-DUP1','admin','경영지원 업무','complete',NULL,'O03','2026-08-11','2026-08-11',''),
		('T-DUP2','admin','경영지원 업무','complete',NULL,'O03','2026-08-12','2026-08-12',''),
		('T-DUP3','admin','경영지원 업무','complete',NULL,'O03','2026-08-13','2026-08-13',''),
		('T-PLAN','admin','차주예정', 'waiting',NULL,'O03','','2026-08-18',''),
		('T-PID','admin','사업있음','complete','WP135','O03','2026-08-12','2026-08-12',''),
		('T-O02','admin','타조직','complete',NULL,'O02','2026-08-12','2026-08-12','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,org_id)
		VALUES ('AS135','R135','C135','2026-08-12 10:00:00','2026-08-12 14:00:00','completed','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id,plan_id,visit_date,customer_id,product_type,completed,completed_date,assignee)
		VALUES ('V135','MP135','2026-08-12','C135','KLAS',1,'2026-08-12','담당')`); err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)

	before, err := repo.BuildCompanyWeeklyDraftOrg(anchor, "O03")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.UnassignedNoProject) == 0 {
		t.Fatalf("통칭 행 없이 no_project 가 비었다 %+v", before.Unassigned)
	}
	sawPlan := false
	for _, u := range before.Unassigned {
		if u.ID == "T-PLAN" {
			sawPlan = true
		}
	}
	if !sawPlan {
		t.Fatal("차주 예정 미배정 없음")
	}

	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "경영지원업무", Kind: "activity", ActivityKind: "unlinked", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertWeeklyReportRow(WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "두번째", Kind: "activity", ActivityKind: "unlinked", Active: true,
	}); err == nil || !strings.Contains(err.Error(), "조직당 하나만") {
		t.Fatalf("1개 제한: %v", err)
	}

	draft, err := repo.BuildCompanyWeeklyDraftOrg(anchor, "O03")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.UnassignedNoProject) != 0 {
		t.Fatalf("통칭 행 있는데 no_project=%+v", draft.UnassignedNoProject)
	}
	var row *model.CompanyWeeklyRow
	for i := range draft.Rows {
		if draft.Rows[i].ActivityKind == "unlinked" {
			row = &draft.Rows[i]
			break
		}
	}
	if row == nil {
		t.Fatal("unlinked 행 없음")
	}
	if row.DisplayName != "경영지원업무" {
		t.Fatalf("표시명=%q", row.DisplayName)
	}
	joined := row.PrevText + "\n" + row.PlanText
	for _, want := range []string{"[행정/지원] 행정없음", "[행정/지원] 지원없음", "[작업] 작업없음", "[영업] 영업없음", "[AS]", "[정기점검]", "경영지원 업무 ×3"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("본문 누락 %q\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "취소됨") || strings.Contains(joined, "영업활동원본") || strings.Contains(joined, "사업있음") || strings.Contains(joined, "타조직") {
		t.Fatalf("제외 대상이 본문에 있다\n%s", joined)
	}
	if !strings.Contains(row.PlanText, "차주예정") {
		t.Fatalf("차주 예정 없음 %s", row.PlanText)
	}
	keys := map[string]int{}
	for _, a := range append(append([]model.CompanyWeeklyAutoLine{}, row.AutoDone...), row.AutoPlan...) {
		keys[a.Key]++
	}
	for k, n := range keys {
		if n > 1 {
			t.Fatalf("키 중복 %s x%d", k, n)
		}
	}

	o02, err := repo.BuildCompanyWeeklyDraftOrg(anchor, "O02")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range o02.Rows {
		if r.ActivityKind == "unlinked" {
			t.Fatal("O02에 O03 통칭 행")
		}
	}
}

func TestCompanyWeeklyUnlinkedDoesNotChangeO01GoldenShape(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw135g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewStatsRepo(db)
	o01, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if len(o01.Rows) != 7 {
		t.Fatalf("O01 시드 행=%d want 7", len(o01.Rows))
	}
	for _, r := range o01.Rows {
		if r.ActivityKind == "unlinked" {
			t.Fatal("시드에 unlinked")
		}
	}
}
