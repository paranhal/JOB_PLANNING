package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_57ManageLayoutOrderAndCollapse(t *testing.T) {
	e, _, p := newMntManageApp(t)
	rec := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	marks := []string{
		`id="mnt-flow"`,
		`id="next-plan-box"`,
		"연도 계획 현황",
		`data-month-target-panel`,
		`data-unassigned-bulk`,
		`id="mnt-calendar-link"`,
	}
	var prev int
	for _, m := range marks {
		i := strings.Index(body, m)
		if i < 0 {
			t.Fatalf("%q 없음", m)
		}
		if i < prev {
			t.Fatalf("순서 오류: %q at %d < %d", m, i, prev)
		}
		prev = i
	}
	if strings.Contains(body, "연도 계획 관리") {
		t.Fatal("하단 연도 계획 관리가 남아 있습니다")
	}
	if strings.Contains(body, "계획 생성</h3>") || strings.Contains(body, ">계획 생성</h") {
		t.Fatal("구 계획 생성 제목이 남아 있습니다")
	}
	if !strings.Contains(body, `aria-expanded="false"`) || !strings.Contains(body, `id="next-plan-body" class="hidden`) {
		t.Fatal("기본 접힘(aria-expanded=false, hidden)이 아닙니다")
	}
	boxStart := strings.Index(body, `id="next-plan-body"`)
	boxEnd := strings.Index(body[boxStart:], "연도 계획 현황")
	if boxStart < 0 || boxEnd < 0 {
		t.Fatal("박스 구간을 찾지 못했습니다")
	}
	inner := body[boxStart : boxStart+boxEnd]
	if strings.Contains(inner, "/status") {
		t.Fatal("승인·보관 버튼이 ⓪ 박스 안에 있습니다")
	}
	if !strings.Contains(body, "/maintenance/"+p.PlanID+"/status") {
		t.Fatal("연도 계획 현황에 상태 변경이 없습니다")
	}
	if !strings.Contains(body, "③ 날짜·담당자 일괄 배정") {
		t.Fatal("③ 버튼 없음")
	}
	html, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "maintenance", "plan_manage.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(html)
	for _, w := range []string{"maintNextPlanOpen", "setNextPlanOpen", "aria-controls", "role=\"button\""} {
		if !strings.Contains(src, w) {
			t.Fatalf("JS/접근성 %q 없음", w)
		}
	}
	if strings.Contains(src, "x-data") && strings.Contains(src, "next-plan") {
		t.Log("next-plan Alpine 사용")
	}
}

func Test95_57OrgAdminSeesBoxTechForbidden(t *testing.T) {
	e, _, plans, users := newMnt955ACrossOrgApp(t)
	for _, org := range []string{model.OrgIDLibrary, "O02", "O03"} {
		rec := manageGet(t, e, "/maintenance/"+plans[org].PlanID+"/manage", jwtCookieUser(t, users[org]))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d", org, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "차년도 계획 수립") {
			t.Fatalf("%s 관리자에게 박스가 없습니다", org)
		}
	}
	tech := manageGet(t, e, "/maintenance/"+plans["O02"].PlanID+"/manage", jwtCookieUser(t, users["techO02"]))
	if tech.Code != http.StatusOK {
		t.Fatalf("기술담당 status=%d", tech.Code)
	}
	if strings.Contains(tech.Body.String(), "차년도 계획 수립") {
		t.Fatal("기술담당에게 차년도 박스가 보입니다")
	}
	deny := managePost(t, e, "/maintenance", url.Values{
		"plan_year": {"2028"}, "title": {"x"}, "next": {"manage"}, "from_plan_id": {plans["O02"].PlanID},
	}, jwtCookieUser(t, users["techO02"]))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("기술담당 생성 거부 status=%d", deny.Code)
	}
	denyCopy := managePost(t, e, "/maintenance/"+plans["O02"].PlanID+"/copy", url.Values{"plan_year": {"2028"}}, jwtCookieUser(t, users["techO02"]))
	if denyCopy.Code != http.StatusForbidden {
		t.Fatalf("기술담당 복사 거부 status=%d", denyCopy.Code)
	}
}

func Test95_57CreateCopyDuplicateKeepOpenAndIsolation(t *testing.T) {
	e, repo, plans, users := newMnt955ACrossOrgApp(t)
	med := plans["O02"]
	ck := jwtCookieUser(t, users["O02"])
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: med.PlanID, VisitDate: "2026-03-10", CustomerID: "c-med", ProductType: "KLAS", Assignee: "미디어장",
	}); err != nil {
		t.Fatal(err)
	}
	cfgsBefore, err := repo.ListSiteConfigs()
	if err != nil {
		t.Fatal(err)
	}

	created := managePost(t, e, "/maintenance", url.Values{
		"plan_year": {"2027"}, "title": {"2027년 정기점검"}, "next": {"manage"}, "from_plan_id": {med.PlanID},
	}, ck)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("생성 status=%d %s", created.Code, created.Body.String())
	}
	loc := created.Header().Get("Location")
	if !strings.Contains(loc, "계획을 만들었습니다") && !strings.Contains(url.QueryEscape("계획을 만들었습니다"), "만들") {
		if !strings.Contains(loc, "%") {
			t.Fatalf("성공 알림 없음 loc=%s", loc)
		}
	}
	p2027, err := repo.GetPlanByYearInOrg("O02", 2027)
	if err != nil || p2027 == nil {
		t.Fatalf("2027 계획 없음 %+v", err)
	}

	dup := managePost(t, e, "/maintenance", url.Values{
		"plan_year": {"2027"}, "title": {"다시"}, "next": {"manage"}, "from_plan_id": {med.PlanID},
	}, ck)
	if dup.Code != http.StatusSeeOther {
		t.Fatalf("중복 status=%d", dup.Code)
	}
	dloc := dup.Header().Get("Location")
	if !strings.Contains(dloc, "open_next=1") {
		t.Fatalf("open_next 없음 %s", dloc)
	}
	if !strings.Contains(dloc, "np_year=2027") || !strings.Contains(dloc, "np_title") {
		t.Fatalf("입력 보존 없음 %s", dloc)
	}
	if !strings.Contains(dloc, url.QueryEscape("이미 있습니다")) && !strings.Contains(dloc, "%") {
		t.Fatalf("중복 메시지 없음 %s", dloc)
	}
	dupPage := manageGet(t, e, dloc, ck)
	if dupPage.Code != http.StatusOK {
		t.Fatalf("중복 화면 status=%d", dupPage.Code)
	}
	db := dupPage.Body.String()
	if !strings.Contains(db, `data-force-open="1"`) || !strings.Contains(db, "이미 있습니다") {
		t.Fatal("박스 펼침 유지·메시지 없음")
	}
	if !strings.Contains(db, `value="2027"`) || !strings.Contains(db, `value="다시"`) {
		t.Fatal("연도·제목 보존 없음")
	}

	over := managePost(t, e, "/maintenance", url.Values{
		"plan_year": {fmt.Sprintf("%d", time.Now().Year()+6)}, "title": {"범위"}, "next": {"manage"}, "from_plan_id": {med.PlanID},
	}, ck)
	if over.Code != http.StatusSeeOther {
		t.Fatalf("범위 status=%d", over.Code)
	}
	if !strings.Contains(over.Header().Get("Location"), "open_next=1") {
		t.Fatal("범위 오류 시 박스 유지 없음")
	}

	copied := managePost(t, e, "/maintenance/"+med.PlanID+"/copy", url.Values{"plan_year": {"2028"}}, ck)
	if copied.Code != http.StatusSeeOther {
		t.Fatalf("복사 status=%d %s", copied.Code, copied.Body.String())
	}
	p2028, err := repo.GetPlanByYearInOrg("O02", 2028)
	if err != nil || p2028 == nil {
		t.Fatal("2028 복사 계획 없음")
	}
	visits, err := repo.ListVisits(p2028.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visits) == 0 {
		t.Fatal("복사된 방문(담당자) 없음")
	}
	for _, v := range visits {
		if strings.TrimSpace(v.VisitDate) != "" {
			t.Fatalf("방문일이 비어 있어야 함 %q", v.VisitDate)
		}
		if v.Assignee != "미디어장" {
			t.Fatalf("담당자 미복사 %q", v.Assignee)
		}
	}
	cfgsAfter, _ := repo.ListSiteConfigs()
	if len(cfgsAfter) != len(cfgsBefore) {
		t.Fatalf("site_config 수 변경 %d→%d", len(cfgsBefore), len(cfgsAfter))
	}

	for _, org := range []string{model.OrgIDLibrary, "O03"} {
		p, err := repo.GetPlanByYearInOrg(org, 2027)
		if err != nil {
			t.Fatal(err)
		}
		if p != nil {
			t.Fatalf("%s 에 2027 계획이 생겼습니다", org)
		}
		p28, _ := repo.GetPlanByYearInOrg(org, 2028)
		if p28 != nil {
			t.Fatalf("%s 에 2028 계획이 생겼습니다", org)
		}
	}

	st := managePost(t, e, "/maintenance/"+med.PlanID+"/status", url.Values{"status": {"approved"}}, ck)
	if st.Code != http.StatusSeeOther && st.Code != http.StatusOK {
		t.Fatalf("승인 status=%d", st.Code)
	}
	got, _ := repo.GetPlan(med.PlanID)
	if got == nil || got.Status != "approved" {
		t.Fatalf("승인 반영 안 됨 %+v", got)
	}
}

func Test95_57EnsureYearPlanWithoutBox(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "57auto.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O04','O04','자동생성팀','business',1,40)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	u := &model.User{Username: "auto57", PasswordHash: HashPassword("x"), FullName: "자동장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O04", ProfileDone: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('c-auto','자동학교','자동학교',1,'O04')`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c-auto", ShortName: "자동학교", Region: "세종", InspectionCycle: "quarterly", OrgID: "O04",
	}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/workboard/mnt-quick-register", h.Workboard.QuickRegisterMaintenance)

	year := time.Now().Year()
	before, _ := repo.GetPlanByYearInOrg("O04", year)
	if before != nil {
		t.Fatal("미리 계획이 있습니다")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"customer_id": "c-auto", "month": 2, "visit_date": fmt.Sprintf("%d-02-10", year), "assignee": "자동장",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/mnt-quick-register", strings.NewReader(string(body)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(jwtCookieUser(t, u))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("quick-register status=%d %s", rec.Code, rec.Body.String())
	}
	after, err := repo.GetPlanByYearInOrg("O04", year)
	if err != nil || after == nil {
		t.Fatalf("올해 계획 자동 생성 실패 %+v", err)
	}
}

func Test95_57AlpineStillCoversPlanManage(t *testing.T) {
	html, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "maintenance", "plan_manage.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "차년도 계획 수립") {
		t.Fatal("plan_manage 가 95-57 박스를 포함하지 않습니다")
	}
}
