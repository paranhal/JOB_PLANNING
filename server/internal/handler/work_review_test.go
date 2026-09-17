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

func newSelfReviewServer(t *testing.T) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "self-review.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "choi", PasswordHash: HashPassword("pw"), FullName: "최혜영",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewWBRepo(db)
	due := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	for _, title := range []string{"지연A", "지연B", "지연C"} {
		mustCreateWorkToday(t, repo, &model.WorkTask{
			WorkType: model.WBWorkAdmin, Title: title, Status: model.WBTaskWaiting,
			Assignee: "관리자", DueDate: due, WorkDate: due, CustomerName: "지연도서관",
		})
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/", h.Dashboard)
	g.POST("/dashboard/week-review/dismiss", h.DismissWeekReview)
	g.GET("/work", h.Work.List)
	g.POST("/work/review/reschedule", h.Work.ReviewReschedule)
	g.POST("/work/review/transfer", h.Work.ReviewTransfer)
	g.POST("/work/review/block", h.Work.ReviewBlock)
	return e, repo
}

func workTodayPost(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestWorkSelfReviewHeaderAverage(t *testing.T) {
	e, _ := newSelfReviewServer(t)
	rec := workTodayGet(t, e, "/work", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "3건이 평균 6일 밀려 있습니다") {
		t.Fatalf("평균 지연 한 줄이 없다: %s", snippet(body))
	}
	if !strings.Contains(body, "정리하기") {
		t.Fatal("[정리하기]가 없다")
	}
	if !strings.Contains(body, "내 업무") || !strings.Contains(body, "지연 3") {
		t.Fatal("헤더 건수가 없다")
	}
}

func TestWorkSelfReviewThreeForks(t *testing.T) {
	e, _ := newSelfReviewServer(t)
	rec := workTodayGet(t, e, "/work?review=1", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "지연 업무 정리") {
		t.Fatal("정리 화면 제목이 없다")
	}
	if strings.Contains(body, "관리자할일") {
		t.Fatal("오늘 예정 건이 정리 목록에 섞였다")
	}
	for _, title := range []string{"지연A", "지연B", "지연C"} {
		if !strings.Contains(body, title) {
			t.Fatalf("지연 건 %s 가 없다", title)
		}
	}
	for _, needle := range []string{"날짜 다시 잡기", "넘기기", "막힌 이유 적기", "오늘", "내일", "직접", "부품 대기", "고객 연락 두절"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("세 갈래 없음: %s", needle)
		}
	}
}

func TestWorkSelfReviewBlockedBecomesWaiting(t *testing.T) {
	e, repo := newSelfReviewServer(t)
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, tsk := range tasks {
		if tsk.Title == "지연A" {
			id = tsk.TaskID
			break
		}
	}
	if id == "" {
		t.Fatal("지연A task_id 없음")
	}
	form := url.Values{
		"prefix":         {model.WorkPrefixGeneral},
		"ref_id":         {id},
		"blocked_reason": {"부품 대기"},
	}
	post := workTodayPost(t, e, "/work/review/block", form, jwtCookie(t))
	if post.Code != http.StatusSeeOther {
		t.Fatalf("block status=%d body=%s", post.Code, post.Body.String())
	}
	rec := workTodayGet(t, e, "/work", jwtCookie(t))
	body := rec.Body.String()
	if !strings.Contains(body, "기다리는 중 · 부품 대기") {
		t.Fatal("기다리는 것 라벨이 없다")
	}
	if strings.Contains(body, "3건이 평균") {
		t.Fatal("막힌 건이 지연 건수에 남아 있다")
	}
	if !strings.Contains(body, "2건이 평균 6일 밀려 있습니다") {
		t.Fatalf("지연에서 빠지지 않았다: %s", snippet(body))
	}
	review := workTodayGet(t, e, "/work?review=1", jwtCookie(t))
	if strings.Contains(review.Body.String(), "지연A") {
		t.Fatal("기다리는 건이 정리하기 목록에 남아 있다")
	}
}

func TestDashboardMineFirstForAdmin(t *testing.T) {
	e, _ := newSelfReviewServer(t)
	rec := workTodayGet(t, e, "/", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "내 업무") {
		t.Fatal("관리자 대시보드에 내 업무가 없다")
	}
	idxMine := strings.Index(body, "<strong>내 업무</strong>")
	idxTeam := strings.Index(body, "팀 전체")
	if idxMine < 0 {
		t.Fatal("내 업무가 강조되지 않는다")
	}
	if idxTeam >= 0 && idxTeam < idxMine {
		t.Fatal("팀 전체가 내 업무보다 앞에 있다")
	}
}

func TestWeekReviewMondayLineAndDismiss(t *testing.T) {
	mon := time.Date(2026, 9, 14, 9, 0, 0, 0, time.Local)
	if !weekReviewShouldShow(mon, "") {
		t.Fatal("월요일에 한 줄이 안 뜬다")
	}
	if weekReviewShouldShow(mon, "2026-09-14") {
		t.Fatal("닫은 주에도 한 줄이 뜬다")
	}
	if weekReviewShouldShow(mon.AddDate(0, 0, 1), "") {
		t.Fatal("화요일에 한 줄이 뜬다")
	}
	line := model.WeekReview{Completed: 12, AvgLead: 2.4, AvgOK: true, OnTime: 10, Carried: 3}.Line()
	if line != "지난주  완료 12건 · 평균 2.4영업일 · 기한 내 10건 · 밀린 채 넘어온 것 3건" {
		t.Fatalf("%s", line)
	}

	e, _ := newSelfReviewServer(t)
	ck := jwtCookie(t)
	post := workTodayPost(t, e, "/dashboard/week-review/dismiss", url.Values{"back": {"/"}}, ck)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("dismiss status=%d", post.Code)
	}
	var dismissed *http.Cookie
	for _, c := range post.Result().Cookies() {
		if c.Name == weekReviewCookie {
			dismissed = c
			break
		}
	}
	if dismissed == nil || dismissed.Value != model.CalendarMonday(time.Now()).Format("2006-01-02") {
		t.Fatalf("닫기 쿠키가 없다 %+v", dismissed)
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.AddCookie(ck)
	req.AddCookie(dismissed)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dash status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "지난주  완료") {
		t.Fatal("닫은 뒤에도 지난주 한 줄이 있다")
	}
	if time.Now().Weekday() == time.Monday {
		open := workTodayGet(t, e, "/", ck)
		if !strings.Contains(open.Body.String(), "지난주  완료") {
			t.Fatal("월요일인데 지난주 한 줄이 없다")
		}
	}
}

func snippet(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}
