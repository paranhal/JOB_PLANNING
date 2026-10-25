package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_48_CurrentMonthCollapsePopupAndAuth(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	h.Maintenance.repo.Now = func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local) }
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-seosan','서산고등학교','서산고등학교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	p, err := h.Maintenance.repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Maintenance.repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-12", CustomerID: "c-seosan", ProductType: "KLAS", Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Maintenance.repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-13", CustomerID: "c-seosan",
	}); err != nil {
		t.Fatal(err)
	}
	accessPath := filepath.Join(t.TempDir(), "access.db")
	if err := auditlog.Init(accessPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auditlog.Close)

	preview := mntGet(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=10")
	if preview.Code != http.StatusOK {
		t.Fatalf("preview %d", preview.Code)
	}
	body := preview.Body.String()
	if strings.Contains(body, "기간 보호") {
		t.Fatal("중복 정리에 기간 보호 문구가 있다")
	}
	if !strings.Contains(body, "삭제 예정") {
		t.Fatal("삭제 예정 없음")
	}
	csrfCk, token := csrfFromRec(t, preview)

	clPrev := mntGetCookies(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=10", jwtCookieRole(t, model.RoleCLevel))
	if !strings.Contains(clPrev.Body.String(), "disabled") {
		t.Fatal("c_level 에 정리 버튼이 활성")
	}
	cl := mntPostCookie(t, e, "/maintenance/"+p.PlanID+"/duplicates", url.Values{
		"confirm": {"1"}, "month": {"10"}, "csrf": {token},
	}, jwtCookieRole(t, model.RoleCLevel), csrfCk)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("c_level status=%d", cl.Code)
	}

	techPrev := mntGet(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=10")
	csrfCk, token = csrfFromRec(t, techPrev)
	ok := mntPostCookie(t, e, "/maintenance/"+p.PlanID+"/duplicates", url.Values{
		"confirm": {"1"}, "month": {"10"}, "view": {"list"}, "csrf": {token},
	}, jwtCookieRole(t, model.RoleOrgAdmin), csrfCk)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("tech 정리 %d loc=%s", ok.Code, ok.Header().Get("Location"))
	}
	if !strings.Contains(ok.Header().Get("Location"), "month=10") {
		t.Fatalf("월 유지 안 됨 %s", ok.Header().Get("Location"))
	}
	left, _ := h.Maintenance.repo.ListVisitsByMonth(p.PlanID, 2026, 10)
	if len(left) != 1 || left[0].VisitDate != "2026-10-12" {
		t.Fatalf("정리 후 %+v", left)
	}

	var flash *http.Cookie
	for _, c := range ok.Result().Cookies() {
		if c.Name == flashMntDupCookie && c.Value != "" {
			cp := *c
			flash = &cp
		}
	}
	if flash == nil {
		t.Fatal("결과 플래시 쿠키 없음")
	}
	planRec := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost"+ok.Header().Get("Location"), nil)
	preq.AddCookie(jwtCookie(t))
	preq.AddCookie(flash)
	e.ServeHTTP(planRec, preq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("계획 status=%d", planRec.Code)
	}
	if !strings.Contains(planRec.Body.String(), "중복 1건을 정리했습니다") {
		t.Fatalf("팝업 없음: %s", planRec.Body.String())
	}
	again := mntGet(t, e, "/maintenance/"+p.PlanID+"?month=10")
	if strings.Contains(again.Body.String(), "중복 1건을 정리했습니다") {
		t.Fatal("새로고침에 팝업이 다시 나옴")
	}

	orgs := repository.NewOrgRepo(db)
	other, err := orgs.Create("미디어", "미디어", "", "t")
	if err != nil {
		t.Fatal(err)
	}
	foreign := mntGetCookies(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=10", jwtCookieOrg(t, model.RoleTech, other.OrgID))
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("타 조직 status=%d", foreign.Code)
	}

	visits, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	del := mntPost(t, e, "/maintenance/visits/"+visits[0].VisitID+"/delete?plan_id="+p.PlanID, url.Values{})
	if del.Code != http.StatusSeeOther {
		t.Fatalf("일반 삭제 status=%d", del.Code)
	}
	if got, _ := h.Maintenance.repo.ListVisits(p.PlanID); len(got) != 1 {
		t.Fatalf("일반 삭제로 Keep이 지워짐 %d", len(got))
	}
}

func mntGetCookies(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_48_ZeroDeletableDisablesButton(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10", "2026-11-14")
	visits, _ := h.Maintenance.repo.ListVisits(p.PlanID)
	for _, v := range visits {
		if err := h.Maintenance.repo.SetVisitCompleted(v.VisitID, true, v.VisitDate); err != nil {
			t.Fatal(err)
		}
	}
	preview := mntGet(t, e, "/maintenance/"+p.PlanID+"/duplicates?month=11")
	body := preview.Body.String()
	if !strings.Contains(body, "제외 — 둘 다 완료") && !strings.Contains(body, "제외 — 완료된 방문") {
		t.Fatalf("완료 제외 문구 없음: %s", body)
	}
	if !strings.Contains(body, "disabled") {
		t.Fatal("0건인데 버튼이 활성")
	}
	if strings.Contains(body, "기간 보호") {
		t.Fatal("기간 보호 문구")
	}
}

func Test95_48_DuplicatePageStatusesInTemplate(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(findStaticRoot(t), "..", "templates", "maintenance", "plan_duplicates.html"))
	if err != nil {
		b, err = os.ReadFile(filepath.Join("web", "templates", "maintenance", "plan_duplicates.html"))
		if err != nil {
			t.Fatal(err)
		}
	}
	s := string(b)
	for _, w := range []string{"StatusLabel", "DeletableCount", "disabled", "csrf"} {
		if !strings.Contains(s, w) {
			t.Fatalf("%q 없음", w)
		}
	}
	if strings.Contains(s, "기간 보호") {
		t.Fatal("기간 보호 문구가 템플릿에 남음")
	}
}
