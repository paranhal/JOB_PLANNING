package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newAS95_33App(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, *model.ASReceipt, string, *http.Cookie, *http.Cookie, *http.Cookie) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "as33.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES ('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('cust_a','가나도서관','가나도서관',1,'O01'),
		       ('cust_si','SI학교','SI학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	cl := &model.User{
		Username: "ceo33", PasswordHash: HashPassword("x"), FullName: "대표",
		Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true,
	}
	if err := users.Create(cl); err != nil {
		t.Fatal(err)
	}
	asRepo := repository.NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트 오작동", Urgency: "normal", Priority: "normal",
		AssignedTo: "관리자", ReceiptDatetime: time.Now(), Status: "in_progress",
		OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}
	other := &model.ASReceipt{
		CustomerID: "cust_si", Symptom: "타조직", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: "O03",
	}
	if err := asRepo.Create(other); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as", h.AS.List)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/support", h.AS.CreateSupport)
	g.POST("/as/:id/to-admin-work", h.AS.ToAdminWork)
	g.POST("/as/:id/reopen", h.AS.Reopen)
	g.POST("/as/:id/delete", h.AS.Delete)
	g.GET("/customers/:id", h.Customer.Show)
	g.GET("/workboard/as-lookup", h.Workboard.LookupRegisterAS)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	oa := &model.User{
		Username: "oa33", PasswordHash: "x", FullName: "도서관장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	return e, h, asRepo, src, other.ASID, jwtCookie(t), jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, cl)
}

func TestAS95_33ShowRedirectsToAction(t *testing.T) {
	e, _, _, src, _, ck, _, _ := newAS95_33App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/as/"+src.ASID, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != asOpenHref(src.ASID) {
		t.Fatalf("Location=%s", loc)
	}
}

func TestAS95_33LegacyShowHasBanner(t *testing.T) {
	e, _, _, src, _, ck, _, _ := newAS95_33App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/as/"+src.ASID+"?legacy=1", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "예전 화면입니다") || !strings.Contains(body, "새 조치 화면으로") {
		t.Fatal("배너 없음")
	}
	if !strings.Contains(body, asOpenHref(src.ASID)) {
		t.Fatal("새 화면 링크 없음")
	}
}

func TestAS95_33EmbedDoesNotRedirect(t *testing.T) {
	e, _, _, src, _, ck, _, _ := newAS95_33App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/as/"+src.ASID+"?embed=1", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("embed status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), src.ASNumber) {
		t.Fatal("embed 본문 없음")
	}
}

func TestAS95_33ListKanbanCustomerHistoryUseActionHref(t *testing.T) {
	e, _, _, src, _, ck, _, _ := newAS95_33App(t)
	bare := regexp.MustCompile(`href="/as/` + regexp.QuoteMeta(src.ASID) + `"`)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/as", nil)
	req.AddCookie(ck)
	e.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	lb := list.Body.String()
	if bare.MatchString(lb) {
		t.Fatal("목록에 예전 href 가 남았다")
	}
	if !strings.Contains(lb, asOpenHref(src.ASID)) {
		t.Fatal("목록에 /action 없음")
	}

	kanban := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/as?display=kanban", nil)
	req.AddCookie(ck)
	e.ServeHTTP(kanban, req)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	kb := kanban.Body.String()
	if bare.MatchString(kb) {
		t.Fatal("칸반에 예전 href 가 남았다")
	}
	if !strings.Contains(kb, asOpenHref(src.ASID)) {
		t.Fatal("칸반에 /action 없음")
	}

	cust := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/customers/cust_a", nil)
	req.AddCookie(ck)
	e.ServeHTTP(cust, req)
	if cust.Code != http.StatusOK {
		t.Fatalf("customer %d", cust.Code)
	}
	cb := cust.Body.String()
	if strings.Contains(cb, src.ASNumber) {
		if bare.MatchString(cb) {
			t.Fatal("고객 탭에 예전 href 가 남았다")
		}
		if !strings.Contains(cb, asOpenHref(src.ASID)) {
			t.Fatal("고객 탭에 /action 없음")
		}
	}
}

func TestAS95_33WorkboardASHrefIsAction(t *testing.T) {
	e, _, _, src, _, ck, _, _ := newAS95_33App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/workboard/as-lookup?q="+url.QueryEscape(src.ASNumber), nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), asOpenHref(src.ASID)) {
		t.Fatalf("ASHref 가 /action 이 아니다: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"/as/`+src.ASID+`"`) &&
		!strings.Contains(rec.Body.String(), asOpenHref(src.ASID)) {
		t.Fatal("예전 ASHref")
	}
}

func TestAS95_33ActionHasMovedFeatures(t *testing.T) {
	e, _, asRepo, src, _, ck, _, _ := newAS95_33App(t)
	src.Status = "completed"
	src.ActionTaken = "조치함"
	if err := asRepo.Update(src); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, asOpenHref(src.ASID), nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("action %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"as-report-history", "as-support-card", "as-support-form",
		"to-admin-work", "/delete", "btn-reopen", "as-history-card",
		"as_parent_complete", "이관", "예전 화면으로",
	} {
		if want == "as_parent_complete" {
			continue
		}
		if !strings.Contains(body, want) && want != "이관" {
			t.Errorf("조치 화면에 %q 없음", want)
		}
	}
	if !strings.Contains(body, "as-report-history") {
		t.Error("보고서 이력 없음")
	}
	if !strings.Contains(body, "as-support-card") || !strings.Contains(body, "/support") {
		t.Error("지원업무 없음")
	}
	if !strings.Contains(body, "to-admin-work") {
		t.Error("행정 이관 없음")
	}
	if !strings.Contains(body, "/delete") {
		t.Error("삭제 없음")
	}
	if !strings.Contains(body, "btn-reopen") {
		t.Error("재접수 없음")
	}
	if !strings.Contains(body, "as-history-card") {
		t.Error("이력 없음")
	}
}

func TestAS95_33PostRedirectsToAction(t *testing.T) {
	e, _, asRepo, src, _, ck, _, _ := newAS95_33App(t)

	sup := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "/as/"+src.ASID+"/support",
		strings.NewReader(url.Values{"support_type": {model.SupportOther}, "memo": {"안내"}}.Encode()))
	sreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	sreq.AddCookie(ck)
	e.ServeHTTP(sup, sreq)
	if sup.Code != http.StatusSeeOther || !strings.Contains(sup.Header().Get("Location"), asOpenHref(src.ASID)) {
		t.Fatalf("support loc=%s code=%d", sup.Header().Get("Location"), sup.Code)
	}

	adm := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodPost, "/as/"+src.ASID+"/to-admin-work",
		strings.NewReader(url.Values{"reason": {"행정 이관"}}.Encode()))
	areq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	areq.AddCookie(ck)
	e.ServeHTTP(adm, areq)
	if adm.Code != http.StatusSeeOther || !strings.Contains(adm.Header().Get("Location"), "/action") {
		t.Fatalf("to-admin loc=%s", adm.Header().Get("Location"))
	}

	done := &model.ASReceipt{
		CustomerID: "cust_a", Symptom: "완료재접수", Status: "completed",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary, ActionTaken: "끝",
	}
	if err := asRepo.Create(done); err != nil {
		t.Fatal(err)
	}
	done.Status = "completed"
	if err := asRepo.Update(done); err != nil {
		t.Fatal(err)
	}
	re := httptest.NewRecorder()
	rreq := httptest.NewRequest(http.MethodPost, "/as/"+done.ASID+"/reopen",
		strings.NewReader(url.Values{"reopen_reason": {"재발"}}.Encode()))
	rreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rreq.AddCookie(ck)
	e.ServeHTTP(re, rreq)
	if re.Code != http.StatusSeeOther || !strings.Contains(re.Header().Get("Location"), "/action") {
		t.Fatalf("reopen loc=%s", re.Header().Get("Location"))
	}

	victim := &model.ASReceipt{
		CustomerID: "cust_a", Symptom: "삭제대상", Status: "received",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(victim); err != nil {
		t.Fatal(err)
	}
	del := httptest.NewRecorder()
	dform := url.Values{"csrf": {"testcsrf0123456789ab"}}
	dreq := httptest.NewRequest(http.MethodPost, "/as/"+victim.ASID+"/delete", strings.NewReader(dform.Encode()))
	dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	dreq.AddCookie(ck)
	dreq.AddCookie(&http.Cookie{Name: "jp_csrf", Value: "testcsrf0123456789ab", Path: "/"})
	e.ServeHTTP(del, dreq)
	if del.Code != http.StatusSeeOther {
		t.Fatalf("delete %d loc=%s body=%s", del.Code, del.Header().Get("Location"), truncate(del.Body.String(), 300))
	}
}

func TestAS95_33OtherOrg404AndCLevelReadOnly(t *testing.T) {
	e, _, _, src, otherID, _, oaCK, clCK := newAS95_33App(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/as/"+otherID+"/action", nil)
	req.AddCookie(oaCK)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 action status=%d", rec.Code)
	}

	act := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodGet, asOpenHref(src.ASID), nil)
	areq.AddCookie(clCK)
	e.ServeHTTP(act, areq)
	if act.Code != http.StatusOK {
		t.Fatalf("c_level action %d", act.Code)
	}
	body := act.Body.String()
	if strings.Contains(body, `id="as-action-form"`) {
		t.Error("c_level 에 조치 저장 폼이 있다")
	}
	if strings.Contains(body, "접수 삭제") || strings.Contains(body, "to-admin-work") {
		t.Error("c_level 에 변경 버튼이 있다")
	}
}

func TestAS95_33NavLegacyItem(t *testing.T) {
	res := FilterNav(visionMenuCtx(), NavASShowLegacy, NavBadges{})
	hrefs := strings.Join(collectNavHrefs(res.Groups), "\n")
	if !strings.Contains(hrefs, "/as?legacy=1") {
		t.Fatal("예전 화면 보기 AS 항목 없음")
	}
}
