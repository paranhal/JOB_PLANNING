package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_59_CheonanRegionCheckCauseAndFilter(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	db := repo.DB()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES
		('cn1','성환도서관','성환도서관',1,?,'충청남도','천안시 동남구'),
		('cn2','쌍용도서관','쌍용도서관',1,?,'충청남도','천안시 서북구'),
		('h59','홍성도서관','홍성도서관',1,?,'충청남도','홍성군')`, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	for id, name := range map[string]string{"cn1": "성환", "cn2": "쌍용", "h59": "홍성"} {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: name, Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary, HasKlas: true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	html, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "maintenance", "plan_manage.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(html)
	if !strings.Contains(src, "&region=") {
		t.Fatal("원인 유지: load() URL 에 region 파라미터가 없음 (천안시 체크해도 목록이 안 바뀜)")
	}
	idxSum := strings.Index(src, "updateRegionSummary();")
	idxLoad := strings.Index(src[idxSum+1:], "load();")
	if idxSum < 0 || idxLoad < 0 || idxLoad > 400 {
		t.Fatal("원인 유지: 지역 칩 변경 후 load()를 부르지 않음")
	}

	all := manageGet(t, e, "/maintenance/"+p.PlanID+"/month-targets?month=2", jwtCookie(t))
	if all.Code != http.StatusOK {
		t.Fatalf("all %d %s", all.Code, all.Body.String())
	}
	if !strings.Contains(all.Body.String(), "성환") || !strings.Contains(all.Body.String(), "홍성") {
		t.Fatalf("전체 목록 %s", all.Body.String())
	}
	filtered := manageGet(t, e, "/maintenance/"+p.PlanID+"/month-targets?month=2&region="+url.QueryEscape("충남|천안시 동남구"), jwtCookie(t))
	if filtered.Code != http.StatusOK {
		t.Fatalf("region %d %s", filtered.Code, filtered.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(filtered.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var sites []repository.MonthTargetSiteRow
	if err := json.Unmarshal(payload["sites"], &sites); err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].CustomerID != "cn1" {
		t.Fatalf("천안시 동남구 사이트 %+v", sites)
	}

	page := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookie(t))
	body := page.Body.String()
	if strings.Contains(body, "이 달로 등록") {
		t.Fatal("이 달로 등록이 남아 있음")
	}
	if !strings.Contains(body, `id="mt-reg-sites"`) || !strings.Contains(body, ">확인<") {
		t.Fatal("확인 버튼 없음")
	}
	if !strings.Contains(body, `id="mt-confirm"`) {
		t.Fatal("확인 팝업 없음")
	}
	if strings.Contains(body, `id="ua-apply"`) || strings.Contains(body, "날짜·담당자 일괄 배정") {
		t.Fatal("대기 목록 일괄 배정 UI")
	}
	if !strings.Contains(body, "data-unassigned-list") {
		t.Fatal("대기 목록 없음")
	}
	if !strings.Contains(body, "조회 전용") {
		t.Fatal("대기 목록 조회 전용 안내 없음")
	}
	_ = db
}

func Test95_59_ConfirmSavesCheckedSitesOnly(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	db := repo.DB()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES
		('cn1','성환도서관','성환도서관',1,?,'충청남도','천안시 동남구'),
		('h59','홍성도서관','홍성도서관',1,?,'충청남도','홍성군')`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cn1", "h59"} {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary, HasKlas: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	empty := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"},
	}, jwtCookie(t))
	if empty.Code != http.StatusSeeOther && empty.Code != http.StatusBadRequest {
		t.Fatalf("빈 등록 %d %s", empty.Code, empty.Body.String())
	}

	ok := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"}, "customer_id": {"cn1"},
	}, jwtCookie(t))
	if ok.Code != http.StatusSeeOther && ok.Code != http.StatusOK {
		t.Fatalf("확인 저장 %d %s", ok.Code, ok.Body.String())
	}
	js := manageGet(t, e, "/maintenance/"+p.PlanID+"/month-targets?month=2", jwtCookie(t))
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var sites []repository.MonthTargetSiteRow
	if err := json.Unmarshal(payload["sites"], &sites); err != nil {
		t.Fatal(err)
	}
	var cnReg, hReg bool
	for _, s := range sites {
		if s.CustomerID == "cn1" {
			cnReg = s.Registered
		}
		if s.CustomerID == "h59" {
			hReg = s.Registered
		}
	}
	if !cnReg {
		t.Fatal("체크한 천안 사이트가 등록되지 않음")
	}
	if hReg {
		t.Fatal("체크하지 않은 홍성이 등록됨")
	}

	sales := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"}, "customer_id": {"h59"},
	}, jwtCookieRole(t, model.RoleSales))
	if sales.Code != http.StatusForbidden {
		t.Fatalf("영업 등록 %d", sales.Code)
	}
}

func Test95_59_AssignPermissionTechVsSales(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	db := repo.DB()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES ('cn1','성환도서관','성환도서관',1,?,'충청남도','천안시 동남구')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cn1", ShortName: "성환", Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary, HasKlas: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterMonthTargets(p, 10, model.MonthTargetSourceSite, "", []string{"cn1"}, "admin-id", "관리자"); err != nil {
		t.Fatal(err)
	}

	techCal := manageGet(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", jwtCookieRole(t, model.RoleTech))
	if techCal.Code != http.StatusOK {
		t.Fatalf("기술 일정 %d %s", techCal.Code, techCal.Body.String())
	}
	if !strings.Contains(techCal.Body.String(), "방문일 배정") && !strings.Contains(techCal.Body.String(), ">배정<") {
		t.Fatalf("기술 계정 미배정 배정 UI 없음")
	}
	salesCal := manageGet(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", jwtCookieRole(t, model.RoleSales))
	if salesCal.Code != http.StatusOK {
		t.Fatalf("영업 일정 %d", salesCal.Code)
	}
	if strings.Contains(salesCal.Body.String(), "방문일 배정") {
		t.Fatal("영업 계정에 배정 열이 보임")
	}

	slots, err := repo.ListOpenUnassignedSlotsForOrg(p.PlanID, 2026, 10, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"month": {"10"}, "mode": {"same_date"}, "visit_date": {"2026-10-15"}, "assignee": {"최혜영"}}
	for _, s := range slots {
		if s.CustomerID == "cn1" {
			form.Add("slot_key", slotKeyOf(s))
		}
	}
	if len(form["slot_key"]) == 0 {
		t.Fatalf("슬롯 없음 %+v", slots)
	}
	deny := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/bulk", form, jwtCookieRole(t, model.RoleSales))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("영업 bulk %d", deny.Code)
	}
	ok := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/bulk", form, jwtCookieRole(t, model.RoleTech))
	if ok.Code != http.StatusOK && ok.Code != http.StatusSeeOther {
		t.Fatalf("기술 bulk %d %s", ok.Code, ok.Body.String())
	}

	oa := manageGet(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", jwtCookie(t))
	if oa.Code != http.StatusOK {
		t.Fatalf("조직관리자 일정 %d", oa.Code)
	}
	if !strings.Contains(oa.Body.String(), "방문일 배정") && !strings.Contains(oa.Body.String(), "배정 관리") {
		t.Fatal("조직관리자 배정 UI 없음")
	}
}
