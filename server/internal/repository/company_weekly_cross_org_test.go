package repository

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

const (
	crossAnchor = "2026-08-17"
	libSchool   = "도서관학교A"
	libAS       = "도서관AS증상"
	libAdmin    = "도서관행정업무"
	libSales    = "도서관영업활동"
	libWork     = "도서관작업수행"
	siSchool    = "SI학교A"
	siAS        = "SIAS증상"
	siAdmin     = "SI행정업무"
)

func seedWeeklyCrossOrg(t *testing.T) (*sql.DB, *StatsRepo) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "cw-cross.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C-LIB',?,'도서관학교',1,?),('C-SI',?,'SI학교',1,'O03')`, libSchool, model.OrgIDLibrary, siSchool)
	mustExec(t, db, `INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('LIB-P1','LIB사업1','active',?),('LIB-P2','LIB사업2','active',?),
		('SI-P1','SI사업1','active','O03'),('SI-P2','SI사업2','active','O03')`,
		model.OrgIDLibrary, model.OrgIDLibrary)
	mustExec(t, db, `INSERT INTO maintenance_plans(plan_id,plan_year,org_id,status) VALUES
		('MP-LIB',2026,?,'approved'),('MP-SI',2026,'O03','approved')`, model.OrgIDLibrary)
	// 수행 조직 O01 인데 계획이 O03 — 계획 org 필터면 SI 보고서에 도서관 점검이 섞인다.
	insVisit := `INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,completed_date,project_id,org_id,assignee)
		VALUES(?,?,?,?,1,?,?,?,?,?)`
	if _, err := db.Exec(insVisit, "V-LIB-DONE", "MP-SI", "2026-08-12", "C-LIB", 1, "2026-08-12", "LIB-P1", model.OrgIDLibrary, "도서관담당"); err != nil {
		mustExec(t, db, `INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,completed_date,project_id,assignee)
			VALUES('V-LIB-DONE','MP-SI','2026-08-12','C-LIB',1,1,'2026-08-12','LIB-P1','도서관담당')`)
	}
	if _, err := db.Exec(insVisit, "V-SI-DONE", "MP-SI", "2026-08-12", "C-SI", 1, "2026-08-12", "SI-P1", "O03", "SI담당"); err != nil {
		mustExec(t, db, `INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,completed_date,project_id,assignee)
			VALUES('V-SI-DONE','MP-SI','2026-08-12','C-SI',1,1,'2026-08-12','SI-P1','SI담당')`)
	}
	if _, err := db.Exec(insVisit, "V-SI-ON-LIB", "MP-SI", "2026-08-13", "C-SI", 1, "2026-08-13", "LIB-P1", "O03", "SI담당"); err != nil {
		mustExec(t, db, `INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,completed_date,project_id,assignee)
			VALUES('V-SI-ON-LIB','MP-SI','2026-08-13','C-SI',1,1,'2026-08-13','LIB-P1','SI담당')`)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,project_id,org_id)
		VALUES('V-LIB-OPEN','MP-LIB','2026-08-12','C-LIB',2,0,'LIB-P2',?)`, model.OrgIDLibrary); err != nil {
		mustExec(t, db, `INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,completed,project_id)
			VALUES('V-LIB-OPEN','MP-LIB','2026-08-12','C-LIB',2,0,'LIB-P2')`)
	}
	mustExec(t, db, `INSERT INTO as_receipts(as_id,as_number,receipt_datetime,customer_id,symptom,status,assigned_to,org_id,complete_datetime,project_id,created_at,updated_at)
		VALUES
		('AS-LIB','R-LIB','2026-08-12 09:00:00','C-LIB',?,'complete','도서관담당',?,'2026-08-12 15:00:00','LIB-P1','2026-08-12 09:00:00','2026-08-12 15:00:00'),
		('AS-SI','R-SI','2026-08-12 09:00:00','C-SI',?,'complete','SI담당','O03','2026-08-12 15:00:00','SI-P1','2026-08-12 09:00:00','2026-08-12 15:00:00')`,
		libAS, model.OrgIDLibrary, siAS)
	mustExec(t, db, `INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,due_date,assignee)
		VALUES
		('WT-LIB-ADM','admin',?,'complete','LIB-P1',?,'2026-08-12','2026-08-12','2026-08-12','도서관담당'),
		('WT-SI-ADM','admin',?,'complete','SI-P1','O03','2026-08-12','2026-08-12','2026-08-12','SI담당'),
		('WT-LIB-PW','project_work',?,'complete','LIB-P2',?,'2026-08-12','2026-08-12','2026-08-12','도서관담당'),
		('WT-SI-PW','project_work','SI작업수행','complete','SI-P2','O03','2026-08-12','2026-08-12','2026-08-12','SI담당')`,
		libAdmin, model.OrgIDLibrary, siAdmin, libWork, model.OrgIDLibrary)
	if _, err := db.Exec(`INSERT INTO sales_projects(sales_id,name,stage,status,org_id) VALUES
		('SP-LIB','도서관영업','propose','active',?),('SP-SI','SI영업','propose','active','O03')`, model.OrgIDLibrary); err != nil {
		mustExec(t, db, `INSERT INTO sales_projects(sales_id,name,stage,status) VALUES
			('SP-LIB','도서관영업','propose','active'),('SP-SI','SI영업','propose','active')`)
	}
	mustExec(t, db, `INSERT INTO sales_activities(activity_id,sales_id,activity_date,activity_type,title) VALUES
		('SA-LIB','SP-LIB','2026-08-12','proposal',?),('SA-SI','SP-SI','2026-08-12','proposal','SI영업활동')`, libSales)
	mustExec(t, db, `INSERT INTO weekly_report_rows(row_key,sheet_row,display_name,row_kind,project_id,org_id,activity_kind,is_active,sort_order)
		VALUES
		('WR-SI-MAP',20,'SI가 가리킨 도서관사업','project','LIB-P1','O03','',1,0),
		('WR-SI-OWN',21,'SI사업1','project','SI-P1','O03','',1,1),
		('WR-SI-MNT',22,'정기점검·AS','activity',NULL,'O03','mnt_as',1,2),
		('WR-SI-AST',23,'자산관리','activity',NULL,'O03','asset',1,3),
		('WR-SI-TEC',24,'기술영업','activity',NULL,'O03','tech',1,4)`)
	return db, NewStatsRepo(db)
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
}

func libMarkers() []string {
	return []string{libSchool, libAS, libAdmin, libSales, libWork, "LIB사업1", "LIB사업2", "도서관담당"}
}

func draftHaystack(d model.CompanyWeeklyDraft) string {
	var b strings.Builder
	for _, row := range d.Rows {
		fmt.Fprintf(&b, "ROW %s name=%s pid=%s prev=%s plan=%s\n", row.RowKey, row.DisplayName, row.ProjectID, row.PrevText, row.PlanText)
	}
	for _, u := range d.Unassigned {
		fmt.Fprintf(&b, "UNASSIGNED kind=%s title=%s pid=%s pname=%s\n", u.Kind, u.Title, u.ProjectID, u.ProjectName)
	}
	for _, v := range d.IncompleteMnt {
		fmt.Fprintf(&b, "INCOMPLETE name=%s pid=%s date=%s\n", v.OrgName, v.ProjectID, v.VisitDate)
	}
	return b.String()
}

func weeklyHaystack(rep model.WeeklyReport) string {
	var b strings.Builder
	for _, p := range rep.PersonRows {
		fmt.Fprintf(&b, "PERSON %s receipt=%d done=%d unplanned=%d\n", p.Label, p.Receipt, p.Completed, p.Unplanned)
	}
	for _, e := range rep.Events {
		fmt.Fprintf(&b, "EVENT kind=%s no=%s cust=%s asg=%s content=%s\n", e.Kind, e.WorkNo, e.Customer, e.Assignee, e.Content)
	}
	return b.String()
}

func findMarkers(hay string, markers []string) []string {
	var hit []string
	for _, m := range markers {
		if strings.Contains(hay, m) {
			hit = append(hit, m)
		}
	}
	return hit
}

// TestWeeklyCrossOrgO03HasNoO01 수정 후 통과해야 하는 재현. 수정 전에는 섞인 마커를 에러에 나열한다.
func TestWeeklyCrossOrgO03HasNoO01(t *testing.T) {
	_, repo := seedWeeklyCrossOrg(t)
	anchor := time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)
	markers := libMarkers()
	var leaks []string

	draft, err := repo.BuildCompanyWeeklyDraftOrg(anchor, "O03")
	if err != nil {
		t.Fatal(err)
	}
	dh := draftHaystack(draft)
	if hit := findMarkers(dh, markers); len(hit) > 0 {
		leaks = append(leaks, "전사주간 "+strings.Join(hit, ","))
		t.Logf("전사주간 초안:\n%s", dh)
	}

	// 통계 주간 엑셀은 선택 주(월~일). 전주 실적(8/10~16)이 들어가게 그 주 월요일을 넘긴다.
	rep, err := repo.BuildWeeklyReportOrg(time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	wh := weeklyHaystack(rep)
	if hit := findMarkers(wh, markers); len(hit) > 0 {
		leaks = append(leaks, "통계주간엑셀 "+strings.Join(hit, ","))
		t.Logf("통계 주간:\n%s", wh)
	}

	if len(leaks) > 0 {
		t.Fatalf("O03 보고서에 O01 데이터가 나옴:\n%s", strings.Join(leaks, "\n"))
	}
}

func TestWeeklyCrossOrgEmptyOrgRejected(t *testing.T) {
	_, repo := seedWeeklyCrossOrg(t)
	_, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "")
	if err == nil {
		t.Fatal("조직 없이 전사 주간 초안이 만들어졌다")
	}
	_, err = repo.BuildWeeklyReportOrg(time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local), "")
	if err == nil {
		t.Fatal("조직 없이 통계 주간 엑셀이 만들어졌다")
	}
}
