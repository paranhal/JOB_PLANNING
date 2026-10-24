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

func seedLookupHTTP(t *testing.T) (*echo.Echo, *http.Cookie, *http.Cookie) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "lkhttp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, party_kind)
		VALUES ('C-EDU','충남교육청','충청남도교육청',1,'O01',0,'customer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, parent_customer_id, party_kind, addr_sigungu)
		VALUES ('C-GONG','공주생명과학고등학교','공주생명과학고등학교',1,'O01',1,'C-EDU','customer','공주시')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, party_kind)
		VALUES ('C-O02','타조직도서관','타조직도서관',1,'O02',0,'customer')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa := &model.User{Username: "oa22", PasswordHash: "x", FullName: "관리", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	pt := &model.User{Username: "pt22", PasswordHash: "x", FullName: "협력", Role: model.RolePartner, IsActive: true, OrgID: "O01", ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(pt); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/lookup/customers", h.Customer.LookupJSON)
	g.GET("/workboard/new-work/form", h.Workboard.NewWorkForm)
	g.GET("/as/new", h.AS.New)
	g.GET("/maintenance/sites/customer-search", h.Maintenance.SearchSiteCustomers)
	g.GET("/workboard/work-sites/search", h.Workboard.SearchWorkTaskSites)
	return e, jwtCookieUser(t, oa), jwtCookieUser(t, pt)
}

func TestLookupCustomersHTTP(t *testing.T) {
	e, ck, pt := seedLookupHTTP(t)
	bad := doGetWith(t, e, "/lookup/customers?scope=nope", ck)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("scope %d %s", bad.Code, bad.Body.String())
	}
	ok := doGetWith(t, e, "/lookup/customers?scope=site&limit=50", ck)
	if ok.Code != http.StatusOK {
		t.Fatalf("lookup %d %s", ok.Code, ok.Body.String())
	}
	var body struct {
		Items []struct{ ID string } `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, it := range body.Items {
		ids[it.ID] = true
	}
	if !ids["C-EDU"] || !ids["C-GONG"] || !ids["C-O02"] {
		t.Fatalf("HTTP 집합 %+v", ids)
	}
	inc := doGetWith(t, e, "/lookup/customers?include=C-GONG&limit=8", ck)
	if !strings.Contains(inc.Body.String(), "C-GONG") {
		t.Fatalf("include %s", inc.Body.String())
	}
	anon := doGetWith(t, e, "/lookup/customers?scope=site")
	if anon.Code != http.StatusSeeOther && anon.Code != http.StatusUnauthorized && anon.Code != http.StatusForbidden {
		t.Fatalf("미로그인 %d %s", anon.Code, anon.Body.String()[:minInt(120, anon.Body.Len())])
	}
	denied := doGetWith(t, e, "/lookup/customers?scope=site", pt)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("협력사 %d %s", denied.Code, denied.Body.String())
	}
}

func jsonItemIDs(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var body struct {
		Items []struct{ ID string `json:"id"` } `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		var alt []struct {
			ID     string `json:"id"`
			CID    string `json:"customer_id"`
			SiteID string `json:"site_id"`
		}
		if err2 := json.Unmarshal(raw, &alt); err2 != nil {
			t.Fatalf("json %v %s", err, raw)
		}
		m := map[string]bool{}
		for _, it := range alt {
			id := it.ID
			if id == "" {
				id = it.CID
			}
			if id == "" {
				id = it.SiteID
			}
			if id != "" {
				m[id] = true
			}
		}
		return m
	}
	m := map[string]bool{}
	for _, it := range body.Items {
		if it.ID != "" {
			m[it.ID] = true
		}
	}
	return m
}

func TestLookupAdaptersSameIDs(t *testing.T) {
	e, ck, _ := seedLookupHTTP(t)
	q := url.QueryEscape("공주생명")
	lk := doGetWith(t, e, "/lookup/customers?scope=site&q="+q+"&limit=20", ck)
	as := doGetWith(t, e, "/as/new", ck)
	form := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-10-24", ck)
	site := doGetWith(t, e, "/maintenance/sites/customer-search?q="+q, ck)
	work := doGetWith(t, e, "/workboard/work-sites/search?q="+q, ck)
	edu := url.QueryEscape("충남교육")
	lkAll := doGetWith(t, e, "/lookup/customers?scope=site&limit=50", ck)
	if lk.Code != http.StatusOK {
		t.Fatalf("lookup %d", lk.Code)
	}
	want := jsonItemIDs(t, lk.Body.Bytes())
	if !want["C-GONG"] {
		t.Fatal("lookup 에 공주 없음")
	}
	siteIDs := jsonItemIDs(t, site.Body.Bytes())
	workIDs := jsonItemIDs(t, work.Body.Bytes())
	if !siteIDs["C-GONG"] {
		t.Fatalf("사이트 검색 %+v", siteIDs)
	}
	if !workIDs["C-GONG"] {
		t.Fatalf("작업 검색 %+v", workIDs)
	}
	all := jsonItemIDs(t, lkAll.Body.Bytes())
	if !all["C-EDU"] || !all["C-GONG"] || !all["C-O02"] {
		t.Fatalf("3종 누락 %+v", all)
	}
	_ = edu
	if as.Code == http.StatusOK && !strings.Contains(as.Body.String(), "C-GONG") && !strings.Contains(as.Body.String(), "공주생명") {
		t.Fatalf("AS 접수 목록에 공주 없음 %d", as.Code)
	}
	if form.Code != http.StatusOK {
		t.Fatalf("새 모달 %d %s", form.Code, form.Body.String()[:minInt(200, form.Body.Len())])
	}
	if !strings.Contains(form.Body.String(), "customerCombo") && !strings.Contains(form.Body.String(), "name=\"customer_id\"") {
		t.Fatalf("모달 콤보 없음 %s", form.Body.String()[:minInt(400, form.Body.Len())])
	}
	if strings.Contains(form.Body.String(), `<option value="C-GONG">`) {
		t.Fatal("새 모달이 고객 option 을 내려보낸다")
	}
}
