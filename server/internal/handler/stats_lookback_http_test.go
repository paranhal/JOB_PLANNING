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

	d := doLookbackGet(t, e, "/")
	if !strings.Contains(d, "[참고]") || !strings.Contains(d, "착수일시가 아니라 방문일 기준") {
		t.Fatal("대시보드 접수→방문 [참고] 없음")
	}
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
	if !strings.Contains(ov, "[AS만]") || !strings.Contains(ov, "[참고]") {
		t.Fatal("지표 범위·참고 표시 없음")
	}
	if !strings.Contains(ov, "[AS · 정기점검 · 행정지원]") {
		t.Fatal("접수·처리 건수 범위 표시 없음")
	}
	if !strings.Contains(ov, "표본 ") {
		t.Fatal("표본 수가 없다")
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

func TestStatsLeadTimeWarnBannerVisible(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leadwarn.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin, process_type, visit_date
		) VALUES
		('v1','R-V1','c1','2026-08-10','2026-08-12',
		 '2026-08-12 10:00:00','2026-08-12 18:00:00','completed','관리자','app','visit','2026-08-20'),
		('v2','R-V2','c1','2026-08-10','2026-08-12',
		 '2026-08-12 10:00:00','2026-08-12 18:00:00','completed','관리자','app','visit','2026-08-20')`)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats", h.Stats.Overview)
	body := doLookbackGet(t, e, "/stats")
	if !strings.Contains(body, "id=\"stats-lead-time-warn\"") || !strings.Contains(body, "방문") || !strings.Contains(body, "완료") {
		t.Fatal("방문>완료 경고 배너가 없다")
	}
}

func TestDashboardKPIScopeBadges(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "dash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','대시보드도서관','대시보드도서관',1)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/", h.Dashboard)
	g.GET("/stats", h.Stats.Overview)

	body := doLookbackGet(t, e, "/")
	if strings.Contains(body, "Internal Server Error") {
		t.Fatal("대시보드가 JSON 500이다")
	}
	if !strings.Contains(body, "대시보드") {
		t.Fatal("대시보드 제목이 없다")
	}
	if !strings.Contains(body, "다가오는 실행") || !strings.Contains(body, "마감 경과") {
		t.Fatal("반복 알림 카드가 없다")
	}
	if !strings.Contains(body, "2026-08-03 이후 데이터 기준") {
		t.Fatal("대시보드에 지표 기준일이 없다")
	}
	if !strings.Contains(body, "최근 1개월") {
		t.Fatal("대시보드 KPI 기본이 최근 1개월이 아니다")
	}
	if strings.Contains(body, "계획대비 실행률") || strings.Contains(body, "계획대비 실행율") {
		t.Fatal("대시보드에 실행률 KPI가 남아 있다")
	}
	if strings.Contains(body, ">일별</button>") || strings.Contains(body, "그래프 단위") {
		t.Fatal("대시보드에 일별/주별/월별 버튼이 남아 있다")
	}
	if !strings.Contains(body, "일별로 묶어 표시") {
		t.Fatal("1개월 기본에서 일별 묶음 안내가 없다")
	}
	if !strings.Contains(body, "접수→방문") || !strings.Contains(body, "접수→조치완료") {
		t.Fatal("대시보드 KPI 2칸이 없다")
	}
	m6 := doLookbackGet(t, e, "/?range=6m")
	if !strings.Contains(m6, "주별로 묶어 표시") {
		t.Fatal("6개월(기준일 클립)에서 주별 묶음 안내가 없다")
	}
	if strings.Contains(m6, ">일별</button>") {
		t.Fatal("6개월 대시보드에 단위 버튼이 있다")
	}
	st := doLookbackGet(t, e, "/stats")
	if !strings.Contains(st, "그래프 단위") || !strings.Contains(st, ">일별</button>") {
		t.Fatal("통계 화면의 단위 선택이 없다")
	}
}

func TestMissingCompleteDateBanner(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "miss-complete.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			status, assigned_to, data_origin, updated_at
		) VALUES ('miss1','R-MISS','c1',?,'2026-08-10',
			'completed','관리자','app',?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats", h.Stats.Overview)
	g.GET("/", h.Dashboard)
	g.GET("/work-status", h.WorkStatus.Calendar)

	stats := doLookbackGet(t, e, "/stats")
	if !strings.Contains(stats, "완료일 미기록 1건 제외") {
		t.Fatal("통계에 완료일 미기록 배너가 없다")
	}
	dash := doLookbackGet(t, e, "/")
	if !strings.Contains(dash, "완료일 미기록 1건 제외") {
		t.Fatal("대시보드에 완료일 미기록 배너가 없다")
	}
	ws := doLookbackGet(t, e, "/work-status")
	if !strings.Contains(ws, "완료일 미기록") || !strings.Contains(ws, "1건 제외") {
		t.Fatal("업무처리현황에 완료일 미기록 배너가 없다")
	}
	if !strings.Contains(ws, "목록 · 전체 기간 · 이관 데이터 포함 (통계 집계와 다름)") {
		t.Fatal("업무처리현황이 목록(이관 포함)임을 안 밝힌다")
	}
}
