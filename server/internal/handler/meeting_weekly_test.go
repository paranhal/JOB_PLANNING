package handler

import (
	"database/sql"
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

func newMeetingWeeklyServer(t *testing.T) (*echo.Echo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "meeting_weekly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true})
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	g.GET("/workboard/register", h.Workboard.Register)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	return e, db
}

func weeklyGet(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestMeetingWeeklyHTTP_OKAndReadOnly(t *testing.T) {
	e, _ := newMeetingWeeklyServer(t)
	rec := weeklyGet(t, e, "/meeting/weekly", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()[:minInt(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	if !strings.Contains(body, "주간 업무 회의") || !strings.Contains(body, "/stats/reports") {
		t.Fatal("제목 또는 주간보고 링크 없음")
	}
	if meetingWeeklyHasPost(body) {
		t.Fatal("주간 회의는 읽기 전용인데 POST 가 있다")
	}
}

func TestMeetingWeeklyEmptyWeek(t *testing.T) {
	e, _ := newMeetingWeeklyServer(t)
	rec := weeklyGet(t, e, "/meeting/weekly?date=2026-02-02", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "이번 주 예정이 없습니다") && !strings.Contains(body, "지난주 완료가 없습니다") {
		t.Fatal("빈 주 안내가 없다")
	}
	if !strings.Contains(body, `id="weekly-done-n"`) || !strings.Contains(body, ">0</p>") {
		t.Fatal("빈 주 요약 0 이 없다")
	}
}

func TestMeetingWeeklyMondayAndYearBoundary(t *testing.T) {
	e, _ := newMeetingWeeklyServer(t)
	// 2026-12-31 목요일 → 그 주 월 12-28, 금 2027-01-01
	rec := weeklyGet(t, e, "/meeting/weekly?date=2026-12-31", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "2026-12-28") || !strings.Contains(body, "2027-01-01") {
		t.Fatal("연말 주 월~금이 아니다")
	}
	if !strings.Contains(body, "date=2026-12-21") {
		t.Fatal("이전 주가 월요일 시작이 아니다")
	}
	sun := weeklyGet(t, e, "/meeting/weekly?date=2026-10-18", jwtCookie(t))
	if !strings.Contains(sun.Body.String(), "2026-10-12") {
		t.Fatal("일요일 기준인데 그 주 월요일이 아니다")
	}
}

func TestMeetingWeeklyAssigneeScopeAndSums(t *testing.T) {
	e, db := newMeetingWeeklyServer(t)
	weekMon := time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local) // 월요일
	thisWed := weekMon.AddDate(0, 0, 2).Format("2006-01-02")
	lastWed := weekMon.AddDate(0, 0, -5).Format("2006-01-02")
	overdue := weekMon.AddDate(0, 0, -20).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','주간도서관','주간도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-d','R2610-D01','c1','2026-10-01',?,1,'completed','양기헌',?,'지난주완료',?),
		('as-p','R2610-P01','c1','2026-10-05',?,1,'in_progress','양기헌',NULL,'이번주예정',?),
		('as-o','R2610-O01','c1','2026-09-01',?,1,'in_progress','양기헌',NULL,'기한경과',?),
		('as-c','R2610-C01','c1','2026-10-05',?,1,'in_progress','최혜영',NULL,'다른담당',?)`,
		lastWed, lastWed+" 16:00:00", lastWed+" 16:00:00",
		thisWed, thisWed+" 09:00:00",
		overdue, overdue+" 09:00:00",
		thisWed, thisWed+" 09:00:00"); err != nil {
		t.Fatal(err)
	}

	all := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&assignee=all", jwtCookie(t))
	if all.Code != http.StatusOK {
		t.Fatalf("status=%d %s", all.Code, all.Body.String()[:minInt(300, all.Body.Len())])
	}
	body := all.Body.String()
	if !strings.Contains(body, "R2610-D01") || !strings.Contains(body, "R2610-P01") || !strings.Contains(body, "R2610-O01") {
		t.Fatal("팀전체에서 완료·예정·경과가 빠졌다")
	}
	if !strings.Contains(body, "R2610-C01") {
		t.Fatal("팀전체에 다른 담당 예정이 없다")
	}

	mine := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&assignee="+url.QueryEscape("양기헌"), jwtCookie(t))
	mb := mine.Body.String()
	if strings.Contains(mb, "R2610-C01") {
		t.Fatal("담당자 필터에 최혜영 건이 보인다")
	}
	if !strings.Contains(mb, "R2610-P01") {
		t.Fatal("양기헌 예정이 없다")
	}

	items := []model.WorkListItem{
		{Prefix: "AS", RefID: "1", Assignee: "양기헌"},
		{Prefix: "AS", RefID: "2", Assignee: "양기헌"},
		{Prefix: "AS", RefID: "3", Assignee: "최혜영"},
	}
	g := groupWorkItemsByAssignee(items, []string{"양기헌", "최혜영"})
	if meetingGroupCount(g) != len(items) {
		t.Fatalf("묶음 합계=%d want %d", meetingGroupCount(g), len(items))
	}
}

func TestMeetingWeeklyLoginRequired(t *testing.T) {
	e, _ := newMeetingWeeklyServer(t)
	rec := weeklyGet(t, e, "/meeting/weekly", nil)
	if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther {
		t.Fatalf("비로그인 status=%d", rec.Code)
	}
}

func TestMeetingWeeklyGroupCountHelper(t *testing.T) {
	if meetingGroupCount(nil) != 0 {
		t.Fatal("빈 묶음")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
