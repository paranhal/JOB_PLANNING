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

func TestMnt95_45ManagePanelMarkupAndAuth(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "가나", Region: "세종", InspectionCycle: "semi",
	}); err != nil {
		t.Fatal(err)
	}
	rec := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, w := range []string{"월 점검 대상 등록", "월 선택", "지역 단위", "사이트 검색", "이 달로 등록", "mt-result"} {
		if !strings.Contains(body, w) {
			t.Fatalf("%q 없음", w)
		}
	}
	tech := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookieRole(t, "tech"))
	if tech.Code != http.StatusOK {
		t.Fatalf("tech status=%d", tech.Code)
	}
	if !strings.Contains(tech.Body.String(), "월 점검 대상 등록") {
		t.Fatal("기술담당 패널 없음")
	}
	cl := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"}, "customer_id": {"c1"},
	}, jwtCookieRole(t, "c_level"))
	if cl.Code != http.StatusForbidden {
		t.Fatalf("c_level 등록 status=%d", cl.Code)
	}
	ok := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
		"month": {"2"}, "source": {"site"}, "customer_id": {"c1"},
	}, jwtCookie(t))
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d loc=%s", ok.Code, ok.Header().Get("Location"))
	}
	js := manageGet(t, e, "/maintenance/"+p.PlanID+"/month-targets?month=2", jwtCookie(t))
	if js.Code != http.StatusOK {
		t.Fatalf("json status=%d %s", js.Code, js.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["regions"]; !ok {
		t.Fatal("regions 없음")
	}
}

func TestMnt95_45SiteFormCycleHint(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "..", "templates", "maintenance", "site_form.html"))
	if err != nil {
		root := filepath.Join("..", "web", "templates", "maintenance", "site_form.html")
		b, err = os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(string(b), "계획 관리에서 점검할 달") {
		t.Fatal("주기 안내 없음")
	}
}

func TestMnt95_45WorkboardModalButtons(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "t45-wb.db"))
	ck := jwtCookieUser(t, me)
	body := doGetWith(t, e, "/workboard/new-work/form?kind=maintenance&date=2026-02-10", ck).Body.String()
	if !strings.Contains(body, "정기점검 등록") {
		t.Fatal("정기점검 등록 버튼 없음")
	}
	if !strings.Contains(body, "mnt-quick-save") {
		t.Fatal("간이 등록 폼 없음")
	}
	as := doGetWith(t, e, "/workboard/new-work/form?kind=as&date=2026-02-10", ck).Body.String()
	if !strings.Contains(as, "AS 접수 추가") {
		t.Fatal("AS 빈 목록 버튼 없음")
	}
	jsPath := filepath.Join(findStaticRoot(t), "js", "mnt-quick-register.js")
	b, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if !strings.Contains(js, "/workboard/mnt-quick-register") || !strings.Contains(js, "bindMntQuickRegister") {
		t.Fatal("퀵 등록 스크립트 없음")
	}
}

func TestMnt95_45QuickRegisterFlow(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "t45-q.db"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('sch1','학교1','학교1',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	if err := mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "sch1", ShortName: "학교1", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/mnt-quick-register", strings.NewReader(`{"customer_id":"sch1","month":2,"assignee":"김현우"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "visit_id") {
		t.Fatalf("visit_id 없음 %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "반기") {
		t.Fatalf("주기 안내 없음 %s", rec.Body.String())
	}
}
