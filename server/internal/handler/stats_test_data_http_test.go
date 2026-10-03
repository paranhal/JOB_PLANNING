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

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestAdminTestStatsRedirectsToSwitcher(t *testing.T) {
	e := newTestStatsApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/test-stats", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/stats?") || !strings.Contains(loc, "test=1") {
		t.Fatalf("redirect loc=%s", loc)
	}
}

func TestStatsTestSwitchUsesSameAggregation(t *testing.T) {
	e := newTestStatsApp(t)

	plain := doLookbackGet(t, e, "/stats")
	if !strings.Contains(plain, "테스터만 보기") {
		t.Fatal("관리자 화면에 스위치가 없다")
	}
	if strings.Contains(plain, "통계 · 테스터만") || strings.Contains(plain, "· 테스터만 보기") {
		t.Fatal("기본 통계가 테스터 모드다")
	}
	if strings.Contains(plain, "R-TST1") {
		t.Fatal("기본 통계에 테스터 AS가 보인다")
	}
	if !strings.Contains(plain, "R-APP1") {
		t.Fatal("기본 통계에 실 AS가 없다")
	}

	only := doLookbackGet(t, e, "/stats?test=1")
	if !strings.Contains(only, "통계 · 테스터만") || !strings.Contains(only, "· 테스터만 보기") {
		t.Fatal("스위치를 켜도 테스터 모드가 아니다")
	}
	if !strings.Contains(only, "R-TST1") {
		t.Fatal("테스터만 보기에 테스터 AS가 없다")
	}
	if strings.Contains(only, "R-APP1") {
		t.Fatal("테스터만 보기에 실 AS가 섞였다")
	}

	sales := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodGet, "/stats?test=1", nil)
	sreq.AddCookie(jwtCookiePerms(t, model.RoleSales))
	e.ServeHTTP(sales, sreq)
	if sales.Code != http.StatusOK {
		t.Fatalf("영업 code=%d", sales.Code)
	}
	body := sales.Body.String()
	if strings.Contains(body, "name=\"test\"") {
		t.Fatal("영업 화면에 테스터 스위치가 있다")
	}
	if strings.Contains(body, "R-TST1") {
		t.Fatal("영업이 test=1 로 테스터 건을 봤다")
	}
}

func newTestStatsApp(t *testing.T) *echo.Echo {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "test-stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	stamp := now.Format("2006-01-02 15:04:05")
	day := now.Format("2006-01-02")
	_, err = db.Exec(fmt.Sprintf(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin, org_id, is_test
		) VALUES
		('app1','R-APP1','c1','%s','%s','%s','%s','completed','양기헌','app','O01',0),
		('tst1','R-TST1','c1','%s','%s','%s','%s','completed','테스터','app','O01',1)`,
		stamp, day, stamp, stamp, stamp, day, stamp, stamp))
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats", h.Stats.Overview)
	g.GET("/admin/test-stats", h.Stats.TestStatsRedirect, h.Auth.RequireTestStatsMW)
	return e
}
