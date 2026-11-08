package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_145_ResolveDisplayProject(t *testing.T) {
	site := map[string][]string{"C1": {"WP1"}, "C3": {"WP1", "WP2"}}
	pid, via, need := repository.ResolveMeetingDisplayProject(model.WorkListItem{ProjectID: "WP9", CustomerID: "C1"}, site)
	if pid != "WP9" || via || need {
		t.Fatalf("저장된 사업 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C1"}, site)
	if pid != "WP1" || !via || need {
		t.Fatalf("사이트 1건 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C3"}, site)
	if pid != "" || via || !need {
		t.Fatalf("사이트 여러 건 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C0"}, site)
	if pid != "" || via || need {
		t.Fatalf("사이트 없음 %s via=%v need=%v", pid, via, need)
	}
}

func Test95_145_GroupUsesSiteWithoutWriting(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m145.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES
		('C1','해미초','해미초',1,'O03'),('C2','미연결초','미연결초',1,'O03'),('C3','중복초','중복초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_test) VALUES
		('WP1','본사업','active','O03',0),('WP2','다른사업','active','O03',0),('WPT','테스트','active','O03',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active) VALUES
		('S1','WP1','C1','해미초','in_progress','O03',1),
		('S2','WP1','C3','중복A','in_progress','O03',1),
		('S3','WP2','C3','중복B','in_progress','O03',1),
		('ST','WPT','C1','테스트사이트','in_progress','O03',1)`); err != nil {
		t.Fatal(err)
	}
	h := &MeetingHandler{wb: repository.NewWBRepo(db)}
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixAS, RefID: "A1", CustomerID: "C1", Title: "사이트보완"},
		{Prefix: model.WorkPrefixAS, RefID: "A2", CustomerID: "C2", Title: "미연결"},
		{Prefix: model.WorkPrefixAS, RefID: "A3", ProjectID: "WP1", CustomerID: "C1", Title: "저장됨"},
		{Prefix: model.WorkPrefixAS, RefID: "A4", CustomerID: "C3", Title: "복수사이트"},
	}
	groups, need, siteN := h.groupWorkItemsByProject(items)
	if need != 1 {
		t.Fatalf("사업 선택 필요 %d", need)
	}
	if siteN != 1 {
		t.Fatalf("사이트 보완 %d", siteN)
	}
	var wp, un meetingProjectGroup
	for _, g := range groups {
		if g.ProjectID == "WP1" {
			wp = g
		}
		if g.Unlinked {
			un = g
		}
	}
	if wp.Count != 2 || !wp.SiteBased || wp.Label != "본사업" {
		t.Fatalf("본사업 그룹 %+v", wp)
	}
	if un.Count != 2 || un.Label != "사업 미연결" {
		t.Fatalf("미연결 %+v", un)
	}
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_projects WHERE project_id='WP1'`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	_ = pid
}

func Test95_145_BoardHTMLSiteFill(t *testing.T) {
	fromto := "from=2026-10-21&to=2026-10-21&view=project"
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m145h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa := &model.User{Username: "oa145", PasswordHash: "x", FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES ('c145a','해미초','해미초',1,'O03'),('c145b','미연결초','미연결초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP145','본사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('S145','WP145','c145a','해미초','in_progress','O03',1)`); err != nil {
		t.Fatal(err)
	}
	day := "2026-10-21"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, symptom, updated_at, org_id
	) VALUES
		('as-145a','R145-A','c145a','2026-10-05',?,1,'in_progress','SI관리자','사이트보완',?,'O03'),
		('as-145b','R145-B','c145b','2026-10-05',?,1,'in_progress','SI관리자','미연결',?,'O03')`,
		day, day+" 09:00:00", day, day+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	ee := echo.New()
	ee.Renderer = NewRenderer()
	hh := New(db)
	g := ee.Group("")
	g.Use(hh.Auth.AuthMiddleware)
	g.GET("/meeting/board", hh.Meeting.Board)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/meeting/board?"+fromto, nil)
	req.AddCookie(jwtCookieUserClaims(t, oa))
	ee.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String()[:minInt(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	if strings.Contains(body, `data-site-rule="1"`) {
		t.Fatal("상시 사이트 등록 기준")
	}
	if !strings.Contains(body, `data-project-hint="1"`) || !strings.Contains(body, `data-unlinked-n="1"`) {
		t.Fatal("안내 상자 없음")
	}
	if !strings.Contains(body, "사이트 미등록 1") || !strings.Contains(body, "사업 선택 필요 0") {
		t.Fatal("안내 숫자")
	}
	if strings.Contains(body, `data-need-select="1"`) {
		t.Fatal("사이트 0건을 선택 필요로 셈")
	}
	if !strings.Contains(body, "본사업") || !strings.Contains(body, "R145-A") {
		t.Fatal("사이트 보완 사업 묶음 없음")
	}
	if !strings.Contains(body, "사업 미연결") || !strings.Contains(body, "R145-B") {
		t.Fatal("미연결 블록 없음")
	}
	var stored string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='as-145a'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Fatalf("소급 저장됨 %q", stored)
	}
}
