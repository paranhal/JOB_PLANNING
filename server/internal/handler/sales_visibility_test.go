package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestSalesVisibilityScopeRoles(t *testing.T) {
	oa := echoCtxRole(model.RoleOrgAdmin, "팀장")
	oa.Set("org_id", "O01")
	oa.Set("user_id", "oa1")
	v := SalesVisibilityScope(oa)
	if v.OrgID != "O01" || v.MineOnly || !v.ShowOwnerFilter {
		t.Fatalf("org_admin %+v", v)
	}

	s := echoCtxRole(model.RoleSales, "영업하나")
	s.Set("org_id", "O01")
	s.Set("user_id", "s1")
	v = SalesVisibilityScope(s)
	if !v.MineOnly || v.ShowOwnerFilter {
		t.Fatalf("sales %+v", v)
	}

	va := echoCtxRole(model.RoleVisionAdmin, "비전")
	va.Set("org_id", "O01")
	v = SalesVisibilityScope(va)
	if v.OrgID != repository.OrgAll || v.MineOnly || !v.ShowOwnerFilter {
		t.Fatalf("vision %+v", v)
	}

	tech := echoCtxRole(model.RoleTech, "기술")
	tech.Set("org_id", "O01")
	v = SalesVisibilityScope(tech)
	if v.MineOnly || v.ShowOwnerFilter {
		t.Fatalf("tech Q-26 %+v", v)
	}

	oa.Set("sim_role", model.RoleSales)
	oa.Set("user_name", "영업하나")
	v = SalesVisibilityScope(oa)
	if !v.MineOnly {
		t.Fatal("view-as sales 는 본인 범위")
	}
}

func TestSalesVisibilityOrgAdminSeesOrgSalesOnly(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "oa", PasswordHash: HashPassword("pw"), FullName: "조직장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "s1", PasswordHash: HashPassword("pw"), FullName: "영업하나",
		Role: model.RoleSales, IsActive: true, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "s2", PasswordHash: HashPassword("pw"), FullName: "영업둘",
		Role: model.RoleSales, IsActive: true, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	oa, _ := users.GetByUsername("oa")
	s1, _ := users.GetByUsername("s1")
	s2, _ := users.GetByUsername("s2")
	repo := repository.NewSalesRepo(db)
	mustSales := func(name, org, owner, ownerID string) string {
		t.Helper()
		p := &model.SalesProject{Name: name, OrgID: org, SalesOwner: owner, SalesOwnerID: ownerID, Stage: model.SalesStage4Discover, Status: model.SalesStatusActive, DealType: model.SalesDealBuild}
		if err := repo.Create(p); err != nil {
			t.Fatal(name, err)
		}
		if err := repo.CreateActivity(&model.SalesActivity{
			SalesID: p.SalesID, ActivityDate: "2026-10-10", ActivityType: "visit", Title: name + "방문",
		}, owner, nil); err != nil {
			t.Fatal("act", err)
		}
		return p.Name
	}
	mustSales("A1사업", "O01", "영업하나", s1.UserID)
	mustSales("A2사업", "O01", "영업둘", s2.UserID)
	mustSales("B사업", "O02", "다른영업", "")

	g := e.Group("")
	h := New(db)
	g.GET("/quotes", h.Quotes.List)
	g.GET("/orders", h.Orders.List)

	oaPage := doGetWith(t, e, "/sales", jwtCookieUser(t, oa))
	if oaPage.Code != http.StatusOK {
		t.Fatalf("org_admin list %d", oaPage.Code)
	}
	body := oaPage.Body.String()
	if !strings.Contains(body, "A1사업") || !strings.Contains(body, "A2사업") {
		t.Fatal("조직관리자에게 O01 사업이 없다")
	}
	if strings.Contains(body, "B사업") {
		t.Fatal("조직관리자에게 O02 사업이 보인다")
	}
	if !strings.Contains(body, "영업담당") {
		t.Fatal("담당자 필터가 없다")
	}

	s1Page := doGetWith(t, e, "/sales", jwtCookieUser(t, s1))
	b1 := s1Page.Body.String()
	if !strings.Contains(b1, "A1사업") {
		t.Fatal("영업하나 본인 사업 없음")
	}
	if strings.Contains(b1, "A2사업") || strings.Contains(b1, "B사업") {
		t.Fatal("영업하나가 남 사업/타 조직을 본다")
	}

	actOA := doGetWith(t, e, "/sales/activities?month=2026-10", jwtCookieUser(t, oa))
	ab := actOA.Body.String()
	if !strings.Contains(ab, "A1사업방문") || !strings.Contains(ab, "A2사업방문") {
		t.Fatal("조직관리자 활동 범위")
	}
	if strings.Contains(ab, "B사업방문") {
		t.Fatal("조직관리자에게 O02 활동")
	}

	actS1 := doGetWith(t, e, "/sales/activities?month=2026-10", jwtCookieUser(t, s1))
	as1 := actS1.Body.String()
	if !strings.Contains(as1, "A1사업방문") {
		t.Fatal("영업하나 본인 활동 없음")
	}
	if strings.Contains(as1, "A2사업방문") {
		t.Fatal("영업하나가 동료 활동을 본다")
	}

	onlyA2 := doGetWith(t, e, "/sales?owner=영업둘", jwtCookieUser(t, oa))
	if !strings.Contains(onlyA2.Body.String(), "A2사업") || strings.Contains(onlyA2.Body.String(), "A1사업") {
		t.Fatal("담당자 필터")
	}

	dash := doGetWith(t, e, "/sales/dashboard", jwtCookieUser(t, oa))
	if dash.Code != http.StatusOK || strings.Contains(dash.Body.String(), "B사업") {
		t.Fatalf("대시보드 O02 누수 status=%d", dash.Code)
	}
	pipe := doGetWith(t, e, "/sales?view=pipeline", jwtCookieUser(t, oa))
	if pipe.Code != http.StatusOK || strings.Contains(pipe.Body.String(), "B사업") {
		t.Fatalf("파이프라인 O02 누수 status=%d", pipe.Code)
	}
}

func TestSalesListHandlersUseVisibilityHelpers(t *testing.T) {
	root := "."
	for _, cand := range []string{".", "internal/handler"} {
		if _, err := os.Stat(filepath.Join(cand, "sales.go")); err == nil {
			root = cand
			break
		}
	}
	files := []string{
		filepath.Join(root, "sales.go"),
		filepath.Join(root, "sales_activity_ui.go"),
		filepath.Join(root, "sales_pipeline_xlsx.go"),
		filepath.Join(root, "quotes.go"),
		filepath.Join(root, "orders.go"),
	}
	need := map[string]string{
		"sales.go":               "applySalesVisibilityToList",
		"sales_activity_ui.go":   "visibleActivityFilter",
		"sales_pipeline_xlsx.go": "applySalesVisibilityToList",
		"quotes.go":              "visibleQuoteFilter",
		"orders.go":              "visibleOrderFilter",
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(f, err)
		}
		base := filepath.Base(f)
		if !strings.Contains(string(b), need[base]) {
			t.Fatalf("%s 에 %s 없음", base, need[base])
		}
		if !strings.Contains(string(b), "SalesVisibilityScope") && base == "sales.go" {
			t.Fatal("sales.go SalesVisibilityScope")
		}
	}
}

func TestSalesVisibilityDoesNotChangeActivityWriteRule(t *testing.T) {
	oa := echoCtxRole(model.RoleOrgAdmin, "팀장")
	oa.Set("org_id", "O01")
	if !canEditSalesActivity(oa, "영업하나") {
		t.Fatal("조직관리자 쓰기 규칙은 유지(남의 활동 수정 가능)")
	}
	s := echoCtxRole(model.RoleSales, "영업하나")
	s.Set("org_id", "O01")
	s.Set("user_name", "영업하나")
	if canEditSalesActivity(s, "영업둘") {
		t.Fatal("영업 담당자가 남의 활동을 수정하면 안 된다")
	}
}

func TestSalesDashboardRouteOK(t *testing.T) {
	e := newSalesServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/sales/dashboard", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}
