package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newWorkTodayServer(t *testing.T) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-today.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if err := users.Create(&model.User{
		Username: "other", PasswordHash: HashPassword("pw"), FullName: "다른담당",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}

	today := time.Now().Format("2006-01-02")
	repo := repository.NewWBRepo(db)
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "관리자할일", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: today, WorkDate: today, CustomerName: "중앙도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "다른담당진행", Status: model.WBTaskInProgress,
		Assignee: "다른담당", DueDate: today, WorkDate: today, CustomerName: "서부도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "기술보류건", Status: model.WBTaskHold,
		HoldReason: "일정조정", ReviewDate: today,
		Assignee: "tech", DueDate: today, WorkDate: today, CustomerName: "가나도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "관리자완료", Status: model.WBTaskComplete,
		Assignee: "관리자", DueDate: today, WorkDate: today, CompleteNote: "끝",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "취소된통합업무", Status: model.WBTaskCancelled,
		CancelReason: "중복", Assignee: "관리자", DueDate: today, WorkDate: today,
	})
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "어제지연행정", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: "2026-01-15", WorkDate: "2026-01-15", CustomerName: "지연도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "내일행정", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: tomorrow, WorkDate: tomorrow, CustomerName: "내일도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정미정", Status: model.WBTaskWaiting,
		Assignee: "관리자", CustomerName: "미정도서관",
	})

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work", h.Work.List)
	g.GET("/work/all", h.Work.ListAll)
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	g.GET("/admin-work", h.AdminWork.List)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.POST("/workboard/tasks/:id/delete", h.Workboard.DeleteRecurrenceParent)
	return e, repo
}

func mustCreateWorkToday(t *testing.T, repo *repository.WBRepo, task *model.WorkTask) {
	t.Helper()
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
}

func workTodayGet(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func TestWorkTodayAdminSeesOwnOnly(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?display=kanban", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "오늘 내 업무") || !strings.Contains(body, "진행 예정") {
		t.Fatal("칸반 열이 없다")
	}
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(body, today+" · 내 업무 · 오늘 예정") || !strings.Contains(body, "지연 1") || !strings.Contains(body, "오늘 완료") || !strings.Contains(body, "진행중") {
		t.Fatal("범위 한 줄이 없다")
	}
	if strings.Contains(body, "팀 전체 보기") || strings.Contains(body, "내 배정만") {
		t.Fatal("오늘 내 업무에 범위 토글이 남았다")
	}
	for _, title := range []string{"관리자할일", "관리자완료", "어제지연행정"} {
		if !strings.Contains(body, title) {
			t.Fatalf("관리자에게 %s 가 안 보인다", title)
		}
	}
	if !strings.Contains(body, "D+") && !strings.Contains(body, "D&#43;") {
		t.Fatal("칸반에 지연 뱃지가 없다")
	}
	if strings.Contains(body, "다른담당진행") || strings.Contains(body, "기술보류건") {
		t.Fatal("오늘 내 업무에 남의 건이 보인다")
	}
	if strings.Contains(body, "취소된통합업무") || strings.Contains(body, "내일행정") || strings.Contains(body, "행정미정") {
		t.Fatal("내일 이후·미정·취소 건이 올라갔다")
	}
	list := workTodayGet(t, e, "/work", jwtCookie(t))
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "D+") {
		t.Fatal("리스트에 지연 뱃지가 없다")
	}
}

func TestWorkTodayTechMineNoTeamToggle(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	tech := jwtCookieRole(t, "tech")
	mine := workTodayGet(t, e, "/work?display=kanban", tech)
	if mine.Code != http.StatusOK {
		t.Fatalf("status=%d", mine.Code)
	}
	body := mine.Body.String()
	if !strings.Contains(body, "내 업무") || !strings.Contains(body, "기술보류건") {
		t.Fatal("기술담당 기본이 본인 건이 아니다")
	}
	if strings.Contains(body, "관리자할일") || strings.Contains(body, "다른담당진행") {
		t.Fatal("기술담당 기본에 다른 사람 건이 보인다")
	}
	if strings.Contains(body, "팀 전체 보기") || strings.Contains(body, "내 배정만") {
		t.Fatal("오늘 내 업무에 범위 토글이 남았다")
	}

	ignored := workTodayGet(t, e, "/work?mine=0", tech)
	if ignored.Code != http.StatusOK {
		t.Fatalf("mine0 status=%d", ignored.Code)
	}
	tbody := ignored.Body.String()
	if strings.Contains(tbody, "관리자할일") || strings.Contains(tbody, "다른담당진행") {
		t.Fatal("mine=0 으로 남의 업무가 다시 열린다")
	}
}

func TestWorkTodayViewKeepsFilters(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?view=list&mine=0", jwtCookieRole(t, "tech"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "종료일") || !strings.Contains(body, "기술보류건") {
		t.Fatal("리스트 보기가 아니다")
	}
	if strings.Contains(body, "mine=0") {
		t.Fatal("칸반 링크에 mine 이 남았다")
	}
	if !strings.Contains(body, "display=kanban") {
		t.Fatal("칸반 링크에 display 가 유지되지 않는다")
	}
}

func TestWorkTodayTechIgnoresAssigneeFilter(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?display=kanban&assignee="+url.QueryEscape("다른담당"), jwtCookieRole(t, "tech"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "기술보류건") {
		t.Fatal("기술담당 본인 건이 빠졌다")
	}
	if strings.Contains(body, "다른담당진행") || strings.Contains(body, "관리자할일") {
		t.Fatal("기술담당이 assignee 로 남의 건을 연다")
	}
	if strings.Contains(body, `id="work-assignee"`) {
		t.Fatal("기술담당에게 담당자 드롭다운이 있다")
	}
}

func TestWorkTodayAdminAssigneePicker(t *testing.T) {
	e, repo := newWorkTodayServer(t)
	admin := jwtCookie(t)

	mine := workTodayGet(t, e, "/work", admin)
	if mine.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", mine.Code, mine.Body.String())
	}
	mb := mine.Body.String()
	sel := workAssigneeSelectHTML(mb)
	if sel == "" {
		t.Fatal("관리자에게 담당자 드롭다운이 없다")
	}
	if !strings.Contains(sel, `value="관리자"`) || !strings.Contains(sel, "selected") {
		t.Fatal("관리자 드롭다운 기본값이 본인이 아니다")
	}
	if strings.Contains(sel, `value="all"`) || strings.Contains(sel, ">전체<") || strings.Contains(sel, "팀전체") {
		t.Fatal("드롭다운에 전체가 있다")
	}
	if strings.Contains(mb, "다른담당진행") {
		t.Fatal("기본값인데 남의 건이 보인다")
	}

	office := workTodayGet(t, e, "/work", jwtCookieRole(t, "office"))
	if office.Code != http.StatusOK {
		t.Fatalf("office status=%d", office.Code)
	}
	if !strings.Contains(office.Body.String(), `id="work-assignee"`) {
		t.Fatal("접수담당에게 담당자 드롭다운이 없다")
	}

	tech := workTodayGet(t, e, "/work", jwtCookieRole(t, "tech"))
	if tech.Code != http.StatusOK {
		t.Fatalf("tech status=%d", tech.Code)
	}
	tb := tech.Body.String()
	if strings.Contains(tb, `id="work-assignee"`) {
		t.Fatal("기술담당에게 담당자 드롭다운이 있다")
	}
	if strings.Contains(tb, "오늘 · 다른담당 업무") {
		t.Fatal("기술담당 제목이 남을 본다")
	}

	sales := workTodayGet(t, e, "/work", jwtCookieRole(t, "sales"))
	if sales.Code != http.StatusOK {
		t.Fatalf("sales status=%d", sales.Code)
	}
	if strings.Contains(sales.Body.String(), `id="work-assignee"`) {
		t.Fatal("영업담당에게 담당자 드롭다운이 있다")
	}
	obs := workTodayGet(t, e, "/work", jwtCookieRole(t, "observer"))
	if obs.Code != http.StatusOK {
		t.Fatalf("observer status=%d", obs.Code)
	}
	if strings.Contains(obs.Body.String(), `id="work-assignee"`) {
		t.Fatal("관찰자에게 담당자 드롭다운이 있다")
	}

	other := workTodayGet(t, e, "/work?assignee="+url.QueryEscape("다른담당"), admin)
	if other.Code != http.StatusOK {
		t.Fatalf("other status=%d body=%s", other.Code, other.Body.String())
	}
	ob := other.Body.String()
	if !strings.Contains(ob, "오늘 · 다른담당 업무") {
		t.Fatal("남을 골랐는데 제목이 안 바뀐다")
	}
	if strings.Contains(ob, "관리자할일") {
		t.Fatal("남을 골랐는데 본인 건이 남았다")
	}
	if !strings.Contains(ob, "다른담당진행") {
		t.Fatal("남을 골랐는데 그 사람 건이 없다")
	}
	if !strings.Contains(ob, "sort=assignee") {
		t.Fatal("남을 골랐는데 담당자 열이 없다")
	}
	if strings.Contains(mb, "sort=assignee") {
		t.Fatal("본인 보기인데 담당자 열이 있다")
	}
	if !strings.Contains(ob, `title="읽기 전용"`) || !strings.Contains(ob, "🔒") {
		t.Fatal("남의 업무에 자물쇠가 없다")
	}
	if !strings.Contains(ob, "assignee=") {
		t.Fatal("칸반·리스트 링크에 assignee 가 유지되지 않는다")
	}

	allTok := workTodayGet(t, e, "/work?assignee=all", admin)
	if allTok.Code != http.StatusOK {
		t.Fatalf("all status=%d", allTok.Code)
	}
	ab := allTok.Body.String()
	if strings.Contains(ab, "다른담당진행") {
		t.Fatal("assignee=all 이 팀 전체를 연다")
	}
	if !strings.Contains(ab, "관리자할일") {
		t.Fatal("assignee=all 이 본인으로 안 돌아온다")
	}

	id := findWorkTaskIDByTitle(t, repo, "관리자할일")
	post := workFormCookie(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"status": {"in_progress"},
	}, jwtCookieRole(t, "tech"))
	if post.Code != http.StatusForbidden {
		t.Fatalf("남의 업무 POST 가 403 이 아니다 status=%d loc=%s", post.Code, post.Header().Get("Location"))
	}
}

func workAssigneeSelectHTML(body string) string {
	start := strings.Index(body, `id="work-assignee"`)
	if start < 0 {
		return ""
	}
	rest := body[start:]
	end := strings.Index(strings.ToLower(rest), "</select>")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func TestWorkTodayHoldSameColumnAsAdminKanban(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	ck := jwtCookie(t)
	admin := workTodayGet(t, e, "/admin-work?view=kanban", ck)
	all := workTodayGet(t, e, "/work/all?display=kanban", ck)
	if admin.Code != http.StatusOK || all.Code != http.StatusOK {
		t.Fatalf("admin=%d all=%d", admin.Code, all.Code)
	}
	if !strings.Contains(admin.Body.String(), "기술보류건") || !strings.Contains(admin.Body.String(), "보류") {
		t.Fatal("행정 칸반 진행중에 보류 건이 없다")
	}
	if !strings.Contains(all.Body.String(), "기술보류건") || !strings.Contains(all.Body.String(), "보류") {
		t.Fatal("전체 업무 진행중에 보류 건이 없다")
	}
}

func TestWorkTodayRoleUnresolvedBanner(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-role.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/work")
	if err := h.Work.List(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "권한 정보를 불러오지 못했습니다") {
		t.Fatal("역할 미확정 배너가 없다")
	}
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(body, today+" · 팀 전체 · 오늘 예정") {
		t.Fatal("역할 미확정 때 범위 한 줄이 팀 전체가 아니다")
	}
}

func TestWorkTodayDefaultIsListAndKanbanCountsMatch(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	ck := jwtCookie(t)
	list := workTodayGet(t, e, "/work", ck)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "종료일") || strings.Contains(lb, "min-w-[260px]") {
		t.Fatal("기본이 리스트가 아니다")
	}
	if !strings.Contains(lb, "display=kanban") {
		t.Fatal("칸반 전환에 display 가 없다")
	}
	kanban := workTodayGet(t, e, "/work?display=kanban", ck)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	kb := kanban.Body.String()
	if !strings.Contains(kb, "min-w-[260px]") || !strings.Contains(kb, "진행 예정") {
		t.Fatal("공통 칸반 열이 없다")
	}
	titles := []string{"관리자할일", "관리자완료", "어제지연행정"}
	for _, title := range titles {
		if !strings.Contains(lb, title) || !strings.Contains(kb, title) {
			t.Fatalf("리스트·칸반 건수가 어긋남: %s", title)
		}
	}
	if strings.Contains(lb, "다른담당진행") || strings.Contains(kb, "다른담당진행") {
		t.Fatal("오늘 내 업무에 남의 건이 보인다")
	}
	if strings.Contains(lb, "취소된통합업무") || strings.Contains(kb, "취소된통합업무") {
		t.Fatal("취소 건이 올라갔다")
	}
	if strings.Contains(lb, "내일행정") || strings.Contains(kb, "내일행정") {
		t.Fatal("내일 이후 건이 오늘 내 업무에 있다")
	}
	if strings.Contains(lb, "행정미정") || strings.Contains(kb, "행정미정") {
		t.Fatal("예정일 없는 건이 오늘 내 업무에 있다")
	}
}

func TestWorkTodayScopeNoteAndBuckets(t *testing.T) {
	got := workTodayScopeNote("2026-09-07", false, "", 3, 2, 1, 4)
	if got != "2026-09-07 · 내 업무 · 오늘 예정 3 · 진행중 2 · 지연 1 · 오늘 완료 4" {
		t.Fatalf("%s", got)
	}
	other := workTodayScopeNote("2026-09-07", false, "홍길동", 1, 0, 0, 0)
	if other != "2026-09-07 · 홍길동 업무 · 오늘 예정 1 · 진행중 0 · 지연 0 · 오늘 완료 0" {
		t.Fatalf("%s", other)
	}
	items := []model.WorkListItem{
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-07"},
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-06", DaysOverdue: 1},
		{MappedStatus: model.WBTaskComplete, CompleteDate: "2026-09-07"},
	}
	s, p, d, c := countWorkTodayBuckets(items, "2026-09-07")
	if s != 1 || p != 0 || d != 1 || c != 1 {
		t.Fatalf("sched=%d progress=%d delayed=%d done=%d", s, p, d, c)
	}
	waiting := []model.WorkListItem{
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-06", DaysOverdue: 6, BlockedReason: "부품 대기"},
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-06", DaysOverdue: 6},
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-01", DaysOverdue: 6},
		{MappedStatus: model.WBTaskWaiting, ScheduledDate: "2026-09-01", DaysOverdue: 6},
	}
	_, _, delayed, _ := countWorkTodayBuckets(waiting, "2026-09-07")
	if delayed != 3 {
		t.Fatalf("blocked still delayed=%d", delayed)
	}
	if avg := avgOverdueDays(waiting, "2026-09-07"); avg != 6 {
		t.Fatalf("avg=%d", avg)
	}
	if got := workTodayDelayWarn(3, 6); got != "3건이 평균 6일 밀려 있습니다" {
		t.Fatalf("%s", got)
	}
}

func TestWorkTodayUndatedGoesToUnplanned(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	ck := jwtCookie(t)
	today := workTodayGet(t, e, "/work", ck)
	if today.Code != http.StatusOK {
		t.Fatalf("work status=%d", today.Code)
	}
	if strings.Contains(today.Body.String(), "행정미정") {
		t.Fatal("예정일 없는 건이 오늘 내 업무에 있다")
	}
	up := workTodayGet(t, e, "/plan/unplanned", ck)
	if up.Code != http.StatusOK {
		t.Fatalf("unplanned status=%d", up.Code)
	}
	if !strings.Contains(up.Body.String(), "행정미정") {
		t.Fatal("미계획 업무함에 예정일 없는 건이 없다")
	}
}

func TestWorkAllStillShowsOutsideToday(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	all := workTodayGet(t, e, "/work/all?from=2026-01-01&to="+tomorrow, jwtCookie(t))
	if all.Code != http.StatusOK {
		t.Fatalf("status=%d", all.Code)
	}
	body := all.Body.String()
	if !strings.Contains(body, "내일행정") {
		t.Fatal("전체 업무에서 내일 건이 빠졌다")
	}
	if !strings.Contains(body, "어제지연행정") {
		t.Fatal("전체 업무에서 지연 건이 빠졌다")
	}
}
