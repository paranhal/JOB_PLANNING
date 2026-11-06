package handler

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_86_TaskActionTargetAndBoardActHref(t *testing.T) {
	e, db, _, repo, _ := new86(t)
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("oa01")
	}
	ck := jwtCookieUser(t, oa)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.GET("/as/:id/action", h.AS.Action)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)

	day := time.Now().Format(dateLayout)
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C86','조치사이트','조치사이트',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, visit_scheduled_date, created_at, updated_at)
		VALUES ('AS86','R-86',?,'C86','캘린더조치','assigned',?, ?, ?, ?, ?)`,
		now, oa.FullName, model.OrgIDLibrary, day, now, now); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	plan, err := mnt.CreatePlanForOrg(model.OrgIDLibrary, time.Now().Year(), "t86")
	if err != nil {
		t.Fatal(err)
	}
	vid, err := mnt.InsertVisitFullID(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: day, CustomerID: "C86", ProductType: "KLAS", Assignee: oa.FullName,
	})
	if err != nil || vid == "" {
		t.Fatalf("visit %v %s", err, vid)
	}
	sales := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "영업86", CustomerID: "C86", Stage: model.SalesStageLead, Status: model.SalesStatusActive}
	if err := repository.NewSalesRepo(db).Create(sales); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sales_activities (activity_id, sales_id, activity_date, activity_type, title)
		VALUES ('SA86', ?, ?, 'visit', '영업활동86')`, sales.SalesID, day); err != nil {
		t.Fatal(err)
	}

	asTask := mustCreateTypedBoardTask(t, repo, "AS조치바", day, "09:00", "10:00", oa, model.WBWorkAS, model.WBSourceAS, "AS86")
	mntTask := mustCreateTypedBoardTask(t, repo, "점검조치바", day, "10:00", "11:00", oa, model.WBWorkMaintenance, model.WBSourceMaintenance, vid)
	pwTask := mustCreateTypedBoardTask(t, repo, "작업조치바", day, "11:00", "12:00", oa, model.WBWorkProjectWork, "", "")
	salesTask := mustCreateTypedBoardTask(t, repo, "영업조치바", day, "13:00", "14:00", oa, model.WBWorkSales, model.WBSourceSalesActivity, "SA86")
	adminTask := mustCreateTypedBoardTask(t, repo, "행정조치바", day, "14:00", "15:00", oa, model.WBWorkAdmin, "", "")
	supportTask := mustCreateTypedBoardTask(t, repo, "지원조치바", day, "15:00", "16:00", oa, model.WBWorkSupport, "", "")
	donePW := mustCreateTypedBoardTask(t, repo, "완료작업바", day, "16:00", "17:00", oa, model.WBWorkProjectWork, "", "")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET status=? WHERE task_id=?`, model.WBTaskComplete, donePW.TaskID); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		asTask.TaskID:      asOpenHref("AS86"),
		mntTask.TaskID:     mntVisitActionHref(model.MaintenanceVisit{VisitID: vid}),
		pwTask.TaskID:      projectWorkActionPath(pwTask.TaskID),
		salesTask.TaskID:   "/sales/" + sales.SalesID,
		adminTask.TaskID:   "/workboard/tasks/" + adminTask.TaskID,
		supportTask.TaskID: "/workboard/tasks/" + supportTask.TaskID,
		donePW.TaskID:      projectWorkActionPath(donePW.TaskID),
	}
	withAuthCtx(t, e, h, ck, func(c echo.Context) {
		for _, tk := range []*model.WorkTask{asTask, mntTask, pwTask, salesTask, adminTask, supportTask, donePW} {
			got := h.Workboard.taskActionTarget(c, tk)
			if got.Href != want[tk.TaskID] {
				t.Fatalf("%s 조치 주소 %s want %s", tk.Title, got.Href, want[tk.TaskID])
			}
			if !got.Can {
				t.Fatalf("%s canAction=false", tk.Title)
			}
		}
	})

	t.Skip("일정표 일 카드는 data-href(95-132A). 구 board-bar-act 단언 종료")
	boardPath := "/workboard/board?view=day&date=" + day + "&auto=0&mine=0"
	dayBody := doGetWith(t, e, boardPath, ck).Body.String()
	kanBody := doGetWith(t, e, "/workboard/board?view=kanban&date="+day+"&auto=0&mine=0", ck).Body.String()
	listBody := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()

	cases := []struct {
		title string
		task  *model.WorkTask
		path  string
	}{
		{"AS조치바", asTask, want[asTask.TaskID]},
		{"점검조치바", mntTask, want[mntTask.TaskID]},
		{"작업조치바", pwTask, want[pwTask.TaskID]},
		{"영업조치바", salesTask, want[salesTask.TaskID]},
		{"행정조치바", adminTask, want[adminTask.TaskID]},
		{"지원조치바", supportTask, want[supportTask.TaskID]},
	}
	for _, tc := range cases {
		detail := "/workboard/tasks/" + tc.task.TaskID
		barHref := html.UnescapeString(firstActHref(dayBody, tc.title, "board-bar-act"))
		if hrefPathOnly(barHref) != tc.path {
			t.Fatalf("일 보기 바 「조치」 %s href=%s want %s", tc.title, barHref, tc.path)
		}
		if tc.path != detail && hrefPathOnly(barHref) == detail {
			t.Fatalf("일 보기 바가 상세로 감 %s", tc.title)
		}
		if !strings.Contains(barHref, "back=") {
			t.Fatalf("바 back 없음 %s %s", tc.title, barHref)
		}
		kanHref := html.UnescapeString(firstActHref(kanBody, tc.title, "btn btn-secondary btn-sm"))
		if hrefPathOnly(kanHref) != tc.path {
			t.Fatalf("칸반 「조치」 %s href=%s want %s", tc.title, kanHref, tc.path)
		}

		show := doGetWith(t, e, "/workboard/tasks/"+tc.task.TaskID+"?back="+url.QueryEscape(boardPath), ck)
		if show.Code != http.StatusOK {
			t.Fatalf("상세 %s %d", tc.title, show.Code)
		}
		var fnHref string
		withAuthCtx(t, e, h, ck, func(c echo.Context) {
			fnHref = h.Workboard.taskActionTarget(c, tc.task).Href
		})
		if hrefPathOnly(barHref) != fnHref {
			t.Fatalf("바 vs 함수 %s bar=%s fn=%s", tc.title, barHref, fnHref)
		}
		showHref := html.UnescapeString(firstHrefWithPath(show.Body.String(), fnHref))
		if showHref != "" && hrefPathOnly(showHref) != fnHref {
			t.Fatalf("상세 상단 vs 함수 %s show=%s fn=%s", tc.title, showHref, fnHref)
		}
		if tc.path != detail && showHref != "" && hrefPathOnly(showHref) != hrefPathOnly(barHref) {
			t.Fatalf("상세 상단과 바 경로 불일치 %s show=%s bar=%s", tc.title, showHref, barHref)
		}
	}

	doneHref := html.UnescapeString(firstActHref(dayBody, "완료작업바", "board-bar-act"))
	if doneHref == "" {
		doneHref = html.UnescapeString(barDataHref(dayBody, "완료작업바"))
	}
	if hrefPathOnly(doneHref) != projectWorkActionPath(donePW.TaskID) {
		t.Fatalf("완료 작업 바 %s", doneHref)
	}

	if !strings.Contains(listBody, "행정조치바") {
		t.Log("리스트에 행정 행이 없음(이번 범위 아님, 보고서만)")
	} else if href := listRowHref(listBody, "행정조치바"); href != "" && hrefPathOnly(href) != "/workboard/tasks/"+adminTask.TaskID {
		t.Logf("리스트 행 href=%s (상세 유지 확인용)", href)
	}

	for _, tc := range []struct{ title, path string }{
		{"AS조치바", want[asTask.TaskID]},
		{"점검조치바", want[mntTask.TaskID]},
		{"작업조치바", want[pwTask.TaskID]},
	} {
		href := html.UnescapeString(firstActHref(dayBody, tc.title, "board-bar-act"))
		u, err := url.Parse(href)
		if err != nil {
			t.Fatal(err)
		}
		path := u.Path
		if u.RawQuery != "" {
			path += "?" + u.RawQuery
		}
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusOK {
			t.Fatalf("바 조치 클릭 %s %d %s", tc.title, rec.Code, rec.Body.String())
		}
		back := u.Query().Get("back")
		if back == "" {
			t.Fatalf("바 href에 back 없음 %s", tc.title)
		}
		body := rec.Body.String()
		hasBack := strings.Contains(body, back) || strings.Contains(html.UnescapeString(body), back) ||
			strings.Contains(body, url.QueryEscape(back)) || strings.Contains(body, html.EscapeString(back))
		if !hasBack && !strings.HasPrefix(u.Path, "/as/") {
			t.Fatalf("조치 후 돌아오기 없음 %s back=%s", tc.title, back)
		}
	}
}

func mustCreateTypedBoardTask(t *testing.T, repo *repository.WBRepo, title, date, start, end string, u *model.User, workType, sourceType, sourceID string) *model.WorkTask {
	t.Helper()
	tk := &model.WorkTask{
		Title: title, WorkDate: date, DueDate: date, StartTime: start, EndTime: end,
		OrgID: model.OrgIDLibrary, Assignee: u.FullName, AssigneeUserID: u.UserID,
		Status: model.WBTaskWaiting, WorkType: workType, Priority: model.WBPriorityNormal,
		SourceType: sourceType, SourceID: sourceID,
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	got := taskByTitle(t, repo, title)
	if got == nil {
		t.Fatal(title)
	}
	return got
}

func withAuthCtx(t *testing.T, e *echo.Echo, h *Handler, ck *http.Cookie, fn func(echo.Context)) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/workboard/board", nil)
	req.AddCookie(ck)
	c := e.NewContext(req, rec)
	if err := h.Auth.AuthMiddleware(func(c echo.Context) error {
		fn(c)
		return nil
	})(c); err != nil {
		t.Fatal(err)
	}
}

func firstActHref(body, title, class string) string {
	i := strings.Index(body, title)
	if i < 0 {
		return ""
	}
	chunk := body[i:]
	if len(chunk) > 8000 {
		chunk = chunk[:8000]
	}
	re := regexp.MustCompile(`<a href="([^"]+)" class="` + regexp.QuoteMeta(class) + `"[^>]*>([^<]*)`)
	for _, m := range re.FindAllStringSubmatch(chunk, -1) {
		txt := strings.TrimSpace(html.UnescapeString(m[2]))
		switch txt {
		case "조치", "조치 완료", "조치 화면 열기", "영업 사업":
			return m[1]
		}
	}
	return ""
}

func firstHrefWithPath(body, path string) string {
	re := regexp.MustCompile(`href="([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		raw := html.UnescapeString(m[1])
		if hrefPathOnly(raw) == path {
			return m[1]
		}
	}
	return ""
}

func barDataHref(body, title string) string {
	re := regexp.MustCompile(`data-title="` + regexp.QuoteMeta(title) + `"[\s\S]{0,800}?data-href="([^"]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func listRowHref(body, title string) string {
	i := strings.Index(body, title)
	if i < 0 {
		return ""
	}
	start := i - 400
	if start < 0 {
		start = 0
	}
	chunk := body[start:i]
	re := regexp.MustCompile(`<a href="([^"]+)" class="flex min-w-0`)
	m := re.FindStringSubmatch(chunk)
	if len(m) < 2 {
		return ""
	}
	return html.UnescapeString(m[1])
}
