package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type g95Users struct {
	hjlee, bUser, oa, vis, cl *model.User
}

func seed95_2G(t *testing.T) (*echo.Echo, *sql.DB, *repository.SalesRepo, g95Users) {
	t.Helper()
	e, db := newSalesServerDB(t)
	AttachHTTPErrorHandler(e)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어사업팀',2),
		('O03','O03','SI사업팀','SI사업팀',3)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	mustU := func(username, name, role, org string) *model.User {
		t.Helper()
		if err := users.Create(&model.User{
			Username: username, PasswordHash: HashPassword("pw"), FullName: name,
			Role: role, IsActive: true, OrgID: org,
		}); err != nil {
			t.Fatal(username, err)
		}
		u, err := users.GetByUsername(username)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	u := g95Users{
		hjlee: mustU("hjlee", "이해진", model.RoleSales, model.OrgIDLibrary),
		bUser: mustU("buser", "SI영업", model.RoleSales, "O03"),
		oa:    mustU("oa-lib", "도서관장", model.RoleOrgAdmin, model.OrgIDLibrary),
		cl:    mustU("clevel", "경영진", model.RoleCLevel, model.OrgIDLibrary),
		vis:   mustU("vsadmin", "비전관리자", model.RoleVisionAdmin, model.OrgIDLibrary),
	}
	return e, db, repository.NewSalesRepo(db), u
}

func mustSales95(t *testing.T, repo *repository.SalesRepo, db *sql.DB, name, org, owner, ownerID string, contracted bool) *model.SalesProject {
	t.Helper()
	p := &model.SalesProject{
		Name: name, OrgID: org, SalesOwner: owner, SalesOwnerID: ownerID,
		Stage: model.SalesStage4Discover, Status: model.SalesStatusActive, DealType: model.SalesDealBuild,
		ExpectedAmount: 1000000,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(name, err)
	}
	if contracted {
		if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=?, contracted_at=?, contract_amount=? WHERE sales_id=?`,
			model.SalesStage4Closed, model.SalesCloseContracted, model.SalesStatusContracted, "2026-10-01", 1000000, p.SalesID); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(p.SalesID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	return p
}

func postFormCookie(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMETextHTML)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestSales95_2GCurrentOrgAndEmptyUnionFallback(t *testing.T) {
	hj := echoCtxRole(model.RoleSales, "이해진")
	hj.Set("org_id", model.OrgIDLibrary)
	hj.Set("user_id", "hjlee")
	if currentOrg(hj) != model.OrgIDLibrary {
		t.Fatalf("hjlee currentOrg=%s (sim 없이 홈 조직이어야 함)", currentOrg(hj))
	}
	if currentUserHomeOrg(hj) != model.OrgIDLibrary {
		t.Fatalf("home=%s", currentUserHomeOrg(hj))
	}

	sim := echoCtxRole(model.RoleVisionAdmin, "비전")
	sim.Set("org_id", model.OrgIDLibrary)
	sim.Set("sim_role", model.RoleVisionAdmin)
	if currentOrg(sim) != repository.OrgAll {
		t.Fatalf("sim_role vision currentOrg=%s want OrgAll — vsadmin 시점보기면 전 조직이 보인다", currentOrg(sim))
	}

	e, db, repo, _ := seed95_2G(t)
	_ = e
	mustSales95(t, repo, db, "빈조직미전환", "", "누구", "", true)
	if _, err := db.Exec(`UPDATE sales_projects SET org_id='' WHERE name='빈조직미전환'`); err != nil {
		t.Fatal(err)
	}
	prepo := repository.NewProjectRepo(db)
	all, err := prepo.ListContractUnion(repository.ContractUnionFilter{OrgID: repository.OrgAll})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range all {
		if r.Name == "빈조직미전환" {
			found = true
		}
	}
	if !found {
		t.Fatal("vsadmin OrgAll 계약 목록에 빈 org 건이 없다")
	}
	empty, err := prepo.ListContractUnion(repository.ContractUnionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range empty {
		if r.Name == "빈조직미전환" {
			t.Fatal("빈 OrgID ListContractUnion 이 OrgAll 로 폴백됐다")
		}
	}
}

func TestSales95_2GHjleeDashboardHidesOtherOrgUnlessOwner(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	pb := mustSales95(t, repo, db, "P_B", "O03", u.bUser.FullName, u.bUser.UserID, true)
	mustSales95(t, repo, db, "P_A", model.OrgIDLibrary, "다른사람", "other-owner", false)
	ck := jwtCookieUser(t, u.hjlee)

	dash := doGetWith(t, e, "/sales/dashboard", ck)
	if dash.Code != http.StatusOK {
		t.Fatalf("dashboard %d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "P_B") {
		t.Fatal("hjlee 영업 대시보드에 P_B가 보인다")
	}
	pd := doGetWith(t, e, "/projects/dashboard", ck)
	if strings.Contains(pd.Body.String(), "P_B") {
		t.Fatal("hjlee 프로젝트 대시보드에 P_B")
	}

	if _, err := db.Exec(`UPDATE sales_projects SET sales_owner_id=?, sales_owner=? WHERE sales_id=?`, u.hjlee.UserID, u.hjlee.FullName, pb.SalesID); err != nil {
		t.Fatal(err)
	}
	dash2 := doGetWith(t, e, "/sales/dashboard", ck)
	if !strings.Contains(dash2.Body.String(), "P_B") || !strings.Contains(dash2.Body.String(), "계약으로 전환") {
		t.Fatal("Owner면 대시보드·전환 링크가 있어야 한다")
	}
}

func TestSales95_2GWriteGuardsAndActivityException(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	pb := mustSales95(t, repo, db, "P_B", "O03", u.bUser.FullName, u.bUser.UserID, true)
	pa := mustSales95(t, repo, db, "P_A", model.OrgIDLibrary, "다른사람", "other-owner", true)
	ck := jwtCookieUser(t, u.hjlee)

	assert403 := func(name string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s status=%d", name, rec.Code)
		}
		if rec.Code == http.StatusSeeOther {
			loc := rec.Header().Get("Location")
			flash := false
			for _, ck := range rec.Result().Cookies() {
				if ck.Name == "jp_flash_err" && ck.Value != "" {
					flash = true
				}
			}
			if !flash && !strings.Contains(loc, "err=") {
				t.Fatalf("%s 403 팝업 없음 loc=%s", name, loc)
			}
		}
		if rec.Code == http.StatusForbidden && !strings.Contains(rec.Body.String(), "다른 조직") && rec.Body.Len() > 0 {
			// DefaultHTTPErrorHandler 본문
		}
	}

	assert403("stage", postFormCookie(t, e, "/sales/"+pb.SalesID+"/stage", url.Values{"stage": {model.SalesStage4Propose}}, ck))
	assert403("promote GET", doGetWith(t, e, "/sales/"+pb.SalesID+"/promote", ck))
	assert403("promote POST", postFormCookie(t, e, "/sales/"+pb.SalesID+"/promote", url.Values{"name": {"x"}}, ck))
	assert403("contract-dialog", doGetWith(t, e, "/sales/"+pb.SalesID+"/contract-dialog", ck))
	assert403("edit", doGetWith(t, e, "/sales/"+pb.SalesID+"/edit", ck))
	assert403("update", postFormCookie(t, e, "/sales/"+pb.SalesID, url.Values{"name": {"해킹"}}, ck))

	act := postFormCookie(t, e, "/sales/"+pb.SalesID+"/activities", url.Values{
		"activity_date": {"2026-10-20"}, "activity_type": {"other"}, "activity_method": {"visit"}, "title": {"타조직방문"},
	}, ck)
	if act.Code != http.StatusSeeOther {
		t.Fatalf("활동 등록 status=%d body=%s", act.Code, act.Body.String())
	}

	homeEdit := doGetWith(t, e, "/sales/"+pa.SalesID+"/edit", ck)
	if homeEdit.Code != http.StatusOK {
		t.Fatalf("홈 조직 P_A 수정 %d", homeEdit.Code)
	}
	homeProm := doGetWith(t, e, "/sales/"+pa.SalesID+"/promote", ck)
	if homeProm.Code != http.StatusOK && homeProm.Code != http.StatusSeeOther {
		t.Fatalf("홈 조직 promote %d", homeProm.Code)
	}
}

func TestSales95_2GOrgAdminViewOrgBypassAndVisionCLevel(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	pb := mustSales95(t, repo, db, "P_B", "O03", u.bUser.FullName, u.bUser.UserID, true)
	oaCK := jwtCookieUser(t, u.oa)
	got := doGetWith(t, e, "/sales/"+pb.SalesID+"/promote?view_org_id=O03", oaCK)
	if got.Code != http.StatusForbidden && got.Code != http.StatusSeeOther {
		t.Fatalf("org_admin view_org_id 우회 status=%d", got.Code)
	}
	if got.Code == http.StatusOK {
		t.Fatal("org_admin이 P_B promote 화면을 열었다")
	}

	vis := doGetWith(t, e, "/sales/"+pb.SalesID+"/promote", jwtCookieUser(t, u.vis))
	if vis.Code != http.StatusOK && vis.Code != http.StatusSeeOther {
		t.Fatalf("vision promote %d", vis.Code)
	}

	cl := doGetWith(t, e, "/sales/"+pb.SalesID, jwtCookieUser(t, u.cl))
	if cl.Code != http.StatusOK {
		t.Fatalf("c_level 조회 %d", cl.Code)
	}
	if strings.Contains(cl.Body.String(), `href="/sales/`+pb.SalesID+`/promote"`) {
		t.Fatal("C-Level에 전환 링크")
	}
	clP := doGetWith(t, e, "/sales/"+pb.SalesID+"/promote", jwtCookieUser(t, u.cl))
	if clP.Code == http.StatusOK {
		t.Fatal("C-Level promote 허용")
	}
}

func TestSales95_2GEmptyOrgDashboardAndListRead(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	p := mustSales95(t, repo, db, "빈조직건", "O03", u.bUser.FullName, u.bUser.UserID, true)
	if _, err := db.Exec(`UPDATE sales_projects SET org_id='' WHERE sales_id=?`, p.SalesID); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, u.hjlee)
	dash := doGetWith(t, e, "/sales/dashboard", ck)
	if strings.Contains(dash.Body.String(), "빈조직건") {
		t.Fatal("빈 org_id 사업이 대시보드 전체 폴백으로 노출")
	}
	list := doGetWith(t, e, "/sales", ck)
	if !strings.Contains(list.Body.String(), "빈조직건") {
		t.Fatal("사업 목록 전 직원 조회에 빈 org 건이 없다")
	}
}

func TestSales95_2GListRenderBadgeNoConvertForForeign(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	mustSales95(t, repo, db, "P_B", "O03", u.bUser.FullName, u.bUser.UserID, true)
	ck := jwtCookieUser(t, u.hjlee)
	list := doGetWith(t, e, "/sales", ck)
	body := list.Body.String()
	if !strings.Contains(body, "P_B") {
		t.Fatal("목록에 타 조직 사업이 없다(95-2E 조회)")
	}
	if !strings.Contains(body, "SI사업팀") {
		t.Fatal("조직 뱃지 없음")
	}
	if !strings.Contains(body, "타 조직 사업 · Owner만 변경") {
		t.Fatal("툴팁 없음")
	}
	pipe := doGetWith(t, e, "/sales?view=pipeline", ck)
	if strings.Contains(pipe.Body.String(), `aria-label="P_B 계약으로 전환"`) {
		t.Fatal("파이프라인에 타 조직 전환 버튼")
	}
}

func TestSales95_2GOwnerPromoteSucceeds(t *testing.T) {
	e, db, repo, u := seed95_2G(t)
	cust := &model.Customer{OrgName: "SI고객", OfficialName: "SI고객", IsActive: true}
	if err := repository.NewCustomerRepo(db).Create(cust); err != nil {
		t.Fatal(err)
	}
	pb := mustSales95(t, repo, db, "P_B_OWN", "O03", u.hjlee.FullName, u.hjlee.UserID, true)
	if _, err := db.Exec(`UPDATE sales_projects SET customer_id=? WHERE sales_id=?`, cust.CustomerID, pb.SalesID); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, u.hjlee)
	form := doGetWith(t, e, "/sales/"+pb.SalesID+"/promote", ck)
	if form.Code != http.StatusOK {
		t.Fatalf("Owner promote GET %d", form.Code)
	}
	rec := postFormCookie(t, e, "/sales/"+pb.SalesID+"/promote", url.Values{
		"name": {"P_B_OWN"}, "customer_id": {cust.CustomerID}, "plan_year": {"2026"},
		"is_paid": {"1"}, "contract_type": {"private"}, "billing_type": {"yearly"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"},
	}, ck)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/projects/") {
		t.Fatalf("Owner promote 저장 status=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}
