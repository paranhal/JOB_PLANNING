package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newAS151Server(t *testing.T) (*echo.Echo, *sql.DB, *Handler) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "as151.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES ('O01','O01','도서관지원','business',1,10)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	oa := &model.User{Username: "oa151", PasswordHash: HashPassword("x"), FullName: "도서관장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C151','성환도서관','성환도서관',1,?),('C151b','타기관','타기관',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/project", h.AS.UpdateProject)
	g.GET("/workboard/projects/for-site", h.Workboard.ListProjectsForSite)
	g.GET("/workboard/projects/search", h.Workboard.SearchWorkProjects)
	g.POST("/workboard/projects/link-site", h.Workboard.LinkProjectSiteCustomers)
	g.GET("/meeting/board", h.Meeting.Board)
	g.GET("/stats/company-weekly", h.Stats.ShowWeeklyReport)
	g.POST("/stats/company-weekly/tasks/project", h.Stats.LinkWeeklyTasksProject)
	return e, db, h
}

func seed151Projects(t *testing.T, db *sql.DB) {
	t.Helper()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026),
		       ('WP-OLD','닫힌사업','complete',?,0,1,2024)`, org, org, org, org); err != nil {
		t.Fatal(err)
	}
}

func insert151AS(t *testing.T, db *sql.DB, id, cid, status, action, pid string, completed bool) {
	t.Helper()
	day := "2026-10-21"
	comp := sql.NullString{}
	if completed {
		comp = sql.NullString{String: day + " 16:00:00", Valid: true}
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, complete_datetime,
		schedule_confirmed, status, assigned_to, symptom, action_taken, org_id, project_id, updated_at, created_at)
		VALUES (?,?,?,?,?,?,1,?,?,?,?,?,?, datetime('now'), datetime('now'))`,
		id, id, cid, day+" 09:00:00", day, comp, status, "도서관장", "게이트", action, model.OrgIDLibrary, pid); err != nil {
		t.Fatal(err)
	}
}

func as151OA(t *testing.T, db *sql.DB) *http.Cookie {
	t.Helper()
	u, err := repository.NewUserRepo(db).GetByUsername("oa151")
	if err != nil || u == nil {
		t.Fatal(err)
	}
	return jwtCookieVerified(t, u)
}

func doAS151Get(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func doAS151Form(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_151_T1_ActionPickerRows(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','C151','성환도서관','in_progress',?,1),
		       ('PS-B','WP-B','C151','성환도서관','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	insert151AS(t, db, "R151-1", "C151", "in_progress", "", "", false)
	insert151AS(t, db, "R151-2", "C151", "in_progress", "", "WP-A", false)
	ck := as151OA(t, db)
	empty := doAS151Get(t, e, "/as/R151-1/action", ck).Body.String()
	if !strings.Contains(empty, `id="project"`) {
		t.Fatal("사업 칸 없음")
	}
	if !strings.Contains(empty, "사업A") || !strings.Contains(empty, "사업B") {
		t.Fatal("A·B 행 없음")
	}
	if strings.Contains(empty, "사업C") {
		t.Fatal("미등록 C")
	}
	if strings.Contains(empty, "타 부서 사업 보기") {
		t.Fatal("타 부서 보기")
	}
	if strings.Contains(empty, `"Selected":"WP-A"`) {
		t.Fatal("미연결 자동 선택")
	}
	linked := doAS151Get(t, e, "/as/R151-2/action", ck).Body.String()
	if !strings.Contains(linked, "현재 연결") {
		t.Fatal("현재 연결 없음")
	}
	one := doAS151Get(t, e, "/as/R151-1/action", ck).Body.String()
	if strings.Count(extractPickerSiteJSON(one), `"project_id"`) != 2 {
		t.Fatalf("행 수 %s", extractPickerSiteJSON(one))
	}
}

func Test95_151_T2_SearchConfirmSave(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	insert151AS(t, db, "R151-e", "C151", "in_progress", "", "", false)
	ck := as151OA(t, db)
	body := doAS151Get(t, e, "/as/R151-e/action", ck).Body.String()
	if !strings.Contains(body, "이 기관이 등록된 사업이 없습니다. 사업명을 검색해 연결하세요") {
		t.Fatal("안내 없음")
	}
	if !strings.Contains(body, `type="search"`) {
		t.Fatal("검색 없음")
	}
	noc := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{
		"project_id": {"WP-A"}, "customer_id": {"C151"},
	}, ck)
	if noc.Code != http.StatusBadRequest {
		t.Fatalf("확인 없이 %d", noc.Code)
	}
	var n0 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE customer_id='C151'`).Scan(&n0)
	if n0 != 0 {
		t.Fatal("확인 전 등록")
	}
	ok := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"C151"},
	}, ck)
	if ok.Code != http.StatusOK {
		t.Fatalf("link %d %s", ok.Code, ok.Body.String())
	}
	again := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"C151"},
	}, ck)
	if again.Code != http.StatusOK {
		t.Fatal(again.Body.String())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE customer_id='C151' AND project_id='WP-A'`).Scan(&n)
	if n != 1 {
		t.Fatalf("sites=%d", n)
	}
	save := doAS151Form(t, e, "/as/R151-e/project", url.Values{"project_id": {"WP-A"}}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String())
	}
	var pid string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='R151-e'`).Scan(&pid)
	if pid != "WP-A" {
		t.Fatalf("project_id=%s", pid)
	}
	var logs int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id='R151-e' AND action='update'`).Scan(&logs)
	if logs < 1 {
		t.Fatal("이력 없음")
	}
	reload := doAS151Get(t, e, "/as/R151-e/action", ck).Body.String()
	if !strings.Contains(reload, "사업A") {
		t.Fatal("새로고침 사업명 없음")
	}
}

func Test95_151_T3_CompletedUnchanged(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','C151','성환도서관','in_progress',?,1)`, org); err != nil {
		t.Fatal(err)
	}
	insert151AS(t, db, "R2610-062", "C151", "completed", "보드 교체", "", true)
	ck := as151OA(t, db)
	save := doAS151Form(t, e, "/as/R2610-062/project", url.Values{"project_id": {"WP-A"}}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("완료 저장 %d %s", save.Code, save.Body.String())
	}
	var status, action, pid string
	if err := db.QueryRow(`SELECT status, COALESCE(action_taken,''), COALESCE(project_id,'') FROM as_receipts WHERE as_id='R2610-062'`).
		Scan(&status, &action, &pid); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || action != "보드 교체" || pid != "WP-A" {
		t.Fatalf("status=%s action=%q pid=%s", status, action, pid)
	}
	var pn int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_processes WHERE as_id='R2610-062'`).Scan(&pn)
	if pn != 0 {
		t.Fatalf("조치 이력 %d", pn)
	}
}

func Test95_151_T4_RejectOtherOrgAndClosed(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, symptom, org_id, updated_at, created_at)
		VALUES ('R151-x','R151-x','C151','2026-10-21 09:00:00','in_progress','도서관장','x','O03', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	insert151AS(t, db, "R151-o", "C151", "in_progress", "", "", false)
	ck := as151OA(t, db)
	other := doAS151Form(t, e, "/as/R151-x/project", url.Values{"project_id": {"WP-A"}}, ck)
	if other.Code != http.StatusNotFound && other.Code != http.StatusForbidden {
		t.Fatalf("타 조직 %d", other.Code)
	}
	closed := doAS151Form(t, e, "/as/R151-o/project", url.Values{"project_id": {"WP-OLD"}}, ck)
	if closed.Code == http.StatusSeeOther {
		t.Fatal("마감 사업 허용")
	}
}

func Test95_151_T5_MeetingAndWeekly(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	org := model.OrgIDLibrary
	insert151AS(t, db, "R151-m1", "C151", "in_progress", "", "", false)
	insert151AS(t, db, "R151-m2", "C151b", "in_progress", "", "", false)
	ck := as151OA(t, db)
	fromto := "from=2026-10-21&to=2026-10-21&view=project"
	before := doAS151Get(t, e, "/meeting/board?"+fromto, ck).Body.String()
	if !strings.Contains(before, `data-as-project-link="1"`) {
		t.Fatal("회의 사업 연결 링크 없음")
	}
	unBefore := strings.Count(before, "R151-m1") + strings.Count(before, "R151-m2")
	if unBefore < 2 {
		t.Fatal("미연결 AS가 회의에 없음")
	}
	linkSite := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"C151"},
	}, ck)
	if linkSite.Code != http.StatusOK {
		t.Fatalf("사이트 %d %s", linkSite.Code, linkSite.Body.String())
	}
	save := doAS151Form(t, e, "/as/R151-m1/project", url.Values{"project_id": {"WP-A"}}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("연결 %d", save.Code)
	}
	after := doAS151Get(t, e, "/meeting/board?"+fromto, ck).Body.String()
	if !strings.Contains(after, "사업A") {
		t.Fatal("사업 블록 없음")
	}
	if !strings.Contains(after, "R151-m1") || !strings.Contains(after, "R151-m2") {
		t.Fatal("합계 행 유실")
	}
	if strings.Count(after, `data-unlinked="1"`) < 1 {
		t.Fatal("남은 미연결 그룹 없음")
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, complete_datetime,
		schedule_confirmed, status, assigned_to, symptom, org_id, updated_at, created_at)
		VALUES ('R151-w','R151-w','C151b','2026-10-21 09:00:00','2026-10-21','2026-10-21 16:00:00',
			1,'completed','도서관장','점검',?, datetime('now'), datetime('now'))`, org); err != nil {
		t.Fatal(err)
	}
	weekRec := doAS151Get(t, e, "/stats/company-weekly?week=2026-10-19&org="+org, ck)
	week := weekRec.Body.String()
	if !strings.Contains(week, `data-as-project-link="1"`) && !strings.Contains(week, "/as/R151-w/action#project") {
		t.Fatalf("주간 사업 연결 링크 없음 code=%d hasID=%v unassigned=%v canwrite=%v",
			weekRec.Code, strings.Contains(week, "R151-w"),
			strings.Contains(week, "사업이 연결되지 않은 업무"),
			strings.Contains(week, "data-weekly-link"))
	}
	link := doAS151Form(t, e, "/stats/company-weekly/tasks/project", url.Values{
		"item": {"as_receipt:R151-m2"}, "project_id": {"WP-B"}, "org": {org}, "week": {"2026-10-19"},
	}, ck)
	if link.Code != http.StatusSeeOther {
		t.Fatalf("주간 연결 %d %s", link.Code, link.Body.String())
	}
	var pid string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='R151-m2'`).Scan(&pid)
	if pid != "WP-B" {
		t.Fatalf("주간 project_id=%s", pid)
	}
}

func Test95_151_T6_BrowserASAction(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	insert151AS(t, db, "R151-b", "C151", "in_progress", "", "", false)
	ck := as151OA(t, db)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_151_as_action_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"SCREENSHOT_DIR="+shot,
		"ACTION_PATH=/as/R151-b/action",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}
