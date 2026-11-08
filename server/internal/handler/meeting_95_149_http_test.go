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

func Test95_149_T2_HintAndNeedSelectHTML(t *testing.T) {
	fromto := "from=2026-10-21&to=2026-10-21&view=project"
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m149h.db"))
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
	oa := &model.User{Username: "oa149", PasswordHash: "x", FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES ('c149a','해미초','해미초',1,'O03'),('c149b','미연결초','미연결초',1,'O03'),('c149c','중복초','중복초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET ordering_party='세종시교육청', plan_year=2026, org_id='O03' WHERE project_id='WPSEED02'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,plan_year,ordering_party)
		VALUES ('WP-011','세종시 도서관 ICT 통합정보시스템 유지관리(2026년)','active','O03',2025,'별도발주')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active) VALUES
		('S149a','WPSEED02','c149a','해미초','in_progress','O03',1),
		('S149c1','WPSEED02','c149c','중복A','in_progress','O03',1),
		('S149c2','WP-011','c149c','중복B','in_progress','O03',1)`); err != nil {
		t.Fatal(err)
	}
	day := "2026-10-21"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, symptom, updated_at, org_id, project_id
	) VALUES
		('as-149a','R149-A','c149a','2026-10-05',?,1,'in_progress','SI관리자','사이트보완',?,'O03',''),
		('as-149b','R149-B','c149b','2026-10-05',?,1,'in_progress','SI관리자','미연결',?,'O03',''),
		('as-149c','R149-C','c149c','2026-10-05',?,1,'in_progress','SI관리자','선택필요',?,'O03',''),
		('as-149s','R149-S','c149a','2026-10-05',?,1,'in_progress','SI관리자','시드저장',?,'O03','WPSEED02'),
		('as-149p','R149-P','c149a','2026-10-05',?,1,'in_progress','SI관리자','등록저장',?,'O03','WP-011')`,
		day, day+" 09:00:00", day, day+" 09:00:00", day, day+" 09:00:00",
		day, day+" 09:00:00", day, day+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, status, assignee, org_id, project_id, customer_id, customer_name)
		VALUES ('wt-149g','admin','네트워크점검',?,?, 'waiting','SI관리자','O03','WP-011','c149a','해미초')`, day, day); err != nil {
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
	if !strings.Contains(body, `data-project-hint="1"`) {
		t.Fatal("안내 상자")
	}
	if !strings.Contains(body, `data-unlinked-n="2"`) || !strings.Contains(body, `data-no-site-n="1"`) ||
		!strings.Contains(body, `data-need-select="1"`) || !strings.Contains(body, `data-site-based="1"`) {
		t.Fatalf("안내 숫자 %s", body[strings.Index(body, "data-project-hint"):minInt(strings.Index(body, "data-project-hint")+400, len(body))])
	}
	if strings.Count(body, "사업 미연결 2건(사이트 미등록 1 · 사업 선택 필요 1) · 사이트 등록 기준 1건 묶음") != 1 {
		t.Fatal("안내 문구")
	}
	if !strings.Contains(body, `data-need-select-chip="1"`) || !strings.Contains(body, "사업 선택 필요 1건") {
		t.Fatal("미연결 칩")
	}
	if !strings.Contains(body, `data-unlinked-kinds="1"`) || !strings.Contains(body, "정기점검 0 · AS 2 · 일반 0") {
		t.Fatal("미연결 내역")
	}
	if strings.Count(body, `data-project-id="WPSEED02"`) != 1 || strings.Count(body, `data-project-id="WP-011"`) != 1 {
		t.Fatal("동명 사업 블록")
	}
	if !strings.Contains(body, `data-project-no="WPSEED02"`) || !strings.Contains(body, `data-project-no="WP-011"`) {
		t.Fatal("사업 번호")
	}
	if !strings.Contains(body, `data-project-head="1"`) || !strings.Contains(body, `data-site-chip="1"`) {
		t.Fatal("회색 머리·사이트 칩")
	}
	if !strings.Contains(body, "사이트 등록 2건") || !strings.Contains(body, "2026년") || !strings.Contains(body, "세종시교육청") {
		t.Fatal("구분 정보")
	}
	if !strings.Contains(body, "해미초 · 네트워크점검") {
		t.Fatal("일반업무 사이트·업무명")
	}
	if !strings.Contains(body, `data-empty="1"`) {
		t.Fatal("0건 흐림 표시")
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "meeting", "board.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func Test95_149_T5_BrowserProjectView(t *testing.T) {
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
	cmd := exec.Command("node", "95_149_meeting_board_browser.mjs")
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
