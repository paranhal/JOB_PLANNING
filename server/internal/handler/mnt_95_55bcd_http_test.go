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
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func Test95_55SpreadWorkdaysAndSlotDates(t *testing.T) {
	days := workdaysInclusive("2026-10-01", "2026-10-14", true, map[string]bool{"2026-10-09": true})
	for _, d := range days {
		tm, _ := time.Parse("2006-01-02", d)
		if tm.Weekday() == time.Saturday || tm.Weekday() == time.Sunday {
			t.Fatalf("주말 %s", d)
		}
		if d == "2026-10-09" {
			t.Fatal("공휴일")
		}
	}
	slots := []model.UnassignedMonthSlot{
		{CustomerID: "a", ShortName: "가", ProductType: "x"},
		{CustomerID: "b", ShortName: "나", ProductType: "x"},
		{CustomerID: "c", ShortName: "다", ProductType: "x"},
	}
	got, err := spreadSlotDates(slots, days[:2], 2)
	if err != nil {
		t.Fatal(err)
	}
	if got["a|x"] != days[0] || got["b|x"] != days[0] || got["c|x"] != days[1] {
		t.Fatalf("%v days=%v", got, days[:2])
	}
	if _, err := spreadSlotDates(slots, days[:1], 2); err == nil {
		t.Fatal("범위 부족이어야 함")
	}
}

func Test95_55BBulkAssignSameDateAndSpread(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	rec := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage?um=10", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("manage=%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "assign-slots/bulk") || !strings.Contains(body, "차년도 계획 수립") {
		t.Fatal("일괄 배정·차년도 계획 수립 없음")
	}

	slots, err := repo.ListOpenUnassignedSlotsForOrg(p.PlanID, 2026, 10, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"month": {"10"}, "mode": {"same_date"}, "visit_date": {"2026-10-15"}, "assignee": {"최혜영"}}
	nSlot := 0
	for _, s := range slots {
		if s.CustomerID == "c1" {
			form.Add("slot_key", slotKeyOf(s))
			nSlot++
		}
	}
	if nSlot < 1 {
		t.Fatalf("슬롯 %d %+v", nSlot, slots)
	}
	prev := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/preview", form, jwtCookie(t))
	if prev.Code != http.StatusOK {
		t.Fatalf("preview=%d %s", prev.Code, prev.Body.String())
	}
	var preview bulkAssignPreview
	if err := json.Unmarshal(prev.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.OK || preview.Count < 1 {
		t.Fatalf("preview %+v", preview)
	}
	cl := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/bulk", form, jwtCookieRole(t, "c_level"))
	if cl.Code != http.StatusForbidden {
		t.Fatalf("c_level bulk=%d", cl.Code)
	}
	ok := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/bulk", form, jwtCookie(t))
	if ok.Code != http.StatusOK && ok.Code != http.StatusSeeOther {
		t.Fatalf("bulk=%d %s", ok.Code, ok.Body.String())
	}
	visits, _ := repo.ListVisits(p.PlanID)
	n := 0
	for _, v := range visits {
		if v.CustomerID == "c1" && strings.HasPrefix(v.VisitDate, "2026-10-15") {
			n++
		}
	}
	if n < 1 {
		t.Fatalf("배정 %d loc=%s", n, ok.Header().Get("Location"))
	}
}

func Test95_55CMntCandidatesAndQuickRegisterVisitDate(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "t55c.db"))
	ck := jwtCookieUser(t, me)
	miss := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/mnt-quick-register", strings.NewReader(`{"customer_id":"sch1","month":2,"assignee":"김현우"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(ck)
	e.ServeHTTP(miss, req)
	if miss.Code == http.StatusOK {
		t.Fatal("방문일 없이 통과")
	}

	body := doGetWith(t, e, "/workboard/new-work/form?kind=maintenance&date=2026-02-10", ck).Body.String()
	if !strings.Contains(body, "방문 예정일") || strings.Contains(body, "방문 예정일 (선택)") {
		t.Fatal("방문일 필수 표시 없음")
	}
	if !strings.Contains(body, "wb_multi_site_mode") && !strings.Contains(body, "multi_site_mode") {
		t.Fatal("다중 사이트 라디오 없음")
	}
	js := doGetWith(t, e, "/workboard/mnt-candidates?q=", ck)
	if js.Code != http.StatusOK {
		t.Fatalf("candidates=%d %s", js.Code, js.Body.String())
	}
}

func Test95_55DManageEmptyAndStickyAndYearPlan(t *testing.T) {
	e, _, p := newMntManageApp(t)
	rec := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookie(t))
	body := rec.Body.String()
	for _, w := range []string{"mt-sticky", "mt-cancel-sites", "mt-move-sites", "차년도 계획 수립", "새 연도 빈 계획 만들기"} {
		if !strings.Contains(body, w) {
			t.Fatalf("%q 없음", w)
		}
	}
	html, _ := os.ReadFile(filepath.Join(findTemplateRoot(t), "maintenance", "plan_manage.html"))
	s := string(html)
	if strings.Count(s, `id="mt-all"`) != 1 {
		t.Fatalf("전체 선택 체크 %d", strings.Count(s, `id="mt-all"`))
	}
	if !strings.Contains(s, "mnt-flash-ok-popup") {
		t.Fatal("결과 팝업 템플릿 없음")
	}
}

func Test95_55JSShowsBulkError(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "..", "templates", "maintenance", "plan_manage.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, w := range []string{"ua-err", "scrollIntoView", "res.j.error", "assign-slots/bulk"} {
		if !strings.Contains(s, w) {
			t.Fatalf("JS %q 없음", w)
		}
	}
	js, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "mnt-quick-register.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "방문일을 입력하세요") || !strings.Contains(string(js), "mnt-candidates") {
		t.Fatal("간이등록 JS 없음")
	}
}
