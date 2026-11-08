package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_147_T3_ProjectViewBoxesHTML(t *testing.T) {
	e, _, u := newMeeting970Server(t)
	fromto := "from=2026-10-21&to=2026-10-21"
	proj := weeklyGet(t, e, "/meeting/board?"+fromto+"&view=project", jwtCookieUserClaims(t, u.oa01))
	if proj.Code != http.StatusOK {
		t.Fatalf("project %d", proj.Code)
	}
	pb := proj.Body.String()
	if !strings.Contains(pb, `data-board-table="project"`) {
		t.Fatal("data-board-table")
	}
	for _, lab := range []string{"정기점검 방문", "AS 방문/완료", "작업 방문", "행정/지원 업무", "영업"} {
		if !strings.Contains(pb, lab) {
			t.Fatalf("박스 라벨 %s", lab)
		}
	}
	if strings.Count(pb, `data-weekly-stat-box="mnt"`) < 1 || !strings.Contains(pb, `type="button"`) {
		t.Fatal("박스 button")
	}
	if !strings.Contains(pb, `aria-expanded="false"`) || !strings.Contains(pb, `hidden x-cloak`) {
		t.Fatal("초기 접힘")
	}
	if strings.Count(pb, `data-item-key="project_work:wt-p"`) != 1 {
		t.Fatal("data-item-key")
	}
	if !strings.Contains(pb, "도서관공사") || !strings.Contains(pb, `data-leave-badge="도서관기술"`) {
		t.Fatal("사업명·휴가 배지")
	}
	if !strings.Contains(pb, "사이트 등록 기준") {
		t.Fatal("95-145 사이트 기준")
	}
	people := weeklyGet(t, e, "/meeting/board?"+fromto+"&view=people", jwtCookieUserClaims(t, u.oa01))
	if people.Code != http.StatusOK {
		t.Fatalf("people %d", people.Code)
	}
	pe := people.Body.String()
	if strings.Contains(pe, `data-board-table="project"`) || strings.Contains(pe, `data-weekly-stat-box="mnt"`) {
		t.Fatal("담당자별 보기에 사업별 박스")
	}
	if !strings.Contains(pe, `data-board-table="people"`) {
		t.Fatal("담당자별 표")
	}
	weekly := weeklyGet(t, e, "/meeting/board?"+fromto+"&view=weekly", jwtCookieUserClaims(t, u.oa01))
	if weekly.Code != http.StatusOK || !strings.Contains(weekly.Body.String(), `data-board-table="done"`) {
		t.Fatal("주간 자료표")
	}
	pr := weeklyGet(t, e, "/meeting/board?"+fromto+"&view=project&print=1", jwtCookieUserClaims(t, u.oa01))
	if pr.Code != http.StatusOK {
		t.Fatalf("print %d", pr.Code)
	}
	if !strings.Contains(pr.Body.String(), `data-weekly-stat-box="mnt"`) {
		t.Fatal("인쇄에 박스 없음")
	}
	css := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "css", "input.css")))
	if !strings.Contains(css, "weekly-stat-grid-fill-last") || !strings.Contains(css, ".weekly-stat-panel") {
		t.Fatal("인쇄·390 클래스")
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "meeting", "board.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func Test95_147_T2_UnlinkedAndSiteFillHTML(t *testing.T) {
	fromto := "from=2026-10-21&to=2026-10-21&view=project"
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m147h.db"))
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
	oa := &model.User{Username: "oa147", PasswordHash: "x", FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES ('c147a','해미초','해미초',1,'O03'),('c147b','미연결초','미연결초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP147','본사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('S147','WP147','c147a','해미초','in_progress','O03',1)`); err != nil {
		t.Fatal(err)
	}
	day := "2026-10-21"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, symptom, updated_at, org_id
	) VALUES
		('as-147a','R147-A','c147a','2026-10-05',?,1,'in_progress','SI관리자','사이트보완',?,'O03'),
		('as-147b','R147-B','c147b','2026-10-05',?,1,'in_progress','SI관리자','미연결',?,'O03')`,
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
	if !strings.Contains(body, `data-unlinked="1"`) || !strings.Contains(body, "사업 미연결") {
		t.Fatal("미연결 블록")
	}
	if strings.Contains(body, "사업 선택 필요(1)") {
		t.Fatal("사이트 0건을 선택 필요로 셈")
	}
	if !strings.Contains(body, "본사업") || !strings.Contains(body, `data-project-hint="1"`) {
		t.Fatal("95-145 보완")
	}
	if strings.Count(body, `data-weekly-stat-box="as"`) < 2 {
		t.Fatal("미연결도 박스")
	}
}

func Test95_147_T5_BrowserStatBoxToggle(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, _, u := newMeeting970Server(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_147_meeting_board_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieUserClaims(t, u.oa01).Value,
		"SCREENSHOT_DIR="+shot,
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
