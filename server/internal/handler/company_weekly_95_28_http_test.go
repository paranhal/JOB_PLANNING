package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func mountWeekly28(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/company-weekly", h.Stats.ShowWeeklyReport)
	g.GET("/stats/reports", h.Stats.Reports)
	g.GET("/stats/company-weekly/rows", h.Stats.ListCompanyWeeklyRows)
	g.POST("/stats/company-weekly/rows", h.Stats.CreateCompanyWeeklyRow)
	g.POST("/stats/company-weekly/rows/from-projects", h.Stats.CreateCompanyWeeklyRowsFromProjects)
	g.POST("/stats/company-weekly/tasks/project", h.Stats.LinkWeeklyTasksProject)
	g.POST("/stats/company-weekly/rows/:key/update", h.Stats.UpdateCompanyWeeklyRow)
}

func TestWeeklyUnassignedHTTPAddLinkIsolate(t *testing.T) {
	accessPath := initTestAccessDB(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw28h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP-P1','미행사업','active','O03'),('WP-P2','행있음사업','active','O03'),
		('WP-A','추가A','active','O03'),('WP-B','추가B','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,assignee) VALUES
		('T-P1A','admin','행정1','complete','WP-P1','O03','2026-08-12','2026-08-12',''),
		('T-NP','admin','사업없음','waiting',NULL,'O03','','2026-08-12',''),
		('T-NP2','admin','사업없음2','waiting',NULL,'O03','','2026-08-12',''),
		('T-O01','admin','타조직','waiting',NULL,?,'','2026-08-12','')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	stats := repository.NewStatsRepo(db)
	if _, err := stats.InsertWeeklyReportRow(repository.WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "행있음사업", Kind: "project", ProjectID: "WP-P2", NoLabel: "1", Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)

	form := url.Values{"project_id": {"WP-A", "WP-B"}, "week": {"2026-08-10"}, "org": {"O03"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add %d %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "사업 2개") {
		t.Fatalf("loc %s", loc)
	}

	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(url.Values{"project_id": {"WP-NOPE"}}.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(bad, breq)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("목록 밖 want 400 got %d", bad.Code)
	}

	vis := httptest.NewRecorder()
	vreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(url.Values{"project_id": {"WP-A"}}.Encode()))
	vreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	vreq.AddCookie(jwtCookieRole(t, model.RoleVisionAdmin))
	e.ServeHTTP(vis, vreq)
	if vis.Code != http.StatusForbidden && vis.Code != http.StatusSeeOther && vis.Code != http.StatusBadRequest {
		t.Fatalf("비전 전체보기 want 금지 got %d", vis.Code)
	}

	cl := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(form.Encode()))
	creq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	creq.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	e.ServeHTTP(cl, creq)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", cl.Code)
	}

	ro := httptest.NewRecorder()
	rreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/from-projects", strings.NewReader(form.Encode()))
	rreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rreq.AddCookie(jwtCookieReadOnly(t, model.RoleOrgAdmin))
	e.ServeHTTP(ro, rreq)
	if ro.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 %d", ro.Code)
	}

	o01 := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/X-missing/update", strings.NewReader(url.Values{"display_name": {"x"}}.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(o01, oreq)
	if o01.Code != http.StatusNotFound {
		t.Fatalf("O01→O03 행 %d", o01.Code)
	}

	link := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(url.Values{
		"task_id": {"T-NP", "T-NP2"}, "project_id": {"WP-P1"}, "org": {"O03"}, "week": {"2026-08-10"},
	}.Encode()))
	lreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	lreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(link, lreq)
	if link.Code != http.StatusSeeOther {
		t.Fatalf("link %d %s", link.Code, link.Body.String())
	}
	wb := repository.NewWBRepo(db)
	tk, _ := wb.GetTask("T-NP")
	if tk == nil || tk.ProjectID != "WP-P1" {
		t.Fatalf("연결 후 %+v", tk)
	}
	procs, _ := wb.ListWorkTaskProcesses("O03", "T-NP")
	found := false
	for _, p := range procs {
		if strings.Contains(p.WorkContent, "사업 연결") {
			found = true
		}
	}
	if !found {
		t.Fatalf("조치 기록 없음 %+v", procs)
	}

	other := httptest.NewRecorder()
	otreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(url.Values{
		"task_id": {"T-O01"}, "project_id": {"WP-P1"}, "org": {"O03"},
	}.Encode()))
	otreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	otreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(other, otreq)
	if other.Code != http.StatusNotFound {
		t.Fatalf("타 조직 업무 want 404 got %d", other.Code)
	}

	ids := make([]string, 201)
	for i := range ids {
		ids[i] = "T"
	}
	over := httptest.NewRecorder()
	ov := url.Values{"project_id": {"WP-P1"}, "org": {"O03"}}
	ov["task_id"] = ids
	ovreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(ov.Encode()))
	ovreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ovreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(over, ovreq)
	if over.Code != http.StatusBadRequest {
		t.Fatalf("상한 %d", over.Code)
	}

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-08-10&org=O03", nil)
	greq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(get, greq)
	if get.Code != 200 {
		t.Fatalf("화면 %d", get.Code)
	}
	body := get.Body.String()
	if strings.Contains(body, "사업 연결 안 됨") {
		t.Fatal("옛 문구")
	}
	if !strings.Contains(body, "보고서에 포함되지 않은 활동") && !strings.Contains(body, "weekly_unassigned") {
		if !strings.Contains(body, "사업이 연결되지 않은 업무") && !strings.Contains(body, "보고서 행이 없는 사업") {
			t.Fatalf("구역 문구 없음: %s", body[0:min(400, len(body))])
		}
	}

	rep := httptest.NewRecorder()
	rpreq := httptest.NewRequest(http.MethodGet, "/stats/reports?company_date=2026-08-17", nil)
	rpreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(rep, rpreq)
	if rep.Code != 200 {
		t.Fatalf("예전 화면 %d", rep.Code)
	}
	if strings.Contains(rep.Body.String(), "사업 연결 안 됨") {
		t.Fatal("예전 화면에 옛 문구")
	}

	logs := readAccessLogs(t, accessPath)
	var saw bool
	for _, l := range logs {
		if l.Target == "weekly_report_rows" && l.Action == auditlog.ActionCreate {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("감사 로그 없음 %+v", logs)
	}
}

func TestWeeklyUnassignedPartialSharedAndUILint(t *testing.T) {
	root := findTemplateRoot(t)
	for _, name := range []string{"stats/reports.html", "stats/company_weekly_result.html", "stats/company_weekly.html"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `{{template "weekly_unassigned"`) {
			t.Fatalf("%s 부분 템플릿 없음", name)
		}
	}
	got := countUILint(string(mustRead(t, filepath.Join(root, "stats/_weekly_unassigned.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
