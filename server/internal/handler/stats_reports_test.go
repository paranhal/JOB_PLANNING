package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestStatsReportsPageCardsAndExistingEndpoints(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "reports.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp-2026', 2026, '2026 계획', 'approved')`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/reports", h.Stats.Reports)
	g.GET("/stats/weekly-report.xlsx", h.Stats.ExportWeeklyReport)
	g.GET("/stats/daily-assignee.xlsx", h.Stats.ExportDailyAssigneeReport)
	g.GET("/stats", h.Stats.Overview)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/stats/reports", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		t.Fatalf("status=%d body=%s", rec.Code, snippet)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"주간업무보고서",
		"/stats/weekly-report.xlsx",
		"담당자별 일일업무",
		"/stats/daily-assignee.xlsx",
		"period=week",
		"월간업무보고",
		"준비 중",
		"정기점검 월별 캘린더",
		"/maintenance/mp-2026/export.xlsx",
		"상세 통계",
		`action="/stats/detail"`,
		`href="/stats/reports"`,
		"전사 주간업무보고 시트 생성",
		"시트 이름",
		`name="sheet_name"`,
		`name="company_date"`,
		`enctype="multipart/form-data"`,
		`action="/stats/company-weekly"`,
		"지원 필요사항",
		"주요 의사결정",
		"회사 공통 파일입니다. 생성 전 원본을 따로 보관하세요.",
		`name="prev_`,
		`name="help_`,
		`name="decision_`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !strings.Contains(body, `name="company_date"`) {
		t.Fatal("보고 기준일 없음")
	}

	cw := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodGet, "http://localhost/stats/reports?company_date=2026-08-17", nil)
	creq.AddCookie(jwtCookie(t))
	e.ServeHTTP(cw, creq)
	if cw.Code != http.StatusOK {
		t.Fatalf("전사 미리보기 status=%d", cw.Code)
	}
	cb := cw.Body.String()
	if !strings.Contains(cb, "26.8월2주(업무)") {
		t.Fatal("시트 이름 기본값이 없다")
	}
	if !strings.Contains(cb, "8.10(월)~8.16(일)") {
		t.Fatal("전주 기간 라벨이 없다")
	}
	if strings.Count(body, `x-data="{`) < 1 {
		t.Fatal("공유 기간 x-data 없음")
	}
	if strings.Contains(body, "monthly-report.xlsx") {
		t.Fatal("월간 보고 엔드포인트를 새로 만들면 안 됨")
	}

	xlsx := httptest.NewRecorder()
	xreq := httptest.NewRequest(http.MethodGet, "http://localhost/stats/weekly-report.xlsx?date=2026-08-10", nil)
	xreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(xlsx, xreq)
	if xlsx.Code != http.StatusOK {
		t.Fatalf("주간보고서 기존 엔드포인트 status=%d", xlsx.Code)
	}

	daily := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodGet, "http://localhost/stats/daily-assignee.xlsx?period=week&date=2026-08-10", nil)
	dreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(daily, dreq)
	if daily.Code != http.StatusOK {
		t.Fatalf("담당자별 일일업무 status=%d", daily.Code)
	}
	cd := daily.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "담당자별일일업무.xlsx") && !strings.Contains(cd, "daily_assignee.xlsx") {
		t.Fatalf("파일명: %s", cd)
	}

	ov := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "http://localhost/stats", nil)
	oreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(ov, oreq)
	if ov.Code != http.StatusOK {
		t.Fatalf("통계 status=%d", ov.Code)
	}
	ob := ov.Body.String()
	if strings.Contains(ob, "주간업무보고서") {
		t.Fatal("통계 화면에 주간업무보고서 버튼이 남아 있음")
	}
	if !strings.Contains(ob, "보고서로 이동") || !strings.Contains(ob, `href="/stats/reports`) {
		t.Fatal("보고서로 이동 링크 없음")
	}
	if !strings.Contains(ob, "최근 1개월") {
		t.Fatal("통계 기본 기간이 최근 1개월이 아니다")
	}
	if !strings.Contains(ob, "2026-08-03 이후 데이터 기준") {
		t.Fatal("통계 상단에 기준일 안내가 없다")
	}
	if !strings.Contains(ob, "AS · 정기점검 기준") {
		t.Fatal("실행률 카드에 대상 안내가 없다")
	}
	if !strings.Contains(body, "2026-08-03 이후 데이터 기준") {
		t.Fatal("보고서 상단에 기준일 안내가 없다")
	}
}

func TestStatsReportsRequiresLogin(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "reports_auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/reports", h.Stats.Reports)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/stats/reports", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnauthorized {
		t.Fatalf("비로그인 status=%d want redirect/401", rec.Code)
	}
}
