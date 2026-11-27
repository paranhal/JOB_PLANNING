package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type env225 struct {
	e               *echo.Echo
	h               *Handler
	oa, same, cross *model.User // 배정자(도서관 조직관리자) · 같은 조직 일반 조직원 · 타 조직 조직원
	ar              *repository.AssignmentRepo
	do              func(method, path string, form url.Values, u *model.User, jsonOut bool) *httptest.ResponseRecorder
	dbExec          func(q string, a ...interface{})
	siteStatus      func(task, cust string) string
}

func setup225(t *testing.T) env225 {
	t.Helper()
	e, db, same, cross, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "a225.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("oa01")
	}
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C1','가도서관','가도서관',1,'O01'),('C2','나도서관','나도서관',1,'O01')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','충남 구축 사업','active','O01')`,
		`INSERT INTO project_phases(phase_id,project_id,org_id,name,seq,status,created_at,updated_at) VALUES ('PH1','WP1','O01','구축',1,'active',datetime('now'),datetime('now'))`,
		`INSERT INTO phase_packages(package_id,phase_id,seq,name,created_at,updated_at) VALUES ('PK1','PH1',1,'장비 설치',datetime('now'),datetime('now'))`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,package_id,work_date,due_date,start_time,end_time,created_by_user_id)
		 VALUES ('T1','project_work','장비 설치 작업','waiting','O01','WP1','PK1','2026-12-01','2026-12-01','09:00','10:00',?)`,
		`INSERT INTO work_task_sites(task_id,customer_id) VALUES ('T1','C1'),('T1','C2')`,
	} {
		if strings.Contains(q, "created_by_user_id") {
			ex(q, oa.UserID)
		} else {
			ex(q)
		}
	}
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/assignments/popup", h.Workboard.AssignmentsPopup)
	g.POST("/workboard/assignments/:id/decide", h.Workboard.AssignmentDecide)
	g.GET("/workboard/assignments/users", h.Workboard.AssignmentUsers)
	g.GET("/workboard/assignments/:id", h.Workboard.AssignmentShow)
	g.POST("/workboard/tasks/:id/site-check", h.Workboard.SaveSiteCheck)
	g.POST("/workboard/tasks/:id/collab", h.Workboard.UpdateTaskCollab)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	do := func(method, path string, form url.Values, u *model.User, jsonOut bool) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			req = httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		} else {
			req = httptest.NewRequest(method, "http://localhost"+path, nil)
		}
		if jsonOut {
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Requested-With", "fetch")
		}
		req.AddCookie(jwtCookieVerified(t, u))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	return env225{e: e, h: h, oa: oa, same: same, cross: cross, ar: repository.NewAssignmentRepo(db), do: do, dbExec: ex,
		siteStatus: func(task, cust string) string {
			var s string
			_ = db.QueryRow(`SELECT COALESCE(status,'') FROM work_task_site_progress WHERE task_id=? AND customer_id=? ORDER BY work_date DESC LIMIT 1`, task, cust).Scan(&s)
			return s
		}}
}

type popup225 struct {
	Items []repository.AssignmentItem `json:"items"`
	First bool                        `json:"first"`
}

func (e env225) popup(t *testing.T, u *model.User, withSeen *http.Cookie) (popup225, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/workboard/assignments/popup", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(jwtCookieVerified(t, u))
	if withSeen != nil {
		req.AddCookie(withSeen)
	}
	rec := httptest.NewRecorder()
	e.e.ServeHTTP(rec, req)
	var p popup225
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("popup %d %s", rec.Code, rec.Body.String())
	}
	return p, rec
}

// 팝업: 세션 첫 확인은 대기+보류(사업·배정자·조직·대상 표시), 이후는 대기만. 처리 결과는 한 번만 반영된다.
func TestHTTP95_225_PopupAndDecide(t *testing.T) {
	e := setup225(t)
	a1, err := e.ar.Create("T1", repository.AssignTargetSite, "C1", e.oa.UserID, e.cross.UserID)
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := e.ar.Create("T1", repository.AssignTargetSite, "C2", e.oa.UserID, e.cross.UserID)
	// 보류 하나를 만들어 둔다
	if r := e.do("POST", "/workboard/assignments/"+a2.ID+"/decide", url.Values{"action": {"hold"}}, e.cross, true); r.Code != 200 {
		t.Fatalf("보류 %d %s", r.Code, r.Body.String())
	}
	first, rec := e.popup(t, e.cross, nil)
	if !first.First || len(first.Items) != 2 {
		t.Fatalf("로그인 첫 확인: first=%v items=%d want 대기+보류 2건", first.First, len(first.Items))
	}
	it := first.Items[0]
	if it.ProjectName != "충남 구축 사업" || it.FromName != "도서관관리자" || it.FromOrgName != "도서관사업팀" && it.FromOrgName == "" || it.TargetLabel == "" {
		t.Fatalf("팝업 표시 정보 %+v", it)
	}
	var seen *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == assignSeenCookie {
			seen = c
		}
	}
	if seen == nil {
		t.Fatal("첫 확인 뒤 세션 쿠키가 없다")
	}
	poll, _ := e.popup(t, e.cross, seen)
	if poll.First || len(poll.Items) != 1 || poll.Items[0].ID != a1.ID {
		t.Fatalf("폴링: first=%v items=%v want 대기 1건", poll.First, poll.Items)
	}
	// 다른 사용자의 팝업에는 보이지 않는다
	if other, _ := e.popup(t, e.same, nil); len(other.Items) != 0 {
		t.Fatalf("남의 배정이 팝업에 보인다 %v", other.Items)
	}
	// 거부는 사유 필수(빈 값 400), 남의 배정은 403, 수락하면 종결
	if r := e.do("POST", "/workboard/assignments/"+a1.ID+"/decide", url.Values{"action": {"reject"}, "reason": {"  "}}, e.cross, true); r.Code != http.StatusBadRequest {
		t.Fatalf("빈 사유 거부 %d want 400", r.Code)
	}
	if r := e.do("POST", "/workboard/assignments/"+a1.ID+"/decide", url.Values{"action": {"accept"}}, e.same, true); r.Code != http.StatusForbidden {
		t.Fatalf("남의 배정 수락 %d want 403", r.Code)
	}
	if r := e.do("POST", "/workboard/assignments/"+a1.ID+"/decide", url.Values{"action": {"accept"}}, e.cross, true); r.Code != 200 {
		t.Fatalf("수락 %d %s", r.Code, r.Body.String())
	}
	if r := e.do("POST", "/workboard/assignments/"+a1.ID+"/decide", url.Values{"action": {"reject"}, "reason": {"늦음"}}, e.cross, true); r.Code != http.StatusBadRequest {
		t.Fatalf("수락 뒤 거부 %d want 400(종결)", r.Code)
	}
}

// 조직 격리: 타 조직 수락자는 수락 전에는 자기 배정 화면의 최소 정보만, 사이트 체크·업무 화면은 403/404.
// 수락 뒤에는 배정받은 사이트만 체크할 수 있고 그 외 사이트·업무 전체는 막힌다.
func TestHTTP95_225_ScopedAccessAfterAccept(t *testing.T) {
	e := setup225(t)
	a, err := e.ar.Create("T1", repository.AssignTargetSite, "C1", e.oa.UserID, e.cross.UserID)
	if err != nil {
		t.Fatal(err)
	}
	check := func(cust string) int {
		return e.do("POST", "/workboard/tasks/T1/site-check", url.Values{"customer_id": {cust}, "work_date": {"2026-12-01"}, "status": {"done"}, "redirect": {"/workboard/assignments/" + a.ID}}, e.cross, false).Code
	}
	// 수락 전: 배정 화면은 열리지만 최소 정보만, 사이트 체크는 403
	r := e.do("GET", "/workboard/assignments/"+a.ID, nil, e.cross, false)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "충남 구축 사업") || strings.Contains(r.Body.String(), "data-assign-site-done") {
		t.Fatalf("수락 전 배정 화면 %d (체크 버튼이 보이면 안 된다)", r.Code)
	}
	if c := check("C1"); c != http.StatusForbidden {
		t.Fatalf("수락 전 사이트 체크 %d want 403", c)
	}
	if r := e.do("POST", "/workboard/assignments/"+a.ID+"/decide", url.Values{"action": {"accept"}}, e.cross, true); r.Code != 200 {
		t.Fatalf("수락 %d", r.Code)
	}
	// 수락 뒤: 배정받은 사이트만 체크, 다른 사이트는 403, 업무 전체 화면은 404(타 조직)
	if c := check("C1"); c != http.StatusSeeOther {
		t.Fatalf("수락 뒤 배정 사이트 체크 %d want 303", c)
	}
	if e.siteStatus("T1", "C1") != "done" {
		t.Fatal("체크가 저장되지 않았다")
	}
	if c := check("C2"); c != http.StatusForbidden {
		t.Fatalf("배정 밖 사이트 체크 %d want 403", c)
	}
	if got := e.do("GET", "/workboard/tasks/T1/action", nil, e.cross, false).Code; got != http.StatusNotFound {
		t.Fatalf("타 조직 업무 전체 화면 %d want 404", got)
	}
	shown := e.do("GET", "/workboard/assignments/"+a.ID, nil, e.cross, false).Body.String()
	if !strings.Contains(shown, "data-assign-site-done") || strings.Contains(shown, "나도서관") {
		t.Fatal("수락 뒤 배정 화면에는 배정받은 사이트만 있어야 한다")
	}
	// 제3자는 배정 화면도 못 연다
	if got := e.do("GET", "/workboard/assignments/"+a.ID, nil, e.same, false).Code; got != http.StatusNotFound {
		t.Fatalf("제3자 배정 화면 %d want 404", got)
	}
	// 하부 업무 배정을 수락하면 그 업무의 모든 사이트를 체크할 수 있다
	e2 := setup225(t)
	sub, _ := e2.ar.Create("T1", repository.AssignTargetSub, "PK1", e2.oa.UserID, e2.cross.UserID)
	_ = e2.do("POST", "/workboard/assignments/"+sub.ID+"/decide", url.Values{"action": {"accept"}}, e2.cross, true)
	for _, c := range []string{"C1", "C2"} {
		if got := e2.do("POST", "/workboard/tasks/T1/site-check", url.Values{"customer_id": {c}, "work_date": {"2026-12-01"}, "status": {"done"}}, e2.cross, false).Code; got != http.StatusSeeOther {
			t.Fatalf("하부 업무 수락 뒤 %s 체크 %d", c, got)
		}
	}
}

// 공동 작업: 지정한 조직원(타 조직 포함)만 조치·사이트 체크, 범위 밖은 403. 같은 조직 범위는 그 조직원 누구나. 해제는 등록자·조직관리자만.
func TestHTTP95_225_Collab(t *testing.T) {
	e := setup225(t)
	siteCheck := func(u *model.User) int {
		return e.do("POST", "/workboard/tasks/T1/site-check", url.Values{"customer_id": {"C1"}, "work_date": {"2026-12-01"}, "status": {"done"}}, u, false).Code
	}
	// 공동 작업 없음: 타 조직은 403
	if c := siteCheck(e.cross); c != http.StatusForbidden {
		t.Fatalf("공동 작업 전 타 조직 %d want 403", c)
	}
	// 지정한 사용자만(타 조직 포함): 지정된 사람만 통과
	if r := e.do("POST", "/workboard/tasks/T1/collab", url.Values{"collab": {"1"}, "collab_scope": {"members"}, "collab_member": {e.cross.UserID}}, e.oa, true); r.Code != 200 {
		t.Fatalf("범위 지정 %d %s", r.Code, r.Body.String())
	}
	if c := siteCheck(e.cross); c != http.StatusSeeOther {
		t.Fatalf("지정된 타 조직원 사이트 체크 %d want 303", c)
	}
	// 같은 조직 범위는 그 조직 구성원이 조치 화면 권한을 갖는다(권한 판정 함수 단위)
	if scope, users := e.ar.CollabOf("T1"); scope != repository.CollabMembers || len(users) != 1 || users[0] != e.cross.UserID {
		t.Fatalf("저장된 범위 %q %v", scope, users)
	}
	if r := e.do("POST", "/workboard/tasks/T1/collab", url.Values{"collab": {"1"}, "collab_scope": {"members"}}, e.oa, true); r.Code != http.StatusBadRequest {
		t.Fatalf("구성원 없는 지정 %d want 400", r.Code)
	}
	// 범위 변경·해제는 등록자·조직관리자만: 일반 조직원은 403
	if r := e.do("POST", "/workboard/tasks/T1/collab", url.Values{"collab": {"0"}}, e.same, true); r.Code != http.StatusForbidden {
		t.Fatalf("일반 조직원 해제 %d want 403", r.Code)
	}
	if r := e.do("POST", "/workboard/tasks/T1/collab", url.Values{"collab": {"0"}}, e.oa, true); r.Code != 200 {
		t.Fatalf("관리자 해제 %d", r.Code)
	}
	if c := siteCheck(e.cross); c != http.StatusForbidden {
		t.Fatalf("해제 뒤 타 조직 %d want 403", c)
	}
}

// 수락한 배정은 수락자의 일정표·칸반에 가상 카드로 보이고, 대기·보류·거부는 보이지 않는다(조직 기준은 수락자의 조직).
func TestAssignCards95_225(t *testing.T) {
	e := setup225(t)
	a1, _ := e.ar.Create("T1", repository.AssignTargetSite, "C1", e.oa.UserID, e.cross.UserID)
	a2, _ := e.ar.Create("T1", repository.AssignTargetSite, "C2", e.oa.UserID, e.cross.UserID)
	_, _ = e.ar.Decide(a2.ID, e.cross.UserID, "hold", "")
	if got := e.h.Workboard.assignCardTasks("2026-12-01", "2026-12-01", ""); len(got) != 0 {
		t.Fatalf("수락 전에 카드가 보인다 %d", len(got))
	}
	if _, err := e.ar.Decide(a1.ID, e.cross.UserID, "accept", ""); err != nil {
		t.Fatal(err)
	}
	cards := e.h.Workboard.assignCardTasks("2026-12-01", "2026-12-01", model.OrgIDLibrary)
	if len(cards) != 0 {
		t.Fatalf("배정자 조직 기준으로 수락자 카드가 보인다 %d (수락자 조직 기준이어야 한다)", len(cards))
	}
	cards = e.h.Workboard.assignCardTasks("2026-12-01", "2026-12-01", e.cross.OrgID)
	if len(cards) != 1 || cards[0].Assignee != e.cross.FullName || cards[0].StartTime != "09:00" || !isAssignCardID(cards[0].TaskID) {
		t.Fatalf("일정표 카드 %+v", cards)
	}
	items := e.h.Workboard.assignKanbanItems("2026-12-01", e.cross.OrgID)
	if len(items) != 1 || items[0].Href != "/workboard/assignments/"+a1.ID || !items[0].EditLocked {
		t.Fatalf("칸반 카드 %+v", items)
	}
	if got := e.h.Workboard.assignCardTasks("2026-12-02", "2026-12-03", ""); len(got) != 0 {
		t.Fatal("부모 업무 날짜 밖인데 카드가 보인다")
	}
}

// 같은 조직 범위 공동 작업: 담당이 아닌 같은 조직 일반 조직원이 조치 화면의 처리 권한을 갖는다(범위 밖 조직은 아님). 처리자(user_id)가 이력에 남는다.
func TestHTTP95_225_CollabOrgScopeAndActor(t *testing.T) {
	e := setup225(t)
	canProcess := func(u *model.User) bool {
		r := e.do("GET", "/workboard/tasks/T1/action", nil, u, false)
		return r.Code == 200 && strings.Contains(r.Body.String(), "사업 연결·변경")
	}
	if canProcess(e.same) {
		t.Fatal("공동 작업 전인데 담당이 아닌 일반 조직원에게 처리 권한이 있다")
	}
	if r := e.do("POST", "/workboard/tasks/T1/collab", url.Values{"collab": {"1"}, "collab_scope": {"org"}}, e.oa, true); r.Code != 200 {
		t.Fatalf("조직 범위 지정 %d %s", r.Code, r.Body.String())
	}
	if !canProcess(e.same) {
		t.Fatal("같은 조직 범위 공동 작업인데 일반 조직원에게 처리 권한이 없다")
	}
	// 다른 조직원은 같은 조직 범위로는 열리지 않는다(404: 조직 격리)
	if got := e.do("GET", "/workboard/tasks/T1/action", nil, e.cross, false).Code; got != http.StatusNotFound {
		t.Fatalf("다른 조직 %d want 404", got)
	}
	// 사이트 체크한 사람이 처리자로 남는다(등록자 ≠ 수행자)
	if got := e.do("POST", "/workboard/tasks/T1/site-check", url.Values{"customer_id": {"C1"}, "work_date": {"2026-12-01"}, "status": {"done"}}, e.same, false).Code; got != http.StatusSeeOther {
		t.Fatalf("공동 작업자 체크 %d", got)
	}
	var by string
	_ = e.h.Workboard.repo.DB().QueryRow(`SELECT COALESCE(checked_by,'') FROM work_task_site_progress WHERE task_id='T1' AND customer_id='C1'`).Scan(&by)
	if by != e.same.UserID {
		t.Fatalf("처리자 이력 %q want %q", by, e.same.UserID)
	}
}

// 브라우저: 배정받은 업무 팝업(1440/390). 타 조직 사용자가 로그인한 상태로 업무 화면을 연다.
func Test95_225_BrowserAssignPopup(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e := setup225(t)
	e.dbExec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C3','다도서관','다도서관',1,'O01')`)
	e.dbExec(`INSERT INTO work_task_sites(task_id,customer_id) VALUES ('T1','C3')`)
	a1, _ := e.ar.Create("T1", repository.AssignTargetSite, "C1", e.oa.UserID, e.cross.UserID)
	a2, _ := e.ar.Create("T1", repository.AssignTargetSite, "C2", e.oa.UserID, e.cross.UserID)
	_ = a1
	if _, err := e.ar.Decide(a2.ID, e.cross.UserID, "hold", ""); err != nil {
		t.Fatal(err)
	}
	// 시험 전용: 접속 중 새 배정 도착을 흉내 낸다(대상 사이트별로 만든다).
	e.e.GET("/__test/assign", func(c echo.Context) error {
		x, err := e.ar.Create("T1", repository.AssignTargetSite, c.QueryParam("target"), e.oa.UserID, e.cross.UserID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"id": x.ID})
	})
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.e.Static("/static", staticDir)
	srv := httptest.NewServer(e.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-225"))
	cmd := exec.Command("node", "95_225_assign_popup_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, e.cross).Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
}
