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

func TestReportsPageStaysLegacyNoRedirect(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw31.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/reports", h.Stats.Reports)
	g.GET("/stats/company-weekly", h.Stats.ShowWeeklyReport)
	g.GET("/stats/company-weekly/rows", h.Stats.ListCompanyWeeklyRows)

	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rep := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/reports", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rep, req)
	if rep.Code != http.StatusOK {
		t.Fatalf("예전 화면 리다이렉트되면 안 됨 %d loc=%s", rep.Code, rep.Header().Get("Location"))
	}
	body := rep.Body.String()
	if !strings.Contains(body, "예전 보고서 화면입니다") {
		t.Fatal("안내 배너 없음")
	}
	if !strings.Contains(body, `href="/stats/company-weekly"`) {
		t.Fatal("새 화면 링크 없음")
	}
	if !strings.Contains(body, `id="outputs"`) {
		t.Fatal("#outputs 없음")
	}
	if strings.Contains(body, "새 주간 업무 보고서 화면으로 이동") {
		t.Fatal("옛 이동 버튼이 남아 있다")
	}

	ui := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O03", nil)
	ureq.AddCookie(ck)
	e.ServeHTTP(ui, ureq)
	if ui.Code != http.StatusOK {
		t.Fatalf("새 화면 %d", ui.Code)
	}
	ub := ui.Body.String()
	if !strings.Contains(ub, "다른 출력물") || !strings.Contains(ub, `href="/stats/reports#outputs"`) {
		t.Fatal("다른 출력물 링크 없음")
	}
	if !strings.Contains(ub, "예전 화면으로") {
		t.Fatal("예전 화면으로 링크 없음")
	}
	if !strings.Contains(ub, "jp_weekly_nav_notice") {
		t.Fatal("최초 안내 없음")
	}

	if ParentPage("/stats/company-weekly/rows") != "/stats/company-weekly" {
		t.Fatal("행 관리 이전이 새 주소가 아니다")
	}
	root := findTemplateRoot(t)
	rows := string(mustRead(t, filepath.Join(root, "stats/company_weekly_rows.html")))
	if strings.Contains(rows, `href="/stats/reports"`) {
		t.Fatal("행 관리가 예전 보고서를 가리킨다")
	}
}

func TestWeeklyUnassignedPartialShared95_31(t *testing.T) {
	root := findTemplateRoot(t)
	for _, name := range []string{"stats/reports.html", "stats/company_weekly_result.html", "stats/company_weekly.html"} {
		b := string(mustRead(t, filepath.Join(root, name)))
		if !strings.Contains(b, `{{template "weekly_unassigned"`) {
			t.Fatalf("%s 부분 템플릿 없음", name)
		}
	}
	got := countUILint(string(mustRead(t, filepath.Join(root, "stats/_weekly_unassigned.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func TestWeeklyNewFlowReportsRefs(t *testing.T) {
	root := findTemplateRoot(t)
	result := string(mustRead(t, filepath.Join(root, "stats/company_weekly_result.html")))
	if !strings.Contains(result, `href="/stats/reports"`) {
		t.Fatal("예전 생성 결과 화면은 /stats/reports 자기 참조를 유지한다")
	}
	ov := string(mustRead(t, filepath.Join(root, "stats/overview.html")))
	if !strings.Contains(ov, `href="/stats/reports`) {
		t.Fatal("통계 개요의 출력물 이동은 예전 화면")
	}
	src := string(mustRead(t, filepath.Join(filepath.Dir(filepath.Dir(findTemplateRoot(t))), "internal", "handler", "meeting_weekly.go")))
	if !strings.Contains(src, `reports = "/stats/company-weekly"`) {
		t.Fatal("주간 회의 팀 자료 링크가 새 보고서가 아니다")
	}
}
