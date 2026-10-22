package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newMnt9515AServer(t *testing.T) (*echo.Echo, *sql.DB, *repository.MaintenanceRepo, *http.Cookie, *http.Cookie, *http.Cookie, *http.Cookie, *http.Cookie, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "h15a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs15a", PasswordHash: HashPassword("x"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa15a", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	si := &model.User{Username: "si15a", PasswordHash: HashPassword("x"), FullName: "에스아이", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl15a", PasswordHash: HashPassword("x"), FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	ro := &model.User{Username: "ro15a", PasswordHash: HashPassword("x"), FullName: "읽기", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{vs, oa, si, cl, ro} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers
		(customer_id, org_name, official_name, is_active, party_kind, industry, org_id, addr_sido, addr_sigungu, address)
		VALUES
		('a15e1','빈학교1','빈학교1',1,'customer','학교',?,'','',''),
		('a15e2','빈학교2','빈학교2',1,'customer','학교',?,'','',''),
		('a15e3','빈학교3','빈학교3',1,'customer','학교',?,'충청남도','계룡시',''),
		('a15r1','충남학교','충남학교',1,'customer','학교',?,'충청남도','논산시',''),
		('a15si','SI학교','SI학교',1,'customer','학교',?,'','','')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, "O03"); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	for _, cfg := range []model.MaintenanceSiteConfig{
		{CustomerID: "a15e1", ShortName: "빈1", InspectionCycle: "semi", Region: ""},
		{CustomerID: "a15e2", ShortName: "빈2", InspectionCycle: "semi", Region: ""},
		{CustomerID: "a15e3", ShortName: "빈3", InspectionCycle: "monthly", Region: ""},
		{CustomerID: "a15r1", ShortName: "충남지정", InspectionCycle: "semi", Region: "충남"},
		{CustomerID: "a15si", ShortName: "SI빈", InspectionCycle: "semi", Region: ""},
	} {
		if err := repo.UpsertSiteConfig(&cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE maintenance_site_config SET org_id=? WHERE customer_id IN ('a15e1','a15e2','a15e3','a15r1')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE maintenance_site_config SET org_id='O03' WHERE customer_id='a15si'`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.GET("/maintenance/regions", h.Maintenance.RegionAssignees)
	g.POST("/maintenance/regions", h.Maintenance.SaveRegionAssignees)
	adminOnly := h.Auth.RequireAdminMW
	ms := g.Group("/maintenance/sites", adminOnly)
	ms.GET("", h.Maintenance.ListSiteConfigs)
	ms.POST("/set-region", h.Maintenance.BulkSetSiteRegions)
	ms.POST("/fill-region", h.Maintenance.FillSiteRegionsFromAddress)
	ms.GET("/bulk", h.Maintenance.SiteBulkPage)
	ms.POST("/bulk", h.Maintenance.SiteBulkRegister)
	return e, db, repo, jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, si), jwtCookieUserClaims(t, cl), jwtCookieUserClaims(t, ro), vs
}

func TestMnt95_15ARegionPageShowsUnspecified(t *testing.T) {
	e, _, _, _, oa, _, _, _, _ := newMnt9515AServer(t)
	rec := doGetWith(t, e, "/maintenance/regions", oa)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "지역 미지정") {
		t.Fatalf("빈 지역 사이트가 지역별 담당에서 사라짐")
	}
	if !strings.Contains(body, "전체 사이트 4곳") || !strings.Contains(body, "지역 지정 1곳") || !strings.Contains(body, "미지정 3곳") {
		t.Fatalf("요약 없음 %s", clip15A(body))
	}
	if !strings.Contains(body, "사이트 3곳") {
		t.Fatal("미지정 건수")
	}
	if !strings.Contains(body, "region=none") {
		t.Fatal("지역 지정하러 가기 링크")
	}
	if !strings.Contains(body, "지역이 없으면 지역 기본 담당이 적용되지 않습니다") {
		t.Fatal("안내")
	}
}

func TestMnt95_15ASIDoesNotCountLibrarySites(t *testing.T) {
	e, _, _, vs, _, si, _, _, vsUser := newMnt9515AServer(t)
	rec := doGetWith(t, e, "/maintenance/regions", si)
	if rec.Code != http.StatusOK {
		t.Fatalf("si %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "전체 사이트 4곳") || strings.Contains(body, "충남지정") {
		t.Fatal("SI가 도서관 사이트를 합산")
	}
	if !strings.Contains(body, "지역 미지정") || !strings.Contains(body, "전체 사이트 1곳") {
		t.Fatalf("SI 미지정 %s", clip15A(body))
	}
	all := doGetWith(t, e, "/maintenance/regions", vs)
	if all.Code != http.StatusOK || !strings.Contains(all.Body.String(), "전체 사이트 5곳") {
		t.Fatalf("vsadmin 전 조직 %d %s", all.Code, clip15A(all.Body.String()))
	}
	lib := doGetWith(t, e, "/maintenance/regions", jwtCookieViewOrg(t, vsUser, model.OrgIDLibrary))
	if lib.Code != http.StatusOK || !strings.Contains(lib.Body.String(), "전체 사이트 4곳") {
		t.Fatalf("vsadmin 선택 조직 %d %s", lib.Code, clip15A(lib.Body.String()))
	}
}

func TestMnt95_15ASiteListRegionNoneAndWritesForbidden(t *testing.T) {
	e, _, repo, _, oa, si, cl, ro, _ := newMnt9515AServer(t)
	page := doGetWith(t, e, "/maintenance/sites?region=none", oa)
	if page.Code != http.StatusOK {
		t.Fatalf("list %d", page.Code)
	}
	body := page.Body.String()
	if strings.Contains(body, "충남지정") {
		t.Fatal("region=none 에 지역 있는 사이트")
	}
	if !strings.Contains(body, "빈1") || !strings.Contains(body, "빈2") {
		t.Fatal("미지정 사이트 없음")
	}
	siPage := doGetWith(t, e, "/maintenance/sites?region=none", si)
	if strings.Contains(siPage.Body.String(), "빈1") {
		t.Fatal("SI 목록에 도서관 사이트")
	}
	foreign := doGetWith(t, e, "/maintenance/sites?batch=other-org-batch", oa)
	if strings.Contains(foreign.Body.String(), "빈1") || strings.Contains(foreign.Body.String(), "충남지정") {
		t.Fatal("타 조직 batch 는 빈 결과")
	}
	form := url.Values{"customer_id": {"a15e1", "a15e2", "a15si", "no-such"}, "region": {"충남"}}
	if rec := doFormUser(t, e, "/maintenance/sites/set-region", form, cl); rec.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", rec.Code)
	}
	if rec := doFormUser(t, e, "/maintenance/sites/set-region", form, ro); rec.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 %d", rec.Code)
	}
	ok := doFormUser(t, e, "/maintenance/sites/set-region", form, oa)
	if ok.Code != http.StatusOK && ok.Code != http.StatusSeeOther {
		t.Fatalf("일괄 지정 %d %s", ok.Code, ok.Body.String())
	}
	c1, _ := repo.GetSiteConfig("a15e1")
	if c1 == nil || c1.Region != "충남" {
		t.Fatalf("지정 후 %+v", c1)
	}
	siCfg, _ := repo.GetSiteConfig("a15si")
	if siCfg != nil && siCfg.Region == "충남" {
		t.Fatal("타 조직 ID 를 바꾸면 안 됨")
	}
	again := doFormUser(t, e, "/maintenance/sites/set-region", url.Values{"customer_id": {"a15e1"}, "region": {"충남"}}, oa)
	loc := again.Header().Get("Location")
	if !strings.Contains(again.Body.String(), "변경 0") && !strings.Contains(loc, "changed=0") {
		t.Fatalf("멱등 %d loc=%s body=%s", again.Code, loc, clip15A(again.Body.String()))
	}
}

func TestMnt95_15AFillFromAddressAndBulkResult(t *testing.T) {
	e, db, repo, _, oa, _, _, _, _ := newMnt9515AServer(t)
	fill := doFormUser(t, e, "/maintenance/sites/fill-region", url.Values{"customer_id": {"a15e1", "a15e3"}}, oa)
	if fill.Code != http.StatusOK && fill.Code != http.StatusSeeOther {
		t.Fatalf("채우기 %d %s", fill.Code, fill.Body.String())
	}
	empty, _ := repo.GetSiteConfig("a15e1")
	if empty != nil && empty.Region != "" {
		t.Fatal("주소 없는 행은 건너뜀")
	}
	filled, _ := repo.GetSiteConfig("a15e3")
	if filled == nil || filled.Region != "충남" {
		t.Fatalf("주소로 채움 %+v", filled)
	}
	var sido string
	if err := db.QueryRow(`SELECT COALESCE(addr_sido,'') FROM customers WHERE customer_id='a15e3'`).Scan(&sido); err != nil || sido != "충청남도" {
		t.Fatalf("고객 주소 불변 %q %v", sido, err)
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind, industry, org_id, addr_sido)
		VALUES ('a15new','신규빈','신규빈',1,'customer','학교',?,'')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	reg := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry": {"학교"}, "inspection_cycle": {"semi"}, "customer_id": {"a15new"}, "fallback_region": {"세종"},
	}, oa)
	if reg.Code != http.StatusOK && reg.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", reg.Code, reg.Body.String())
	}
	body := reg.Body.String()
	if !strings.Contains(body, "지역 없음") && !strings.Contains(body, "지역 없는") {
		t.Fatalf("결과 지역 없음 경고 %s", clip15A(body))
	}
	if !strings.Contains(body, "batch=") {
		t.Fatal("batch 링크")
	}
	cfg, _ := repo.GetSiteConfig("a15new")
	if cfg == nil || cfg.Region != "세종" {
		t.Fatalf("빈 행만 대체 지역 %+v", cfg)
	}
}

func TestMnt95_15ABulkHTMLUILintZero(t *testing.T) {
	for _, name := range []string{"regions.html", "bulk.html"} {
		b, err := os.ReadFile(filepath.Join("web", "templates", "maintenance", name))
		if err != nil {
			b, err = os.ReadFile(filepath.Join("..", "..", "web", "templates", "maintenance", name))
		}
		if err != nil {
			t.Fatal(err)
		}
		got := countUILint(string(b))
		if !got.zero() {
			t.Fatalf("%s ui_lint %+v", name, got)
		}
	}
}

func clip15A(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
