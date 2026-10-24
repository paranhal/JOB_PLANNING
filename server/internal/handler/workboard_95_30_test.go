package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestWorkboardHomeURLAndDate(t *testing.T) {
	if got := workboardHomeURL(""); got != "/workboard/board" {
		t.Fatalf("빈 날짜 %s", got)
	}
	if got := workboardHomeURL("2026-10-24"); got != "/workboard/board?view=month&date=2026-10-24" {
		t.Fatalf("정상 날짜 %s", got)
	}
	today := time.Now().Format(dateLayout)
	if got := workboardHomeURL("not-a-date"); got != "/workboard/board?view=month&date="+today {
		t.Fatalf("잘못된 날짜 %s", got)
	}
	if got := workboardHomeDate("2026-10-24T09:00"); got != "2026-10-24" {
		t.Fatalf("시각 잘림 %s", got)
	}
	if got := workboardCalendarURL("2026-09-01", "2026-12-31"); !strings.Contains(got, "date=2026-09-01") {
		t.Fatalf("작업일 우선 %s", got)
	}
	if got := workboardCalendarURL("", "2026-12-31"); !strings.Contains(got, "date=2026-12-31") {
		t.Fatalf("예정일 %s", got)
	}
	if got := workboardCalendarURL("", ""); !strings.Contains(got, "date="+today) {
		t.Fatalf("오늘 %s", got)
	}
}

func TestSafeWorkboardRedirectFallsBackToHome(t *testing.T) {
	home := workboardHomeURL("")
	if got := safeWorkboardRedirect("https://evil.example/x", home); got != home {
		t.Fatalf("외부 %s", got)
	}
	if got := safeWorkboardRedirect("//evil", home); got != home {
		t.Fatalf("프로토콜상대 %s", got)
	}
	if got := safeWorkboardRedirect("/admin-work", home); got != home {
		t.Fatalf("허용목록 밖은 기본값이어야 한다 %s", got)
	}
	if got := safeWorkboardRedirect("/workboard/board?view=month&date=2026-10-24", home); got != "/workboard/board?view=month&date=2026-10-24" {
		t.Fatalf("허용 %s", got)
	}
}

func mountWorkboard95_30(t *testing.T) (*echo.Echo, *repository.WBRepo, *Handler) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "wb30.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.POST("/workboard/tasks/:id/action", h.Workboard.SaveProjectWorkAction)
	g.POST("/workboard/tasks/:id/delete", h.Workboard.DeleteRecurrenceParent)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.POST("/maintenance/visits/:visit_id/action", h.Maintenance.UpdateVisitAction)
	g.GET("/as/:id/action", h.AS.Action)
	return e, repository.NewWBRepo(db), h
}

func create30Task(t *testing.T, repo *repository.WBRepo, title, workType, workDate, dueDate, org string) *model.WorkTask {
	t.Helper()
	tk := &model.WorkTask{
		Title: title, WorkType: workType, WorkDate: workDate, DueDate: dueDate,
		OrgID: org, Assignee: "admin", Status: model.WBTaskWaiting, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	return tk
}

var calendarHrefRe = regexp.MustCompile(`(?s)href="(/workboard/board\?view=month&amp;date=\d{4}-\d{2}-\d{2})"[^>]*>\s*일정표`)

func calendarHref(body string) string {
	m := calendarHrefRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.ReplaceAll(m[1], "&amp;", "&")
}

func TestTaskShowCalendarAndBackLinks(t *testing.T) {
	e, repo, _ := mountWorkboard95_30(t)
	admin := create30Task(t, repo, "행정30", model.WBWorkAdmin, "2026-10-24", "2026-10-30", model.OrgIDLibrary)
	dueOnly := create30Task(t, repo, "예정만30", model.WBWorkAdmin, "", "2026-11-03", model.OrgIDLibrary)
	empty := create30Task(t, repo, "날짜없음30", model.WBWorkAdmin, "", "", model.OrgIDLibrary)
	work := create30Task(t, repo, "작업30", model.WBWorkProjectWork, "2026-10-21", "", model.OrgIDLibrary)
	sales := create30Task(t, repo, "영업30", model.WBWorkSales, "2026-10-22", "", model.OrgIDLibrary)
	asDaily := create30Task(t, repo, "AS일일30", model.WBWorkAS, "2026-10-23", "", model.OrgIDLibrary)
	mntDaily := create30Task(t, repo, "점검일일30", model.WBWorkMaintenance, "2026-10-20", "", model.OrgIDLibrary)

	today := time.Now().Format(dateLayout)
	cases := []struct {
		id, date string
	}{
		{admin.TaskID, "2026-10-24"},
		{dueOnly.TaskID, "2026-11-03"},
		{empty.TaskID, today},
		{work.TaskID, "2026-10-21"},
		{sales.TaskID, "2026-10-22"},
		{asDaily.TaskID, "2026-10-23"},
		{mntDaily.TaskID, "2026-10-20"},
	}
	for _, tc := range cases {
		page := doGet(t, e, "/workboard/tasks/"+tc.id)
		if page.Code != http.StatusOK {
			t.Fatalf("%s code=%d", tc.id, page.Code)
		}
		body := page.Body.String()
		href := calendarHref(body)
		want := "/workboard/board?view=month&date=" + tc.date
		if href != want {
			t.Fatalf("%s 일정표=%q want %s", tc.id, href, want)
		}
		if strings.Contains(body, "← 일일 업무 등록") {
			t.Fatalf("%s 예전 돌아가기 문구", tc.id)
		}
		if !strings.Contains(body, `href="/workboard/board"`) || !strings.Contains(body, "← 업무 관리") {
			t.Fatalf("%s 기본 돌아가기 없음", tc.id)
		}
	}

	back := doGet(t, e, "/workboard/tasks/"+admin.TaskID+"?back="+url.QueryEscape("/admin-work"))
	if !strings.Contains(back.Body.String(), `href="/admin-work"`) {
		t.Fatal("back=/admin-work 유지 안 됨")
	}
	if calendarHref(back.Body.String()) != "/workboard/board?view=month&date=2026-10-24" {
		t.Fatal("일정표가 BackURL을 쓰면 안 된다")
	}

	regBack := doGet(t, e, "/workboard/tasks/"+admin.TaskID+"?back="+url.QueryEscape("/workboard/register"))
	if !strings.Contains(regBack.Body.String(), "← 일일 업무 등록") {
		t.Fatal("예전 화면에서 들어온 돌아가기 문구")
	}
}

func TestProjectWorkActionBackDefault(t *testing.T) {
	e, repo, _ := mountWorkboard95_30(t)
	tk := create30Task(t, repo, "조치작업30", model.WBWorkProjectWork, "2026-10-21", "", model.OrgIDLibrary)
	page := doGet(t, e, "/workboard/tasks/"+tk.TaskID+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("code=%d %s", page.Code, page.Body.String()[:min(300, page.Body.Len())])
	}
	body := page.Body.String()
	if !strings.Contains(body, `href="/workboard/board"`) {
		t.Fatal("작업 조치 기본 돌아가기")
	}
	if strings.Contains(body, "← 일일 업무 등록") {
		t.Fatal("작업 조치에 예전 돌아가기 문구")
	}
	with := doGet(t, e, "/workboard/tasks/"+tk.TaskID+"/action?back="+url.QueryEscape("/admin-work"))
	if !strings.Contains(with.Body.String(), `href="/admin-work"`) {
		t.Fatal("작업 조치 back 존중")
	}
}

func TestVisitAndASActionBackDefault(t *testing.T) {
	e, _, h := mountWorkboard95_30(t)
	db := h.AS.db
	if _, err := db.Exec(`INSERT OR IGNORE INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C30','해미','해미',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('MP30', 2026, '계획', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('V30','MP30','2026-10-24','C30',0,'normal','admin','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	vis := doGet(t, e, "/maintenance/visits/V30/action")
	if vis.Code != http.StatusOK {
		t.Fatalf("visit %d %s", vis.Code, vis.Body.String()[:min(400, vis.Body.Len())])
	}
	vb := vis.Body.String()
	if !strings.Contains(vb, `href="/workboard/board"`) || strings.Contains(vb, "← 일일 업무 등록") {
		t.Fatal("점검 조치 기본 돌아가기")
	}

	asRepo := h.AS.repo
	src := &model.ASReceipt{
		CustomerID: "C30", ReceiptChannel: "phone", Requester: "요청",
		Symptom: "증상", Urgency: "normal", Priority: "normal",
		AssignedTo: "admin", ReceiptDatetime: time.Now(), Status: "in_progress",
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}
	asPage := doGet(t, e, "/as/"+src.ASID+"/action")
	if asPage.Code != http.StatusOK {
		t.Fatalf("as action %d", asPage.Code)
	}
	if !strings.Contains(asPage.Body.String(), `href="/workboard/board"`) || !strings.Contains(asPage.Body.String(), "업무 관리로") {
		t.Fatal("AS 조치 기본 돌아가기")
	}
	if strings.Contains(asPage.Body.String(), "일일 업무 등록으로") {
		t.Fatal("AS 예전 title")
	}
}

func TestBoardMonthFromCalendarLinkAndOrgDeny(t *testing.T) {
	e, repo, _ := mountWorkboard95_30(t)
	cal := "/workboard/board?view=month&date=2026-10-24"
	page := doGet(t, e, cal)
	if page.Code != http.StatusOK {
		t.Fatalf("board %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "2026년 10월") {
		t.Fatal("월 표시 없음")
	}

	other := create30Task(t, repo, "타조직30", model.WBWorkAdmin, "2026-10-24", "", "O03")
	deny := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/workboard/tasks/"+other.TaskID, nil)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(deny, req)
	if deny.Code != http.StatusNotFound && deny.Code != http.StatusForbidden {
		t.Fatalf("타 조직 상세 %d", deny.Code)
	}
}

func TestDeleteRecurrenceRedirectsHome(t *testing.T) {
	e, repo, _ := mountWorkboard95_30(t)
	tk := create30Task(t, repo, "삭제30", model.WBWorkAdmin, "2026-10-24", "", model.OrgIDLibrary)
	del := doForm(t, e, "/workboard/tasks/"+tk.TaskID+"/delete", url.Values{})
	if del.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", del.Code)
	}
	loc := del.Header().Get("Location")
	if !strings.HasPrefix(loc, "/workboard/board") {
		t.Fatalf("삭제 후 %s", loc)
	}
}

func TestDetailTemplatesHaveNoRegisterDefault(t *testing.T) {
	root := findTemplateRoot(t)
	deny := []string{
		"workboard/task_show.html",
		"workboard/project_work_action.html",
		"maintenance/visit_action.html",
		"as/action.html",
		"admin_work/list.html",
	}
	for _, rel := range deny {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "/workboard/register") {
			t.Errorf("%s 에 /workboard/register 기본값이 남아 있다", rel)
		}
	}
	keep := map[string]string{
		"workboard/register.html": "예전 화면 자신",
		"plan/unplanned.html":     "흡수 탭은 예전 등록 화면과 짝",
	}
	for rel, why := range keep {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "/workboard/register") {
			t.Errorf("%s 유지 사유(%s)인데 링크가 없다", rel, why)
		}
	}
}
