package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestNavConstantsMatchSidebar(t *testing.T) {
	body, err := os.ReadFile("web/templates/layout/base.html")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`eq \$a "([a-z_]+)"`)
	found := map[string]struct{}{}
	for _, m := range re.FindAllSubmatch(body, -1) {
		found[string(m[1])] = struct{}{}
	}
	if len(found) == 0 {
		t.Fatal("사이드바 Active 키가 없다")
	}
	have := map[string]struct{}{}
	for _, k := range navSidebarKeys() {
		have[k] = struct{}{}
		if _, ok := found[k]; !ok {
			t.Errorf("상수 %q 가 base.html 사이드바에 없다", k)
		}
	}
	for k := range found {
		if _, ok := have[k]; !ok {
			t.Errorf("사이드바 키 %q 에 대응하는 상수가 없다", k)
		}
	}
}

func TestWorkListSidebarHighlightsWork(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-nav.db"))
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
	g.GET("/work", h.Work.List)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `href="/" class="bg-blue-600`) {
		t.Fatal("대시보드가 강조되고 있다")
	}
	if !strings.Contains(body, `href="/work" class="bg-blue-600`) {
		t.Fatal("오늘 내 업무가 강조되지 않는다")
	}
}

func TestIsSalesNav(t *testing.T) {
	cases := []struct {
		path, q string
		want    bool
	}{
		{"/sales", "", true},
		{"/sales/dashboard", "", true},
		{"/sales/abc", "", true},
		{"/quotes", "", true},
		{"/quotes/q1", "", true},
		{"/orders", "", true},
		{"/contracts", "", true},
		{"/customers", "view=sales", true},
		{"/customers", "view=sales&page=2", true},
		{"/customers", "", false},
		{"/customers", "review=1", false},
		{"/work", "", false},
		{"/items", "", false},
		{"/projects", "", false},
	}
	for _, tc := range cases {
		if got := IsSalesNav(tc.path, tc.q); got != tc.want {
			t.Errorf("IsSalesNav(%q,%q)=%v want %v", tc.path, tc.q, got, tc.want)
		}
	}
}

func TestSalesSidebarBackToWork(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sales-nav.db"))
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
	g.GET("/work", h.Work.List)
	g.GET("/sales", h.Sales.List)
	g.GET("/sales/dashboard", h.Sales.Dashboard)
	g.GET("/customers", h.Customer.List)
	g.GET("/contracts", h.Project.Contracts)

	ck := jwtCookie(t)
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	salesBody := get("/sales")
	if !strings.Contains(salesBody, "← 업무로") {
		t.Fatal("/sales 에 ← 업무로 가 없다")
	}
	if !strings.Contains(salesBody, `href="/work"`) {
		t.Fatal("/sales 에 /work 링크가 없다")
	}
	if strings.Contains(salesBody, "오늘 내 업무") {
		t.Fatal("/sales 사이드바에 오늘 내 업무가 남아 있다")
	}
	if !strings.Contains(salesBody, "영업 대시보드") || !strings.Contains(salesBody, "견적 관리") ||
		!strings.Contains(salesBody, "수주 관리") || !strings.Contains(salesBody, "계약 관리") ||
		!strings.Contains(salesBody, "고객·파트너") {
		t.Fatal("/sales 영업 메뉴가 빠졌다")
	}
	if strings.Contains(salesBody, "사업 발굴") {
		t.Fatal("메뉴 이름이 사업 발굴이다")
	}

	dash := get("/sales/dashboard")
	if !strings.Contains(dash, "← 업무로") || !strings.Contains(dash, "영업 대시보드") {
		t.Fatal("/sales/dashboard 영업 사이드바가 아니다")
	}

	cust := get("/customers?view=sales")
	if !strings.Contains(cust, "← 업무로") {
		t.Fatal("고객 영업 보기에 ← 업무로 가 없다")
	}
	if !strings.Contains(cust, "고객·파트너") {
		t.Fatal("고객 영업 보기 제목이 아니다")
	}
	if !strings.Contains(cust, "진행 중 사업") || !strings.Contains(cust, "누적 수주액") ||
		!strings.Contains(cust, "마지막 활동") {
		t.Fatal("고객 영업 보기 열이 없다")
	}
	if strings.Contains(cust, "오늘 내 업무") {
		t.Fatal("고객 영업 보기에 업무 사이드바가 남아 있다")
	}

	work := get("/work")
	if !strings.Contains(work, "오늘 내 업무") {
		t.Fatal("/work 에 오늘 내 업무가 없다")
	}
	if strings.Contains(work, "← 업무로") {
		t.Fatal("/work 에 ← 업무로 가 있다")
	}

	plainCust := get("/customers")
	if strings.Contains(plainCust, "← 업무로") {
		t.Fatal("기준정보 고객현황이 영업 사이드바다")
	}
	if !strings.Contains(plainCust, "고객현황") {
		t.Fatal("기준정보 고객현황 제목이 없다")
	}
}
