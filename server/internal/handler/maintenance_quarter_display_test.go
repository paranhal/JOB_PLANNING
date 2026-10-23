package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"customer-support/internal/model"
)

func TestShowPlanQuarterDisplayHTMLByMonth(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c_q1','분기관','분기관',1,'O01'),('c_m1','월간관','월간관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	repo := h.Maintenance.repo
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c_q1", ShortName: "분기관", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c_m1", ShortName: "월간관", HasKlas: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}

	early := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list&month=1")
	if early.Code != http.StatusOK {
		t.Fatalf("1월 status=%d", early.Code)
	}
	b1 := early.Body.String()
	if !strings.Contains(b1, "분기 점검 대기") || !strings.Contains(b1, "분기관") {
		t.Fatalf("1월 안내/목록 없음: %s", b1)
	}
	if strings.Contains(b1, "분기 점검 확인") || strings.Contains(b1, "이미 점검함") || strings.Contains(b1, "건너뛰기") {
		t.Fatal("1월에 확인 3입력이 보이면 안 됨")
	}
	if strings.Contains(b1, "아직 배정되지 않았습니다 —") {
		t.Fatal("1월에 붉은 배너가 있으면 안 됨")
	}
	if strings.Contains(b1, ">분기 점검 미확정<") {
		t.Fatal("1월 미배정에 분기 미확정")
	}
	if !strings.Contains(b1, "월간관") {
		t.Fatal("1월 월별 미배정이 빠짐")
	}

	feb := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list&month=2")
	b2 := feb.Body.String()
	if !strings.Contains(b2, "분기 점검 대기") || strings.Contains(b2, "이미 점검함") {
		t.Fatal("2월도 보여주기만")
	}

	last := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=list&month=3")
	b3 := last.Body.String()
	if !strings.Contains(b3, "분기 점검 확인") {
		t.Fatal("3월 확인 박스 없음")
	}
	if !strings.Contains(b3, "이미 점검함") || !strings.Contains(b3, "건너뛰기") {
		t.Fatal("3월 세 입력이 없음")
	}
	if !strings.Contains(b3, "1분기 점검이 아직 배정되지 않았습니다 — 3월 말까지") {
		t.Fatalf("3월 붉은 배너 없음: %s", b3)
	}
	if !strings.Contains(b3, ">분기 점검 미확정<") {
		t.Fatal("3월 미배정에 분기 미확정이 없음")
	}
}

func TestQuarterConfirmReadonlyAndViewAsAndOtherOrg(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c_home','우리관','우리관',1,'O01'),('c_away','다른관','다른관',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	repo := h.Maintenance.repo
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c_home", ShortName: "우리관", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c_away", ShortName: "다른관", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}

	homeCookie := jwtCookieOrg(t, model.RoleOrgAdmin, "O01")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/maintenance/"+p.PlanID+"?view=list&month=3", nil)
	req.AddCookie(homeCookie)
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	// 95-2E 고객은 전사 공용 마스터 → 방문 추가 콤보에 「다른관」이 나올 수 있다.
	// 95-16A는 대기 목록을 접어도 슬롯 행 HTML은 남긴다. 조직 격리는 대기 슬롯 data-name과 skip API로 본다.
	if !strings.Contains(body, `data-name="우리관"`) {
		t.Fatal("자기 조직 분기 사이트가 안 보임")
	}
	if strings.Contains(body, `data-name="다른관"`) {
		t.Fatal("다른 조직 사이트가 대기 목록에 노출됨")
	}

	awaySkip := mntPostWithCookie(t, e, "/maintenance/"+p.PlanID+"/quarter-confirm", url.Values{
		"action": {"skip"}, "customer_id": {"c_away"}, "product_type": {"KLAS"},
		"quarter": {"1"}, "reason": {"다른 조직"}, "month": {"3"}, "view": {"list"},
	}, homeCookie)
	if awaySkip.Code != http.StatusForbidden {
		t.Fatalf("다른 조직 skip=%d loc=%s", awaySkip.Code, locText(awaySkip))
	}

	ro := mntPostWithCookie(t, e, "/maintenance/"+p.PlanID+"/quarter-confirm", url.Values{
		"action": {"skip"}, "customer_id": {"c_home"}, "product_type": {"KLAS"},
		"quarter": {"1"}, "reason": {"읽기전용"}, "month": {"3"}, "view": {"list"},
	}, jwtCookieReadOnly(t, model.RoleTech))
	if ro.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 skip=%d", ro.Code)
	}

	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id)
		VALUES ('u-view','viewtech','x','시점기술','tech',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	viewReq := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/"+p.PlanID+"/quarter-confirm", strings.NewReader(url.Values{
		"action": {"skip"}, "customer_id": {"c_home"}, "product_type": {"KLAS"},
		"quarter": {"1"}, "reason": {"시점보기"}, "month": {"3"}, "view": {"list"},
	}.Encode()))
	preq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	preq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O01"))
	preq.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: "u-view", Path: "/"})
	e.ServeHTTP(viewReq, preq)
	if viewReq.Code != http.StatusForbidden {
		t.Fatalf("view-as skip=%d", viewReq.Code)
	}
}

func jwtCookieOrg(t *testing.T, role, orgID string) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": role + "-org-id", "username": role, "role": role,
		"name": role, "org_id": orgID, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}
