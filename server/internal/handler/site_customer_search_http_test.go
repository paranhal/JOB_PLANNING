package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSiteCustomerSearchHTTP(t *testing.T) (*echo.Echo, *repository.MaintenanceRepo, *http.Cookie, *http.Cookie, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sitesearch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs96", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	o3 := &model.User{Username: "oa03s", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	for _, u := range []*model.User{vs, o3} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido)
		VALUES ('c-gy','계룡고등학교','계룡고등학교',1,'O03','충남'),
		       ('c-done','계룡중','계룡중',1,'O03','충남'),
		       ('c-lib','계룡도서관','계룡도서관',1,'O01','대전')`); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	if err := mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c-done", ShortName: "계룡중", InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/sites/new", h.Maintenance.NewSiteConfigPage)
	g.GET("/maintenance/sites/customer-search", h.Maintenance.SearchSiteCustomers)
	g.POST("/maintenance/sites", h.Maintenance.SaveSiteConfig, h.Auth.RequireMasterWrite)
	g.GET("/maintenance/sites/:customer_id/edit", h.Maintenance.EditSiteConfigPage)
	return e, mnt, jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, o3), vs
}

func TestSiteCustomerSearchHTTP(t *testing.T) {
	e, mnt, _, o3, vs := newSiteCustomerSearchHTTP(t)

	form := doGetWith(t, e, "/maintenance/sites/new", o3)
	if form.Code != http.StatusOK {
		t.Fatalf("form %d", form.Code)
	}
	body := form.Body.String()
	if strings.Contains(body, `<option value="">선택</option>`) || strings.Contains(body, ">선택</option>") {
		t.Fatal("선택 옵션이 남아 있음")
	}
	if !strings.Contains(body, `id="site-customer-q"`) || !strings.Contains(body, `role="combobox"`) {
		t.Fatal("콤보박스 없음")
	}
	if strings.Contains(body, `id="site-customer-q"`) && strings.Contains(body, `value="선택"`) {
		t.Fatal("입력 초깃값이 선택")
	}

	one := doGetWith(t, e, "/maintenance/sites/customer-search?q="+url.QueryEscape("계"), o3)
	if one.Code != http.StatusOK {
		t.Fatalf("1글자 %d", one.Code)
	}
	var oneJ struct {
		Items []repository.SiteCustomerHit `json:"items"`
		Hint  string                       `json:"hint"`
	}
	if err := json.Unmarshal(one.Body.Bytes(), &oneJ); err != nil {
		t.Fatal(err)
	}
	if len(oneJ.Items) != 0 || !strings.Contains(oneJ.Hint, "두 글자") {
		t.Fatalf("1글자 %+v", oneJ)
	}

	ok := doGetWith(t, e, "/maintenance/sites/customer-search?q="+url.QueryEscape("계룡"), o3)
	var okJ struct {
		Items []repository.SiteCustomerHit `json:"items"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &okJ); err != nil {
		t.Fatal(err)
	}
	if len(okJ.Items) != 2 {
		t.Fatalf("검색 %+v", okJ.Items)
	}
	ids := map[string]bool{}
	for _, it := range okJ.Items {
		ids[it.ID] = true
	}
	if !ids["c-gy"] || !ids["c-lib"] || ids["c-done"] {
		t.Fatalf("전 기관 미설정만 %+v", okJ.Items)
	}

	all := doGetWith(t, e, "/maintenance/sites/customer-search?q="+url.QueryEscape("계룡"), jwtCookieViewOrg(t, vs, repository.OrgAll))
	var allJ struct {
		Items []repository.SiteCustomerHit `json:"items"`
		Hint  string                       `json:"hint"`
	}
	if err := json.Unmarshal(all.Body.Bytes(), &allJ); err != nil {
		t.Fatal(err)
	}
	if len(allJ.Items) != 2 || allJ.Hint != "" {
		t.Fatalf("전체보기 %+v", allJ)
	}
	picked := doGetWith(t, e, "/maintenance/sites/customer-search?q="+url.QueryEscape("계룡"), jwtCookieViewOrg(t, vs, "O03"))
	var pickedJ struct {
		Items []repository.SiteCustomerHit `json:"items"`
	}
	_ = json.Unmarshal(picked.Body.Bytes(), &pickedJ)
	if len(pickedJ.Items) != 2 {
		t.Fatalf("조직 선택 후 %+v", pickedJ.Items)
	}

	needOrg := doGetWith(t, e, "/maintenance/sites/new", jwtCookieViewOrg(t, vs, repository.OrgAll))
	if !strings.Contains(needOrg.Body.String(), "소속 조직을 선택하세요") {
		t.Fatal("조직 안내 없음")
	}

	miss := postForm947(t, e, "/maintenance/sites", url.Values{
		"short_name": {"표시유지"}, "region": {"세종"},
	}, o3)
	if miss.Code != http.StatusBadRequest || !strings.Contains(miss.Body.String(), errNeedSiteCustomer) {
		t.Fatalf("미선택 %d %s", miss.Code, miss.Body.String()[:minInt(240, miss.Body.Len())])
	}
	if !strings.Contains(miss.Body.String(), "표시유지") || !strings.Contains(miss.Body.String(), "세종") {
		t.Fatal("입력값 유지 없음")
	}

	foreign := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c-lib"}, "short_name": {"도서관"}, "inspection_cycle": {"monthly"},
	}, o3)
	if foreign.Code != http.StatusSeeOther {
		t.Fatalf("타조직 저장 %d %s", foreign.Code, foreign.Body.String()[:minInt(240, foreign.Body.Len())])
	}

	save := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c-gy"}, "short_name": {"계룡고"}, "inspection_cycle": {"monthly"},
	}, o3)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String()[:minInt(200, save.Body.Len())])
	}
	got, err := mnt.GetSiteConfig("c-gy")
	if err != nil || got == nil || got.ShortName != "계룡고" {
		t.Fatalf("저장 확인 %+v %v", got, err)
	}
	edit := doGetWith(t, e, "/maintenance/sites/c-gy/edit", o3)
	if edit.Code != http.StatusOK {
		t.Fatalf("수정 %d", edit.Code)
	}
	if strings.Contains(edit.Body.String(), `id="site-customer-q"`) {
		t.Fatal("수정 모드에 콤보박스")
	}
	if !strings.Contains(edit.Body.String(), `name="customer_id"`) || !strings.Contains(edit.Body.String(), "c-gy") {
		t.Fatal("수정 모드 고객 고정 없음")
	}
}
