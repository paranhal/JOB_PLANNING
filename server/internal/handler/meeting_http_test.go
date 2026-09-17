package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestMeetingHTTP_YesterdayDoneTodayScheduled(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true})
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})

	anchor := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local)
	today := anchor.Format("2006-01-02")
	yesterday := anchor.AddDate(0, 0, -1).Format("2006-01-02")

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-y','R2608-Y01','c1','2026-08-01',?,1,'completed','양기헌',?,'어제완료',?),
		('as-t','R2608-T01','c1','2026-08-05',?,1,'in_progress','양기헌',NULL,'오늘예정',?),
		('as-c','R2608-C01','c1','2026-08-05',?,1,'in_progress','최혜영',NULL,'다른담당',?)`,
		yesterday, yesterday+" 16:00:00", yesterday+" 16:00:00",
		today, today+" 09:00:00",
		today, today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp1', 2026, '계획', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (
		visit_id, plan_id, visit_date, customer_id, sort_order, completed, product_type
	) VALUES ('mv1','mp1',?, 'c1',1,0,'앤로보틱스')`, today); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 400 {
			snippet = snippet[:400]
		}
		t.Fatalf("status=%d body=%s", rec.Code, snippet)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"일일 업무 회의", "전일 실적", "오늘 예정",
		"R2608-Y01", "R2608-T01", "회의도서관",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body", want)
		}
	}
	if !strings.Contains(body, "/as/as-y") || !strings.Contains(body, "/as/as-t") {
		t.Fatal("AS 상세 링크 없음")
	}
	if !strings.Contains(body, "/maintenance/mp1") {
		t.Fatal("정기점검 링크 없음")
	}
	if !strings.Contains(body, "팀전체") || !strings.Contains(body, "양기헌") {
		t.Fatal("담당자 드롭다운이 없다")
	}
	if strings.Contains(body, "팀전체 보기") || strings.Contains(body, "내 업무만") {
		t.Fatal("내 것/전체 토글이 남아 있다")
	}
	if !strings.Contains(body, "양기헌 · ") || !strings.Contains(body, "최혜영 · ") {
		t.Fatal("담당자 소제목 줄이 없다")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("양기헌"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter status=%d", rec.Code)
	}
	filtered := rec.Body.String()
	if !strings.Contains(filtered, "R2608-T01") {
		t.Fatal("양기헌 예정이 없다")
	}
	if strings.Contains(filtered, "R2608-C01") {
		t.Fatal("다른 담당자 건이 필터에 남았다")
	}
	if !strings.Contains(filtered, "assignee=") {
		t.Fatal("날짜 이동 링크에 담당자 선택이 없다")
	}
	if !strings.Contains(filtered, "양기헌 기준") {
		t.Fatal("상단 범위 안내가 담당자 기준이 아니다")
	}

	techTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "yang-id", "username": "yang", "role": "tech",
		"name": "양기헌", "exp": time.Now().Add(time.Hour).Unix(),
	})
	techStr, err := techTok.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today, nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: techStr, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tech status=%d", rec.Code)
	}
	techBody := rec.Body.String()
	if !strings.Contains(techBody, "양기헌 기준") {
		t.Fatal("기술담당 기본이 본인 기준이 아니다")
	}
	if strings.Contains(techBody, "R2608-C01") {
		t.Fatal("기술담당 기본 보기에 다른 사람 건이 있다")
	}
	if !strings.Contains(techBody, `option value="all"`) {
		t.Fatal("기술담당도 팀전체를 고를 수 있어야 한다")
	}
}

func TestMeetingKanbanFiveColumnsNoDragKeepsFilters(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_kanban.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true})
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})

	anchor := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local)
	today := anchor.Format("2006-01-02")
	yesterday := anchor.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-y','R2608-Y01','c1','2026-08-01',?,1,'completed','양기헌',?,'어제완료',?),
		('as-t','R2608-T01','c1','2026-08-05',?,1,'in_progress','양기헌',NULL,'오늘진행',?),
		('as-w','R2608-W01','c1','2026-08-05',?,1,'received','양기헌',NULL,'오늘대기',?),
		('as-c','R2608-C01','c1','2026-08-05',?,1,'in_progress','최혜영',NULL,'다른담당',?)`,
		yesterday, yesterday+" 16:00:00", yesterday+" 16:00:00",
		today, today+" 09:00:00",
		today, today+" 09:00:00",
		today, today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("양기헌"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "전일 실적") || !strings.Contains(lb, "display=kanban") {
		t.Fatal("리스트에 칸반 전환이 없다")
	}
	if !strings.Contains(lb, "date="+today) || !strings.Contains(lb, "assignee=") {
		t.Fatal("칸반 링크에 날짜·담당자가 없다")
	}

	kanban := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("양기헌")+"&display=kanban", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(kanban, req)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d %s", kanban.Code, kanban.Body.String()[:min(300, kanban.Body.Len())])
	}
	kb := kanban.Body.String()
	for _, want := range []string{"전일 완료", "오늘 완료", "오늘 예정", "진행중", "미계획", "min-w-[260px]", "0건"} {
		if !strings.Contains(kb, want) {
			t.Fatalf("칸반에 %q 없음", want)
		}
	}
	if strings.Contains(kb, `draggable="true"`) {
		t.Fatal("회의 칸반에서 드래그가 된다")
	}
	if !strings.Contains(kb, "R2608-Y01") || !strings.Contains(kb, "R2608-T01") || !strings.Contains(kb, "R2608-W01") {
		t.Fatal("양기헌 카드가 빠졌다")
	}
	if strings.Contains(kb, "R2608-C01") {
		t.Fatal("다른 담당자 건이 칸반에 남았다")
	}
	if n := strings.Count(kb, `data-ref="as-t"`); n != 1 {
		t.Fatalf("같은 진행 건이 칸반에 %d번 나타난다", n)
	}
	if n := strings.Count(kb, `data-ref="as-y"`); n != 1 {
		t.Fatalf("전일 완료가 칸반에 %d번", n)
	}
	if !strings.Contains(kb, "display=list") || !strings.Contains(kb, "date="+today) || !strings.Contains(kb, "assignee=") {
		t.Fatal("리스트 전환에 날짜·담당자가 풀렸다")
	}
	if !strings.Contains(kb, "칸반 ") || !strings.Contains(kb, "건") {
		t.Fatal("칸반 건수가 없다")
	}
}

func TestMeetingTodayCompleteFillsKanbanAndSummaryIdentity(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_today_done.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})

	today := "2026-09-07"
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("as-c%d", i)
		num := fmt.Sprintf("R2609-C%02d", i)
		if _, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
		) VALUES (?,?,'c1','2026-09-01',?,1,'completed','최혜영',?,?,?)`,
			id, num, today, today+" 15:00:00", "당일완료", today+" 15:00:00"); err != nil {
			t.Fatal(err)
		}
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("최혜영"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d %s", list.Code, list.Body.String()[:min(400, list.Body.Len())])
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "예정 4건") {
		t.Fatal("요약 뱃지 예정 4건이 없다")
	}
	if strings.Contains(lb, "예정 0건") {
		t.Fatal("뱃지와 큰 숫자가 다른 값이다")
	}
	if !strings.Contains(lb, "예정 4") {
		t.Fatal("요약 큰 숫자 예정이 없다")
	}
	for i := 1; i <= 4; i++ {
		num := fmt.Sprintf("R2609-C%02d", i)
		if !strings.Contains(lb, num) {
			t.Fatalf("리스트에 %s 없음", num)
		}
	}

	kanban := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("최혜영")+"&display=kanban", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(kanban, req)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	kb := kanban.Body.String()
	if !strings.Contains(kb, "오늘 완료") {
		t.Fatal("오늘 완료 열이 없다")
	}
	if !strings.Contains(kb, "예정 4건") || strings.Contains(kb, "예정 0건") {
		t.Fatal("칸반 요약 뱃지가 큰 숫자와 다르다")
	}
	if n := strings.Count(kb, `data-col="completed_today"`); n != 5 {
		t.Fatalf("오늘 완료 열+카드 %d want 5 (열1+카드4)", n)
	}
	for i := 1; i <= 4; i++ {
		href := fmt.Sprintf("/as/as-c%d", i)
		ref := fmt.Sprintf(`data-ref="as-c%d"`, i)
		if strings.Count(kb, href) < 1 {
			t.Fatalf("%s 링크가 없다", href)
		}
		if strings.Count(kb, ref) != 1 {
			t.Fatalf("%s 가 칸반에 %d번", ref, strings.Count(kb, ref))
		}
	}
}

func TestMeetingUnconfirmedAppearsInStatsAndList(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_unconf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	users := repository.NewUserRepo(db)
	_ = users.Create(&model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true})

	today := "2026-09-07"
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES ('as-u','R2609-U01','c1','2026-09-01',?,0,'assigned','최혜영',NULL,'미확정',?)`,
		today, today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("최혜영"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "R2609-U01") {
		t.Fatal("미확정 건이 목록에 없다")
	}
	if !strings.Contains(lb, "예정 1건") {
		t.Fatal("통계 예정이 목록과 다르다")
	}
	if !strings.Contains(lb, "미확정") {
		t.Fatal("미확정 뱃지가 없다")
	}

	kanban := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("최혜영")+"&display=kanban", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(kanban, req)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	kb := kanban.Body.String()
	if !strings.Contains(kb, "R2609-U01") {
		t.Fatal("미확정 건이 칸반에 없다")
	}
	if !strings.Contains(kb, "미확정") {
		t.Fatal("칸반 미확정 뱃지가 없다")
	}
}

func TestMeetingProgressExcludingScheduledKeepsIdentity(t *testing.T) {
	date := "2026-09-07"
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixAS, RefID: "today", ScheduledDate: date, Status: "in_progress"},
		{Prefix: model.WorkPrefixAS, RefID: "other", ScheduledDate: "2026-09-01", Status: "in_progress"},
	}
	got := meetingProgressExcludingScheduled(items, date)
	if len(got) != 1 || got[0].RefID != "other" {
		t.Fatalf("오늘 예정 진행중을 빼야 한다: %+v", got)
	}
}

func TestMeetingRendersListAndKanbanWithoutRefetch(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "meeting_toggle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	today := "2026-09-07"
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','회의도서관','회의도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES ('as-t','R2609-T01','c1','2026-09-01',?,1,'in_progress','최혜영',NULL,'오늘예정',?)`,
		today, today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/meeting/overview", h.Meeting.Overview)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting?date="+today+"&assignee="+url.QueryEscape("최혜영"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="meeting-list"`) || !strings.Contains(body, `id="meeting-kanban"`) {
		t.Fatal("리스트와 칸반을 한 번에 그리지 않았다")
	}
	if !strings.Contains(body, `data-meeting-display="kanban"`) || !strings.Contains(body, "preventDefault") {
		t.Fatal("화면 전환 스크립트가 없다")
	}
	if !strings.Contains(body, "/meeting/overview?") || !strings.Contains(body, `hx-trigger="load"`) {
		t.Fatal("요약 카드 지연 로드가 없다")
	}
	if strings.Contains(body, "text-emerald-700") {
		t.Fatal("목록 첫 페인트에 통계 처리 숫자가 있다")
	}
	if !strings.Contains(body, "전일 실적") || strings.Count(body, `data-col="`) < 5 {
		t.Fatal("리스트와 칸반 두 벌이 비었다")
	}

	ov := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "http://localhost/meeting/overview?date="+today+"&assignee="+url.QueryEscape("최혜영")+"&yesterday_n=0&planned_n=1&unplanned_n=0", nil)
	oreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(ov, oreq)
	if ov.Code != http.StatusOK {
		t.Fatalf("overview %d %s", ov.Code, ov.Body.String()[:min(300, ov.Body.Len())])
	}
	ob := ov.Body.String()
	if !strings.Contains(ob, "처리 ") || !strings.Contains(ob, "예정 1건") {
		t.Fatal("요약 카드가 처리·예정을 채우지 않았다")
	}
	if strings.Contains(ob, "전일 실적") {
		t.Fatal("요약 요청이 목록까지 다시 그렸다")
	}
}

func TestDashboardDoesNotCallFillPeriodOverview(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("internal", "handler", "handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, "func (h *Handler) Dashboard")
	if i < 0 {
		t.Fatal("Dashboard 를 못 찾았다")
	}
	j := strings.Index(text[i:], "\nfunc ")
	if j < 0 {
		j = len(text) - i
	}
	fn := text[i : i+j]
	if strings.Contains(fn, ".FillPeriodOverview(") {
		t.Fatal("대시보드가 FillPeriodOverview 를 호출하면 로그인 직후가 멈춘다")
	}
	if !strings.Contains(fn, "DashHome") {
		t.Fatal("대시보드가 목록을 한 번에 읽지 않는다")
	}
}

func TestMeetingShowDoesNotCallFillPeriodOverview(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("internal", "handler", "meeting.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, "func (h *MeetingHandler) Show")
	j := strings.Index(text, "func (h *MeetingHandler) Overview")
	if i < 0 || j < 0 || j <= i {
		t.Fatal("Show/Overview 를 못 찾았다")
	}
	show := text[i:j]
	if strings.Contains(show, "h.stats.FillPeriodOverview") {
		t.Fatal("Show 가 FillPeriodOverview 를 호출한다")
	}
	if !strings.Contains(text, "func (h *MeetingHandler) Overview") {
		t.Fatal("Overview 핸들러가 없다")
	}
}
