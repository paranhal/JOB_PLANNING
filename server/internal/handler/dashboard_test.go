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

func TestDashboardRenders(t *testing.T) {
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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
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
	if !strings.Contains(body, "AS · 정기점검 기준") {
		t.Fatal("실행률 카드에 대상 안내가 없다")
	}
}
