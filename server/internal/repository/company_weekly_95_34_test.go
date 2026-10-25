package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeekly95_34MissingASAndJanggok(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-diag.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(user_id,username,password_hash,full_name,role,is_active,org_id,profile_done)
		VALUES ('U-KIM','kimsh','x','김상현','tech',1,'O03',1),
		       ('U-YU','yuds','x','유대성','tech',1,'O03',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C-JG','장곡초','장곡초',1,'O03'),('C-AS','AS고객','AS고객',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES
		('WP-005','충청남도교육청 테크센터 운영 사업','active',?,'maintenance',0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, data_origin, is_test, created_at, updated_at)
		VALUES ('AS-R089','R2610-089','C-AS','2026-10-15 09:00:00','2026-10-15 13:36:00','completed',
			'유대성','U-YU','O03','장애','app',0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id)
		VALUES ('MP-JG',2026,'도서관계획','approved',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits(
		visit_id, plan_id, visit_date, customer_id, sort_order, assignee, assignee_user_id,
		product_type, completed, project_id, is_test)
		VALUES ('V-JG','MP-JG','2026-10-23','C-JG',0,'김상현','U-KIM','KLAS',0,'WP-005',0)`); err != nil {
		t.Fatal(err)
	}

	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraftOrg(time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.CompanyWeeklyUnassigned{}
	for _, u := range draft.Unassigned {
		byID[u.ID] = u
	}
	as, ok := byID["AS-R089"]
	if !ok {
		t.Fatalf("R2610-089 미포함 n=%d %+v", len(draft.Unassigned), draft.Unassigned)
	}
	if as.Reason != model.UnassignedReasonNoProject {
		t.Fatalf("AS Reason=%q", as.Reason)
	}
	v, ok := byID["V-JG"]
	if !ok {
		t.Fatalf("장곡초 미포함 %+v", draft.Unassigned)
	}
	if v.ProjectID != "WP-005" {
		t.Fatalf("장곡초 pid=%s", v.ProjectID)
	}
	if len(draft.UnassignedForeign) != 1 || draft.UnassignedForeign[0].ProjectID != "WP-005" {
		t.Fatalf("타 조직 그룹 %+v", draft.UnassignedForeign)
	}
	for _, g := range draft.UnassignedNoRow {
		if g.ProjectID == "WP-005" {
			t.Fatal("WP-005 가 기본 목록에 있다")
		}
	}
}

func TestCompanyWeekly95_34MeetingVsReportCounts(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-meet.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C34','회의교','회의교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind) VALUES
		('WP-DEV34','개발사업','active','O03','dev')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,org_id,symptom,created_at,updated_at)
		VALUES ('AS34','AS34','C34','2026-08-12 10:00:00','2026-08-12 16:00:00','completed','O03','장애',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MP34',2026,'p','approved','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,completed,completed_date)
		VALUES ('V34','MP34','2026-08-13','C34',1,'2026-08-13')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,complete_date,work_date)
		VALUES ('T-AD34','admin','행정34','complete','O03','2026-08-14','2026-08-14'),
		       ('T-PW34','project_work','작업34','complete','O03','2026-08-11','2026-08-11')`); err != nil {
		t.Fatal(err)
	}

	anchor := time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)
	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraftOrg(anchor, "O03")
	if err != nil {
		t.Fatal(err)
	}
	p := CompanyWeeklyPeriod(anchor)
	meet, err := NewWorkBoardRepo(db).collectCompletedRange("", nil, p.PrevFrom, p.PrevTo)
	if err != nil {
		t.Fatal(err)
	}
	meetN := 0
	meetIDs := map[string]bool{}
	for _, it := range meet {
		switch strings.ToLower(it.Prefix) {
		case "as", "maintenance", "mnt", "정기점검", "admin", "support", "project_work", "작업", "행정":
			meetN++
			meetIDs[it.RefID] = true
		default:
			if it.RefID == "AS34" || it.RefID == "V34" || it.RefID == "T-AD34" || it.RefID == "T-PW34" {
				meetN++
				meetIDs[it.RefID] = true
			}
		}
	}
	repIDs := map[string]bool{}
	for _, u := range draft.Unassigned {
		if u.Bucket == "prev" {
			repIDs[u.ID] = true
		}
	}
	for _, id := range []string{"AS34", "V34", "T-AD34", "T-PW34"} {
		if !repIDs[id] {
			t.Fatalf("보고서 전주 미포함 %s unassigned=%+v meet=%d", id, draft.Unassigned, meetN)
		}
	}
	if len(repIDs) != 4 {
		t.Fatalf("보고서 prev=%d %+v", len(repIDs), repIDs)
	}
}

func TestCompanyWeekly95_34DevKindUnassignedThenRow(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-kind.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind) VALUES
		('WP-020','테크센터 개발','active','O03','dev'),
		('WP-BLD','구축사업','active','O03','build')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date) VALUES
		('T-DEV','admin','개발업무','complete','WP-020','O03','2026-08-12','2026-08-12'),
		('T-BLD','project_work','구축업무','complete','WP-BLD','O03','2026-08-13','2026-08-13')`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	draft, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]model.CompanyWeeklyUnassigned{}
	for _, u := range draft.Unassigned {
		by[u.ID] = u
	}
	if by["T-DEV"].ProjectKind != model.ProjectKindDev || by["T-BLD"].ProjectKind != model.ProjectKindBuild {
		t.Fatalf("종류 배지 %+v", by)
	}
	if _, err := repo.AddWeeklyReportRowsFromProjects("O03", []string{"WP-020"}, false); err != nil {
		t.Fatal(err)
	}
	draft, err = repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mapIDs(draft)["T-DEV"]; ok {
		t.Fatal("행 추가 후에도 미포함")
	}
	var onCard bool
	for _, row := range draft.Rows {
		if row.ProjectID == "WP-020" && strings.Contains(row.PrevText, "개발업무") {
			onCard = true
		}
	}
	if !onCard {
		t.Fatalf("dev 행 카드에 없음 %+v", draft.Rows)
	}
}

func mapIDs(d model.CompanyWeeklyDraft) map[string]bool {
	m := map[string]bool{}
	for _, u := range d.Unassigned {
		m[u.ID] = true
	}
	return m
}

func TestCompanyWeekly95_34IsTestExcluded(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C-T','테스트고객','테스트고객',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_test) VALUES
		('WP-T','테스트숨김','active','O03',1),
		('WP-OK','실사업','active','O03',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,is_test) VALUES
		('T-HIDE','admin','숨김','complete','WP-T','O03','2026-08-12',1),
		('T-OK','admin','실무','complete','WP-OK','O03','2026-08-12',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id,as_number,customer_id,complete_datetime,status,org_id,is_test,created_at,updated_at)
		VALUES ('AS-T','AS-T','C-T','2026-08-12 10:00:00','completed','O03',1,datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	draft, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	ids := mapIDs(draft)
	if ids["T-HIDE"] || ids["AS-T"] {
		t.Fatalf("테스트 건이 미포함에 있음 %+v", draft.Unassigned)
	}
	if !ids["T-OK"] {
		t.Fatal("실무 누락")
	}
	list, err := repo.ListWeeklyRowProjects("O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ProjectID == "WP-T" {
			t.Fatal("테스트 사업이 선택 목록에 있다")
		}
	}
	wb := NewWBRepo(db)
	ps, err := wb.ListProjects(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ProjectID == "WP-T" {
			t.Fatal("ListProjects 에 테스트 사업")
		}
	}
}

func TestCompanyWeekly95_34MigrationIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_test) VALUES
		('WP-TN','(TEST) 충청남도교육청 테크센터 운영 사업','active',?,0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,is_test) VALUES
		('T-SI','admin','SI13','waiting','WP-TN','O03',0),
		('T-O01','admin','O01-15','waiting','WP-TN',?,0),
		('T-O02','admin','O02-4','waiting','WP-TN','O02',0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	up, err := testProjectFlagMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if testProjectFlagHasSeedInsert(up) {
		t.Fatal("084 INSERT 시드")
	}
	first, err := ApplyTestProjectNameFlag(db)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProjectsMarked != 1 {
		t.Fatalf("사업 표시 %d", first.ProjectsMarked)
	}
	if first.TasksMarked != 1 {
		t.Fatalf("타조직 업무 표시 %d want 1 (O01·O02 유지)", first.TasksMarked)
	}
	second, err := ApplyTestProjectNameFlag(db)
	if err != nil {
		t.Fatal(err)
	}
	if second.ProjectsMarked != 0 || second.TasksMarked != 0 {
		t.Fatalf("멱등 실패 %+v", second)
	}
	var pflag, si, o01, o02 int
	_ = db.QueryRow(`SELECT is_test FROM work_projects WHERE project_id='WP-TN'`).Scan(&pflag)
	_ = db.QueryRow(`SELECT is_test FROM work_tasks WHERE task_id='T-SI'`).Scan(&si)
	_ = db.QueryRow(`SELECT is_test FROM work_tasks WHERE task_id='T-O01'`).Scan(&o01)
	_ = db.QueryRow(`SELECT is_test FROM work_tasks WHERE task_id='T-O02'`).Scan(&o02)
	if pflag != 1 || si != 1 || o01 != 0 || o02 != 0 {
		t.Fatalf("flag p=%d si=%d o01=%d o02=%d", pflag, si, o01, o02)
	}
}

func TestCompanyWeekly95_34ProjectKindChangeReflects(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cw34-pk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	pr := NewProjectRepo(db)
	p := &model.WorkProject{Name: "종류변경", Status: model.WBProjectActive, OrgID: "O03", ProjectKind: model.ProjectKindDev, ContractAmount: 1}
	if err := pr.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date) VALUES
		('T-K','admin','종류업무','complete',?,'O03','2026-08-12')`, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	st := NewStatsRepo(db)
	if _, err := st.AddWeeklyReportRowsFromProjects("O03", []string{p.ProjectID}, false); err != nil {
		t.Fatal(err)
	}
	before, err := st.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, row := range before.Rows {
		if row.ProjectID == p.ProjectID && strings.Contains(row.PrevText, "종류업무") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("변경 전 카드 %+v", before.Rows)
	}
	if err := pr.SetProjectKind(p.ProjectID, model.ProjectKindMaintenance, "t"); err != nil {
		t.Fatal(err)
	}
	got, err := pr.Get(p.ProjectID)
	if err != nil || got.ProjectKind != model.ProjectKindMaintenance {
		t.Fatalf("종류 저장 %+v %v", got, err)
	}
	after, err := st.BuildCompanyWeeklyDraftOrg(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	saw = false
	for _, row := range after.Rows {
		if row.ProjectID == p.ProjectID && strings.Contains(row.PrevText, "종류업무") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("종류 변경 후 카드 누락 %+v", after.Rows)
	}
}
