package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_49_MonthTargetJSONAndEmptyPost(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "t49.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES ('c1','가나도서관','가나도서관',1,?, '충청남도','홍성군')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "가나", Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id/month-targets", h.Maintenance.MonthTargetPanelJSON)
	g.POST("/maintenance/:id/month-targets", h.Maintenance.RegisterMonthTargets, h.Auth.RequireMaintenanceEdit)

	js := manageGet(t, e, "/maintenance/"+p.PlanID+"/month-targets?month=2", jwtCookie(t))
	if js.Code != http.StatusOK {
		t.Fatalf("json %d %s", js.Code, js.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	raw := js.Body.String()
	for _, w := range []string{"Key", "RegionLabel", "Children"} {
		if !strings.Contains(raw, w) {
			t.Fatalf("%s 없음 %s", w, raw)
		}
	}
	if _, ok := payload["regions"]; !ok {
		t.Fatal("regions 없음")
	}
	var regions []repository.MonthTargetRegionStat
	if err := json.Unmarshal(payload["regions"], &regions); err != nil {
		t.Fatal(err)
	}
	var sawChild bool
	for _, rg := range regions {
		if rg.Key == "충남" {
			for _, ch := range rg.Children {
				if ch.Key == "충남|홍성군" {
					sawChild = true
				}
			}
		}
	}
	if !sawChild {
		t.Fatalf("충남|홍성군 자식 없음 %s", raw)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/"+p.PlanID+"/month-targets", strings.NewReader(url.Values{
		"month": {"2"}, "source": {"region"},
	}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("빈 선택 status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) && !strings.Contains(rec.Body.String(), `"ok": false`) {
		t.Fatalf("ok:false 없음 %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "등록할 지역을 선택하세요") {
		t.Fatalf("메시지 없음 %s", rec.Body.String())
	}

	ok := httptest.NewRecorder()
	form := url.Values{"month": {"2"}, "source": {"region"}, "region": {"충남|홍성군"}}
	oreq := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/"+p.PlanID+"/month-targets", strings.NewReader(form.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	oreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusOK {
		t.Fatalf("홍성 등록 %d %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), "홍성군") {
		t.Fatalf("지역명 없음 %s", ok.Body.String())
	}
}

func Test95_49_PlanManageMarkupAndCSS(t *testing.T) {
	root := testWebRoot(t)
	tpl, err := os.ReadFile(filepath.Join(root, "templates", "maintenance", "plan_manage.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(tpl)
	for _, w := range []string{"mt-sido", "mt-sigungu", "aria-expanded", "RegionLabel", "Children", "등록할 사이트를 선택하세요"} {
		if !strings.Contains(s, w) {
			t.Fatalf("%q 없음", w)
		}
	}
	css, err := os.ReadFile(filepath.Join(root, "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	for _, tok := range extractClassTokens(s) {
		if !strings.Contains(tok, "[") {
			continue
		}
		esc := strings.ReplaceAll(tok, "[", `\[`)
		esc = strings.ReplaceAll(esc, "]", `\]`)
		if !strings.Contains(bundle, esc) && !strings.Contains(bundle, tok) {
			t.Fatalf("임의값 클래스 %q 가 번들에 없다 — npm run build:css 필요", tok)
		}
	}
}
