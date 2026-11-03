package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_87PromoteSIOrgNotLibrary(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: "oa03-87", PasswordHash: HashPassword("pw"), FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := repository.NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	got, err := repository.NewUserRepo(db).GetByUsername("oa03-87")
	if err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, got)
	rec := doFormWith(t, e, "/sales", url.Values{"name": {"SI수주사업87"}}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("영업 등록 %d %s", rec.Code, rec.Body.String())
	}
	sid := salesIDFromRedirect(t, rec.Header().Get("Location"))
	var sorg string
	if err := db.QueryRow(`SELECT org_id FROM sales_projects WHERE sales_id=?`, sid).Scan(&sorg); err != nil || sorg != "O03" {
		t.Fatalf("영업 org=%q %v", sorg, err)
	}
	rec = doFormWith(t, e, "/sales/"+sid+"/stage", url.Values{"stage": {"direct_won"}}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("수주 %d", rec.Code)
	}
	rec = doFormWith(t, e, "/sales/"+sid+"/close-contract", url.Values{
		"contracted_at": {"2026-09-01"}, "contract_amount": {"1000000"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("계약 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO customers(customer_id,org_name,official_name,is_active) VALUES('C87','고객87','고객87',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET customer_id='C87' WHERE sales_id=?`, sid); err != nil {
		t.Fatal(err)
	}
	form := doGetWith(t, e, "/sales/"+sid+"/promote", ck)
	if form.Code != http.StatusOK {
		t.Fatalf("전환 폼 %d", form.Code)
	}
	body := form.Body.String()
	if !strings.Contains(body, "주관 부서") || !strings.Contains(body, "유관 부서") {
		t.Fatalf("전환 폼에 주관/유관 없음: %s", clipBody(body))
	}
	rec = doFormWith(t, e, "/sales/"+sid+"/promote", url.Values{
		"name": {"SI수주사업87"}, "customer_id": {"C87"}, "is_paid": {"1"}, "org_id": {"O01"},
	}, ck)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "/projects/") {
		t.Fatalf("전환 저장 %d loc=%s body=%s", rec.Code, loc, clipBody(rec.Body.String()))
	}
	var worg string
	if err := db.QueryRow(`SELECT COALESCE(org_id,'') FROM work_projects WHERE sales_project_id=?`, sid).Scan(&worg); err != nil {
		t.Fatal(err)
	}
	if worg != "O03" {
		t.Fatalf("전환 사업 org=%q (O01 보정 재발)", worg)
	}
}

func Test95_87VisionCreateRequiresOrg(t *testing.T) {
	e, _, _ := newProjectServer(t)
	ck := jwtCookieOrg(t, model.RoleVisionAdmin, "")
	rec := doFormWith(t, e, "/projects", url.Values{"name": {"비전미선택"}, "project_kind": {"build"}}, ck)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("비젼 미선택인데 저장됨")
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "소속 조직") {
		t.Fatalf("오류 표시 아님 %d %s", rec.Code, clipBody(rec.Body.String()))
	}
}

func Test95_87OrgAdminSalesFormAndRejectForeignOrg(t *testing.T) {
	e, db := newSalesServerDB(t)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: "oa03-form", PasswordHash: HashPassword("pw"), FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := repository.NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	got, _ := repository.NewUserRepo(db).GetByUsername("oa03-form")
	ck := jwtCookieUser(t, got)
	form := doGetWith(t, e, "/sales/new", ck)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "주관 부서") {
		t.Fatalf("영업 폼 주관 없음 %d", form.Code)
	}
	rec := doFormWith(t, e, "/sales", url.Values{"name": {"위조조직"}, "org_id": {"O01"}}, ck)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("타 조직 등록 허용")
	}
}

func Test95_87RelatedWorkRegisterAndReject(t *testing.T) {
	e, repo, wb := newProjectServer(t)
	p := &model.WorkProject{Name: "유관업무사업", OrgID: "O01", Status: model.WBProjectActive, ContractAmount: 1, ProjectKind: model.ProjectKindBuild, ProjectKindSet: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	pick, err := wb.ListWorkPickProjects("O01")
	if err != nil || !namesOfProjects(pick)[p.Name] {
		t.Fatalf("주관 콤보 %+v %v", pick, err)
	}
	o3, err := wb.ListWorkPickProjects("O03")
	if err != nil || namesOfProjects(o3)[p.Name] {
		t.Fatalf("비유관 콤보에 있음 %+v", o3)
	}
	got, err := repo.Get(p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !model.CanRegisterWorkOnProject(*got, "O01") {
		t.Fatal("주관 등록 거절")
	}
	if model.CanRegisterWorkOnProject(*got, "O03") {
		t.Fatal("비유관 등록 허용")
	}
	reg := doGet(t, e, "/projects/registry")
	if !strings.Contains(reg.Body.String(), "유관 부서") {
		t.Fatal("목록 필터 없음")
	}
	show := doGet(t, e, "/projects/"+p.ProjectID)
	if !strings.Contains(show.Body.String(), "주관 부서") {
		t.Fatal("상세 주관 없음")
	}
	edit := doGet(t, e, "/projects/"+p.ProjectID+"/edit")
	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), "주관 부서") {
		t.Fatalf("수정 폼 %d", edit.Code)
	}
}

func namesOfProjects(items []model.WorkProject) map[string]bool {
	out := map[string]bool{}
	for _, p := range items {
		out[p.Name] = true
	}
	return out
}
