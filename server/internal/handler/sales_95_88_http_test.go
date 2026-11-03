package handler

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_88SalesFormTentativeCustomerComboOwners(t *testing.T) {
	e, db := newSalesServerDB(t)
	page := doGet(t, e, "/sales/new")
	if page.Code != 200 {
		t.Fatalf("new %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, `name="tentative_name"`) || !strings.Contains(body, `x-model="isTentative"`) {
		t.Fatal("가칭 입력칸이 없다")
	}
	if strings.Contains(body, `<select name="customer_id"`) {
		t.Fatal("고객 select 가 남아 있다")
	}
	if !strings.Contains(body, `customerCombo(`) || !strings.Contains(body, `name="customer_id"`) {
		t.Fatal("고객 콤보가 없다")
	}
	if strings.Contains(body, `printf "%q"`) || strings.Contains(body, `customerId: "\"\""`) {
		t.Fatal("printf percent-q 또는 이중 따옴표 customerId")
	}
	if !strings.Contains(body, `name="sales_owner_id"`) || !strings.Contains(body, "data-assignee-picker") || !strings.Contains(body, "orgCount") {
		t.Fatal("영업담당 조직 펼침이 없다")
	}

	rec := doForm(t, e, "/sales", url.Values{
		"is_tentative_name": {"1"},
		"tentative_name":    {"가칭만사업"},
		"prospect_name":     {"직접기관"},
	})
	if rec.Code != 303 {
		t.Fatalf("가칭 저장 %d %s", rec.Code, rec.Body.String())
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	var name, tent, prospect string
	var flag int
	if err := db.QueryRow(`SELECT name, COALESCE(is_tentative_name,0), COALESCE(tentative_name,''), COALESCE(prospect_name,'') FROM sales_projects WHERE sales_id=?`, id).
		Scan(&name, &flag, &tent, &prospect); err != nil {
		t.Fatal(err)
	}
	if name != "가칭만사업" || flag != 1 || tent != "가칭만사업" {
		t.Fatalf("가칭 저장 name=%q tent=%q flag=%d", name, tent, flag)
	}
	if prospect != "직접기관" {
		t.Fatalf("기관명=%q", prospect)
	}

	cust := &model.Customer{OrgName: "등록고객배타", IsActive: true}
	if err := repository.NewCustomerRepo(db).Create(cust); err != nil {
		t.Fatal(err)
	}
	rec = doForm(t, e, "/sales", url.Values{
		"name":          {"배타고객"},
		"customer_id":   {cust.CustomerID},
		"prospect_name": {"직접도같이"},
	})
	if rec.Code != 303 {
		t.Fatalf("배타 저장 %d %s", rec.Code, rec.Body.String())
	}
	xid := salesIDFromRedirect(t, rec.Header().Get("Location"))
	var cid, pnm string
	if err := db.QueryRow(`SELECT COALESCE(customer_id,''), COALESCE(prospect_name,'') FROM sales_projects WHERE sales_id=?`, xid).Scan(&cid, &pnm); err != nil {
		t.Fatal(err)
	}
	if cid != cust.CustomerID || pnm != "" {
		t.Fatalf("고객 선택 시 기관명이 비어야 한다 cid=%q prospect=%q", cid, pnm)
	}

	rec = doForm(t, e, "/sales", url.Values{
		"is_tentative_name": {"1"},
		"tentative_name":    {""},
	})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "가칭을 입력하세요") {
		t.Fatalf("가칭 필수 거절: %d %s", rec.Code, rec.Body.String())
	}

	show := doGet(t, e, "/sales/"+id)
	if !strings.Contains(show.Body.String(), "(가칭)") {
		t.Fatal("상세 (가칭) 배지 없음")
	}
	list := doGet(t, e, "/sales?view=list")
	pipe := doGet(t, e, "/sales")
	if !strings.Contains(list.Body.String(), "(가칭)") {
		t.Fatal("목록 (가칭) 배지 없음")
	}
	if !strings.Contains(pipe.Body.String(), "(가칭)") {
		t.Fatal("파이프라인 (가칭) 배지 없음")
	}
}

func Test95_88PromoteYearDatesSameRowAndRejectEndBeforeStart(t *testing.T) {
	e, db := newSalesServerDB(t)
	rec := doForm(t, e, "/sales", url.Values{
		"name": {"연도전환"}, "expected_ym": {"2026-09"}, "expected_precision": {"month"},
	})
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=?, contracted_at=?, contract_amount=?, customer_id=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, model.SalesStatusContracted, "2026-10-01", 1000, "", id); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT customer_id FROM customers LIMIT 1`).Scan(new(string))
	cust := &model.Customer{OrgName: "전환고객", IsActive: true}
	if err := repository.NewCustomerRepo(db).Create(cust); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET customer_id=? WHERE sales_id=?`, cust.CustomerID, id); err != nil {
		t.Fatal(err)
	}

	form := doGet(t, e, "/sales/"+id+"/promote")
	body := form.Body.String()
	if form.Code != 200 {
		t.Fatalf("promote form %d", form.Code)
	}
	iYear := strings.Index(body, `name="plan_year"`)
	iStart := strings.Index(body, `name="start_date"`)
	iEnd := strings.Index(body, `name="end_date"`)
	if iYear < 0 || iStart < 0 || iEnd < 0 || !(iYear < iStart && iStart < iEnd) {
		t.Fatalf("연도·시작·마감 한 줄 순서 year=%d start=%d end=%d", iYear, iStart, iEnd)
	}
	if !strings.Contains(body, `projectYearDates(`) {
		t.Fatal("공용 연도·기간 스크립트 없음")
	}
	if !strings.Contains(body, "2026") {
		t.Fatal("영업 예상 연도 기본값이 없다")
	}
	if strings.Contains(body, `value="2008"`) {
		t.Fatal("사업 연도 2008")
	}
	if !strings.Contains(body, "회계 담당") || !strings.Contains(body, "assign_accounting") || !strings.Contains(body, "data-assignee-picker") {
		t.Fatal("담당자 조직 펼침 없음")
	}

	bad := doForm(t, e, "/sales/"+id+"/promote", url.Values{
		"name": {"연도전환"}, "customer_id": {cust.CustomerID}, "plan_year": {"2026"},
		"project_kind": {"build"}, "start_date": {"2026-12-01"}, "end_date": {"2026-01-01"},
	})
	if bad.Code != 200 || !strings.Contains(bad.Body.String(), "마감은 시작일보다") {
		t.Fatalf("마감<시작 거절 실패 %d %s", bad.Code, bad.Body.String())
	}

	yr := doForm(t, e, "/sales/"+id+"/promote", url.Values{
		"name": {"연도전환"}, "customer_id": {cust.CustomerID}, "plan_year": {"1999"},
		"project_kind": {"build"},
	})
	if yr.Code != 200 || !strings.Contains(yr.Body.String(), "2000") {
		t.Fatalf("연도 범위 거절 실패 %d %s", yr.Code, yr.Body.String())
	}
}

func Test95_88SalesOwnersMultiAndAccountingReject(t *testing.T) {
	e, db, sales, u := seed95_2G(t)
	p := mustSales95(t, sales, db, "공동담당", model.OrgIDLibrary, u.hjlee.FullName, u.hjlee.UserID, false)
	rec := doForm(t, e, "/sales/"+p.SalesID, url.Values{
		"name":           {"공동담당"},
		"sales_owner_id": {u.hjlee.UserID, u.oa.UserID},
	})
	if rec.Code != 303 {
		t.Fatalf("공동 담당 저장 %d %s", rec.Code, rec.Body.String())
	}
	got, err := sales.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SalesOwnerID != u.hjlee.UserID {
		t.Fatalf("대표 담당 %s", got.SalesOwnerID)
	}
	if len(got.SalesOwnerIDs) < 2 {
		t.Fatalf("owners=%v", got.SalesOwnerIDs)
	}

	foreign := doForm(t, e, "/sales/"+p.SalesID, url.Values{
		"name":           {"공동담당"},
		"sales_owner_id": {u.bUser.UserID},
	})
	if foreign.Code != 200 || !strings.Contains(foreign.Body.String(), "소속 부서") {
		t.Fatalf("타 조직 영업담당 거절 실패 %d %s", foreign.Code, foreign.Body.String())
	}

	wp := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "회계검증", Status: model.WBProjectActive, ProjectKind: model.ProjectKindBuild, PlanYear: 2026}
	pr := repository.NewProjectRepo(db)
	if err := pr.Create(wp); err != nil {
		t.Fatal(err)
	}
	err = pr.ReplaceAssignments(wp.ProjectID, u.hjlee.UserID, []model.ProjectAssignment{
		{UserID: u.bUser.UserID, Duty: model.ProjectDutySales},
	})
	if err == nil {
		t.Fatal("타 조직 수행·영업 담당 우회가 통과했다")
	}
	err = pr.ReplaceAssignments(wp.ProjectID, u.hjlee.UserID, []model.ProjectAssignment{
		{UserID: u.bUser.UserID, Duty: model.ProjectDutyAccounting},
	})
	if err != nil {
		t.Fatalf("회계 담당 전 조직 허용이어야 한다: %v", err)
	}
}

func Test95_88DefaultPlanYear(t *testing.T) {
	s := &model.SalesProject{ExpectedYM: "2026-09"}
	wp := model.WorkProjectFromSales(s)
	if wp.PlanYear != 2026 {
		t.Fatalf("expected ym year=%d", wp.PlanYear)
	}
	empty := model.WorkProjectFromSales(&model.SalesProject{})
	if empty.PlanYear != time.Now().Year() {
		t.Fatalf("올해 기본값=%d", empty.PlanYear)
	}
}

func Test95_88FormUILint(t *testing.T) {
	root := findTemplateRoot(t)
	for _, rel := range []string{
		filepath.Join("sales", "form.html"),
		filepath.Join("_project_year_dates.html"),
	} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		got := countUILint(string(b))
		if !got.zero() {
			t.Fatalf("%s ui_lint %+v", rel, got)
		}
	}
}
