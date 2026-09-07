package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestStatsLookbackSection15_7(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "lookback.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats", h.Stats.Overview)
	g.GET("/", h.Dashboard)
	g.GET("/stats/reports", h.Stats.Reports)

	now := time.Now()
	today := now.Format("2006-01-02")
	twoDaysAgo := now.AddDate(0, 0, -1).Format("2006-01-02")
	monthAgo := now.AddDate(0, -1, 1).Format("2006-01-02")

	ov := doLookbackGet(t, e, "/stats")
	if !strings.Contains(ov, "최근 1개월") {
		t.Fatal("기본 통계가 최근 1개월이 아니다")
	}
	if !strings.Contains(ov, fmt.Sprintf("최근 1개월 (%s ~ %s)", monthAgo, today)) {
		t.Fatalf("환산 날짜가 없다: want %s ~ %s", monthAgo, today)
	}
	if !strings.Contains(ov, `name="range_n"`) || !strings.Contains(ov, `name="range_unit"`) {
		t.Fatal("기간 숫자·단위 입력이 없다")
	}
	if !strings.Contains(ov, `value="custom"`) || !strings.Contains(ov, `name="from"`) || !strings.Contains(ov, `name="to"`) {
		t.Fatal("사용자 지정 입력이 없다")
	}

	d2 := doLookbackGet(t, e, "/stats?range=2d")
	if !strings.Contains(d2, fmt.Sprintf("최근 2일 (%s ~ %s)", twoDaysAgo, today)) {
		t.Fatalf("2일이 오늘 포함 2일이 아니다: %s ~ %s", twoDaysAgo, today)
	}

	clip := doLookbackGet(t, e, "/stats?range=6m")
	if !strings.Contains(clip, "기준일(2026-08-03) 이후만 집계됩니다") {
		t.Fatal("6개월 선택 시 기준일 자름 안내가 없다")
	}
	if !strings.Contains(clip, "2026-08-03 ~ "+today) {
		t.Fatal("6개월 실제 집계 시작이 기준일이 아니다")
	}

	narrow := doLookbackGet(t, e, "/stats?range=2d&view=month")
	if !strings.Contains(narrow, "기간이 짧아 월별 대신 일별로 그립니다") {
		t.Fatal("짧은 기간+월별 view 축소 안내가 없다")
	}

	filt := doLookbackGet(t, e, "/stats?range=2d&scope=assignee&key=관리자")
	if !strings.Contains(filt, `option value="d"`) || !strings.Contains(filt, ">일</option>") {
		t.Fatal("일 단위 옵션이 없다")
	}
	if !strings.Contains(filt, `value="2"`) {
		t.Fatal("기간 숫자 2가 유지되지 않는다")
	}
	if !strings.Contains(filt, `selected>관리자</option>`) && !strings.Contains(filt, `value="관리자" selected`) {
		t.Fatal("담당자 필터가 유지되지 않는다")
	}

	dash := doLookbackGet(t, e, "/")
	if !strings.Contains(dash, "최근 1개월") {
		t.Fatal("대시보드 KPI 기본 기간이 최근 1개월이 아니다")
	}
	if !strings.Contains(dash, `action="/"`) || !strings.Contains(dash, `name="range_unit"`) {
		t.Fatal("대시보드에 같은 기간 선택기가 없다")
	}

	rep := doLookbackGet(t, e, "/stats/reports")
	if !strings.Contains(rep, "최근 1개월") {
		t.Fatal("보고서 일일업무 기간이 최근 1개월이 아니다")
	}
	if !strings.Contains(rep, "/stats/daily-assignee.xlsx?range=1m") {
		t.Fatal("담당자별 일일업무가 lookback range를 쓰지 않는다")
	}
	if !strings.Contains(rep, "period=week") {
		t.Fatal("주간업무보고서 주차 선택이 없다")
	}
}

func doLookbackGet(t *testing.T, e *echo.Echo, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 800 {
			snippet = snippet[:800]
		}
		t.Fatalf("%s status=%d body=%s", path, rec.Code, snippet)
	}
	return rec.Body.String()
}
