package handler

import (
	"database/sql"
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

func seedWeekly146Page(t *testing.T, db *sql.DB, stats *repository.StatsRepo) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES
		('C146','해미초','해미초',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES
		('WP146','본사업','active','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id)
		VALUES ('MP146',2026,'t','approved','O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id,plan_id,visit_date,customer_id,product_type,completed,completed_date,assignee,project_id)
		VALUES ('V146D','MP146','2026-08-12','C146','KLAS',1,'2026-08-12','담당','WP146')`); err != nil {
		t.Fatal(err)
	}
	if _, err := stats.InsertWeeklyReportRow(repository.WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "본사업", Kind: "project", ProjectID: "WP146", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stats.InsertWeeklyReportRow(repository.WeeklyReportRowInput{
		OrgID: "O03", DisplayName: "사업 미연결 업무", Kind: "activity", ActivityKind: "unlinked", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func Test95_146_T1T3T5_WeeklyStatBoxesHTML(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw146h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	stats := repository.NewStatsRepo(db)
	seedWeekly146Page(t, db, stats)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")

	get := func(q string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-08-10&org=O03"+q, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	body := get("")
	if !strings.Contains(body, "정기점검 방문") || !strings.Contains(body, "AS 방문/완료") ||
		!strings.Contains(body, "작업 방문") || !strings.Contains(body, "행정/지원 업무") || !strings.Contains(body, "영업") {
		t.Fatal("박스 5종 라벨")
	}
	if !strings.Contains(body, "완료 1건 / 총 1건") || !strings.Contains(body, "진행률 100%") {
		t.Fatal("T1 완료/총/진행률")
	}
	if !strings.Contains(body, "진행률 —") {
		t.Fatal("T1 총 0 진행률 —")
	}
	if !strings.Contains(body, `data-weekly-stat-box="mnt"`) || !strings.Contains(body, `aria-expanded="false"`) {
		t.Fatal("T3 초기 접힘")
	}
	if !strings.Contains(body, `x-data="{ open: '' }"`) {
		t.Fatal("T3 카드당 패널 하나")
	}
	if strings.Contains(body, `name="stat_`) {
		t.Fatal("T5 폼 필드")
	}
	if !strings.Contains(body, `type="button"`) || !strings.Contains(body, "접기") {
		t.Fatal("박스 button·접기")
	}
	if !strings.Contains(body, "board-done-badge") {
		t.Fatal("95-142 완료 배지")
	}
	if !strings.Contains(body, "해당 업무가 없습니다") {
		t.Fatal("0건 안내")
	}
	edit := get("&edit=1")
	if !strings.Contains(edit, `data-weekly-stats="1"`) || !strings.Contains(edit, `name="done_`) {
		t.Fatal("T5 편집 모드에도 박스·기존 폼")
	}
	if strings.Count(edit, `data-weekly-stat-box="mnt"`) < 1 {
		t.Fatal("T5 편집에서 박스 사라짐")
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "stats", "_weekly_card.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func Test95_146_T6_BrowserStatBoxToggle(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw146br.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	stats := repository.NewStatsRepo(db)
	seedWeekly146Page(t, db, stats)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_146_weekly_stat_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieOrg(t, model.RoleOrgAdmin, "O03").Value,
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
