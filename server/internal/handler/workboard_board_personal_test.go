package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	sqlite "modernc.org/sqlite"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const sqliteQCount9520 = "sqlite-qcount-95-20"

var (
	qcount9520Once sync.Once
	qcount9520N    atomic.Pointer[int64]
)

type qcount9520Driver struct{ sqlite.Driver }

func (d *qcount9520Driver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return qcount9520Conn{Conn: c}, nil
}

type qcount9520Conn struct{ driver.Conn }

func bumpQ9520() {
	if p := qcount9520N.Load(); p != nil {
		atomic.AddInt64(p, 1)
	}
}

func (c qcount9520Conn) Prepare(query string) (driver.Stmt, error) {
	bumpQ9520()
	return c.Conn.Prepare(query)
}

func (c qcount9520Conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	bumpQ9520()
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func (c qcount9520Conn) Begin() (driver.Tx, error) { return c.Conn.Begin() }
func (c qcount9520Conn) Close() error              { return c.Conn.Close() }

func (c qcount9520Conn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c qcount9520Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.Conn.Begin()
}

func seedPersonalBoard(t *testing.T, driver, path string) (*echo.Echo, *sql.DB, *model.User, *model.User, *repository.WBRepo) {
	t.Helper()
	qcount9520Once.Do(func() { sql.Register(sqliteQCount9520, &qcount9520Driver{}) })
	var db *sql.DB
	var err error
	if driver != "" && driver != "sqlite" {
		db, err = repository.InitDBDriver(driver, path)
	} else {
		db, err = repository.InitDB(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','미디어사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	u1 := &model.User{Username: "kimhw", PasswordHash: HashPassword("password10"), FullName: "김현우", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u2 := &model.User{Username: "leeo2", PasswordHash: HashPassword("password10"), FullName: "이다른", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	oa := &model.User{Username: "oa01", PasswordHash: HashPassword("password10"), FullName: "도서관관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{u1, u2, oa} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	got1, _ := users.GetByUsername("kimhw")
	got2, _ := users.GetByUsername("leeo2")
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.GuardCLevelHidden)
	g.GET("/workboard", h.Workboard.Index)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/workboard/projects/other", h.Workboard.ListOtherProjects)
	g.GET("/workboard/projects/picker-other", h.Workboard.ListPickerOtherProjects)
	g.GET("/workboard/projects/for-site", h.Workboard.ListProjectsForSite)
	g.POST("/workboard/tasks/:id/match-project", h.Workboard.MatchTaskProject)
	g.GET("/workboard/new-work/form", h.Workboard.NewWorkForm)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/workboard/new-work/as-cards", h.Workboard.NewWorkASCardsJSON)
	g.GET("/workboard/register", h.Workboard.Register)
	g.POST("/workboard/mnt-quick-register", h.Workboard.QuickRegisterMaintenance)
	g.GET("/workboard/mnt-candidates", h.Workboard.MntCandidates)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.POST("/workboard/schedule", h.Workboard.Schedule)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/workboard/kanban", h.Workboard.Kanban)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.POST("/workboard/tasks/:id/merge-interrupt", h.Workboard.MergeInterrupt)
	g.POST("/workboard/tasks/:id/return-confirm", h.Workboard.ConfirmReturn)
	mountLiveWorkRoutes(g, h.Workboard)
	g.GET("/work", h.Work.List)
	g.GET("/work/all", h.Work.ListAll)
	return e, db, got1, got2, h.Workboard.repo
}

func mustCreateBoardTask(t *testing.T, repo *repository.WBRepo, title, date, start, end, org, assignee, uid, status, workType string) {
	t.Helper()
	tk := &model.WorkTask{
		Title: title, WorkDate: date, DueDate: date, StartTime: start, EndTime: end,
		OrgID: org, Assignee: assignee, AssigneeUserID: uid, Status: status,
		WorkType: workType, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
}

func TestPersonalBoardDefaultListMineOnly(t *testing.T) {
	e, _, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b1.db"))
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "내업무-표시", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "동료업무-숨김", today, "11:00", "12:00", model.OrgIDLibrary, other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	rec := doGetWith(t, e, "/workboard/board", jwtCookieUser(t, me))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-summary-card="today"`) {
		t.Fatal("기본 화면이 요약이 아니다")
	}
	if strings.Contains(body, "동료업무-숨김") {
		t.Fatal("타 담당자 업무가 보인다")
	}
	day := doGetWith(t, e, "/workboard/board?view=day", jwtCookieUser(t, me))
	db := day.Body.String()
	if !strings.Contains(db, "내업무-표시") {
		t.Fatal("본인 업무 없음")
	}
	if strings.Contains(db, "동료업무-숨김") {
		t.Fatal("타 담당자 업무가 일정표에 보인다")
	}
}

func TestPersonalBoardOrgIsolationAndOrgAllMineOnly(t *testing.T) {
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b2.db"))
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "O01내업무", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "O02타업무", today, "09:00", "10:00", "O02", other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	rec := doGetWith(t, e, "/workboard/board?view=day", jwtCookieUser(t, me))
	body := rec.Body.String()
	if strings.Contains(body, "O02타업무") {
		t.Fatal("O01 사용자에게 O02 업무가 보인다")
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs1", PasswordHash: HashPassword("password10"), FullName: "비젼관리자", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	if err := users.Create(vs); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("vs1")
	mustCreateBoardTask(t, repo, "비전본인", today, "13:00", "14:00", "O02", got.FullName, got.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	page := doGetWith(t, e, "/workboard/board?view=day", jwtCookieUser(t, got))
	p := page.Body.String()
	if !strings.Contains(p, "비전본인") {
		t.Fatal("OrgAll 비전 본인 업무 없음")
	}
	if !strings.Contains(p, "O01내업무") || !strings.Contains(p, "O02타업무") {
		t.Fatal("비전 관리자 기본 전체에서 조직 업무가 안 보인다")
	}
	mine := doGetWith(t, e, "/workboard/board?view=day&assignee="+url.QueryEscape(got.FullName), jwtCookieUser(t, got))
	mb := mine.Body.String()
	if !strings.Contains(mb, "비전본인") {
		t.Fatal("비전 내 업무 필터에 본인 없음")
	}
	if strings.Contains(mb, "O01내업무") {
		t.Fatal("비전 내 업무 필터에 타인 업무가 보인다")
	}
}

func TestPersonalBoardCLevelAndLogin(t *testing.T) {
	e, db, _, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b3.db"))
	users := repository.NewUserRepo(db)
	ceo := &model.User{Username: "ceo1", PasswordHash: HashPassword("password10"), FullName: "대표", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	if err := users.Create(ceo); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("ceo1")
	rec := doGetWith(t, e, "/workboard/board", jwtCookieUser(t, got))
	if rec.Code != http.StatusOK {
		t.Fatalf("c_level 조회 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "조회 전용") {
		t.Fatal("C-Level 조회 전용 표시 없음")
	}
	anon := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/workboard/board", nil)
	e.ServeHTTP(anon, req)
	if anon.Code != http.StatusSeeOther && anon.Code != http.StatusFound {
		t.Fatalf("미로그인 %d", anon.Code)
	}
	if loc := anon.Header().Get("Location"); !strings.Contains(loc, "/login") {
		t.Fatalf("login loc=%s", loc)
	}
}

func TestPersonalBoardViewDateFallback(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b4.db"))
	ck := jwtCookieUser(t, me)
	for _, path := range []string{
		"/workboard/board?view=month",
		"/workboard/board?view=week",
		"/workboard/board?view=day",
		"/workboard/board?view=nope&date=not-a-date",
	} {
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s → %d", path, rec.Code)
		}
	}
	bad := doGetWith(t, e, "/workboard/board?view=nope", ck)
	if !strings.Contains(bad.Body.String(), `data-summary-card="today"`) {
		t.Fatal("잘못된 view 가 요약으로 안 떨어짐")
	}
}

func TestPersonalBoardMonthMoreAndOutsideMonth(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b5.db"))
	day := time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local)
	ds := day.Format(dateLayout)
	for i := 0; i < 5; i++ {
		mustCreateBoardTask(t, repo, fmt.Sprintf("월칩%d", i+1), ds, fmt.Sprintf("%02d:00", 8+i), fmt.Sprintf("%02d:30", 8+i),
			model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	rec := doGetWith(t, e, "/workboard/board?view=month&date="+ds, jwtCookieUser(t, me))
	body := rec.Body.String()
	if !strings.Contains(body, "+3건 더보기") {
		t.Fatalf("더보기 없음")
	}
	if !strings.Contains(body, "text-ink-subtle") {
		t.Fatal("앞뒤 달 흐린 숫자 클래스 없음")
	}
}

func TestPersonalBoardWeekStartsMonday(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b6.db"))
	rec := doGetWith(t, e, "/workboard/board?view=week&date=2026-10-24", jwtCookieUser(t, me))
	body := rec.Body.String()
	iMon := strings.Index(body, `data-dow>월<`)
	iSun := strings.Index(body, `data-dow>일<`)
	if iMon < 0 || iSun < 0 || iMon > iSun {
		t.Fatalf("주 화면 월~일 순서 아님 mon=%d sun=%d", iMon, iSun)
	}
}

func TestBoardStatusAndCancelSkip(t *testing.T) {
	today := "2026-10-24"
	done := boardStatus(model.WorkTask{Status: model.WBTaskComplete, WorkDate: "2026-10-20"}, today)
	if done.Key != "done" {
		t.Fatalf("완료=%s", done.Key)
	}
	prog := boardStatus(model.WorkTask{Status: model.WBTaskInProgress, WorkDate: today}, today)
	if prog.Key != "prog" {
		t.Fatalf("진행중=%s", prog.Key)
	}
	late := boardStatus(model.WorkTask{Status: model.WBTaskWaiting, WorkDate: "2026-10-01", DueDate: "2026-10-01"}, today)
	if late.Key != "late" {
		t.Fatalf("지연=%s", late.Key)
	}
	noDue := boardStatus(model.WorkTask{Status: model.WBTaskWaiting, WorkDate: "2026-10-01"}, today)
	if noDue.Key == "late" {
		t.Fatal("종료일 없음은 지연이 아님")
	}
	plan := boardStatus(model.WorkTask{Status: model.WBTaskWaiting, WorkDate: today}, today)
	if plan.Key != "plan" {
		t.Fatalf("예정=%s", plan.Key)
	}
	tAS := model.WorkTask{SourceType: model.WBSourceAS, SourceID: "A1", Status: model.WBTaskWaiting, WorkType: model.WBWorkAS}
	if _, ok := applyWorkboardSourceStatus(tAS, map[string]string{"A1": model.WBTaskCancelled}, nil); ok {
		t.Fatal("AS 취소가 보여서는 안 된다")
	}
	tAdm := model.WorkTask{WorkType: model.WBWorkAdmin, Status: model.WBTaskCancelled}
	if _, ok := applyWorkboardSourceStatus(tAdm, nil, nil); ok {
		t.Fatal("행정 취소가 보여서는 안 된다")
	}
}

func TestPersonalBoardUntimedLastAndDayStrip(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b9.db"))
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "시각있음", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "시간미정업무", today, "", "", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	ck := jwtCookieUser(t, me)
	list := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0", ck)
	a := strings.Index(list.Body.String(), "시각있음")
	b := strings.Index(list.Body.String(), "시간미정업무")
	if a < 0 || b < 0 || a > b {
		t.Fatalf("리스트 시각 없음이 아래가 아님 a=%d b=%d", a, b)
	}
	day := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0", ck)
	if !strings.Contains(day.Body.String(), "시간미정업무") {
		t.Fatal("일 화면에 시각 없는 업무가 없다")
	}
}

func TestPersonalBoardSearch(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b10.db"))
	today := time.Now().Format(dateLayout)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, address)
		VALUES ('C1','부여군립도서관','부여군립도서관',1,'충남 부여')`); err != nil {
		t.Fatal(err)
	}
	tk := &model.WorkTask{
		Title: "검색대상업무", WorkDate: today, DueDate: today, StartTime: "09:00", EndTime: "10:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, CustomerID: "C1", CustomerName: "부여군립도서관",
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "다른제목", today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	hit := doGetWith(t, e, "/workboard/board?view=sched&range=day&q="+url.QueryEscape("부여"), jwtCookieUser(t, me))
	if hit.Code != http.StatusOK {
		t.Fatalf("검색 무시 조회 %d", hit.Code)
	}
	for _, q := range []string{"%", "_", "'"} {
		rec := doGetWith(t, e, "/workboard/board?q="+url.QueryEscape(q), jwtCookieUser(t, me))
		if rec.Code != http.StatusOK {
			t.Fatalf("q=%q → %d", q, rec.Code)
		}
	}
}

func TestPersonalBoardNoAutoPlaceSnapshot(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b11.db"))
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "스냅샷업무", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	before := workTasksSnap(t, db)
	rec := doGetWith(t, e, "/workboard/board?view=month", jwtCookieUser(t, me))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	after := workTasksSnap(t, db)
	if before != after {
		t.Fatal("조회 후 work_tasks 가 변했다")
	}
}

func workTasksSnap(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT task_id, work_date, start_time, end_time, status, assignee, updated_at FROM work_tasks ORDER BY task_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, wd, st, et, status, asg, upd string
		if err := rows.Scan(&id, &wd, &st, &et, &status, &asg, &upd); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s|%s\n", id, wd, st, et, status, asg, upd)
	}
	return b.String()
}

func TestPersonalBoardQueryCountBound(t *testing.T) {
	var n int64
	qcount9520N.Store(&n)
	t.Cleanup(func() { qcount9520N.Store(nil) })
	path := filepath.Join(t.TempDir(), "bq.db")
	e, _, me, _, repo := seedPersonalBoard(t, sqliteQCount9520, path)
	today := time.Now().Format(dateLayout)
	for i := 0; i < 20; i++ {
		mustCreateBoardTask(t, repo, fmt.Sprintf("q20-%d", i), today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	atomic.StoreInt64(&n, 0)
	if rec := doGetWith(t, e, "/workboard/board?view=month&date="+today, jwtCookieUser(t, me)); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	_ = atomic.LoadInt64(&n)
	for i := 0; i < 180; i++ {
		mustCreateBoardTask(t, repo, fmt.Sprintf("q200-%d", i), today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	atomic.StoreInt64(&n, 0)
	if rec := doGetWith(t, e, "/workboard/board?view=month&date="+today, jwtCookieUser(t, me)); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	q200 := atomic.LoadInt64(&n)
	if q200 > 2000 {
		t.Fatalf("쿼리 상한 초과 q200=%d", q200)
	}
}

func TestPersonalBoardMenuAndLegacyURLs(t *testing.T) {
	ctx := visionMenuCtx()
	res := FilterNav(ctx, NavWorkBoard, NavBadges{})
	hrefs := collectNavHrefs(res.Groups)
	if !containsHref(hrefs, "/workboard/board") {
		t.Fatal("업무 관리 주소 없음")
	}
	var legacy NavItemView
	for _, g := range res.Groups {
		if g.ID != "work" {
			continue
		}
		for _, it := range g.Items {
			if it.ID == "legacy_views" {
				legacy = it
			}
			if it.ID == "work_review" || it.Label == "업무 확인" {
				t.Fatal("업무 확인이 최상위에 있다")
			}
			if it.Label == "일일 업무 등록" {
				t.Fatal("일일 업무 등록이 최상위에 있다")
			}
		}
	}
	want := map[string]string{
		"/workboard/register": "일일 업무 등록 (예전)",
		"/work":               "오늘 업무 (예전)",
		"/work/all":           "전체 업무 (예전)",
		"/stats/reports":      "보고서 (예전)",
	}
	found := map[string]bool{}
	for _, ch := range legacy.Children {
		found[ch.Href] = true
		if w, ok := want[ch.Href]; ok && w != ch.Label {
			t.Errorf("%s label=%s", ch.Href, ch.Label)
		}
	}
	for h := range want {
		if !found[h] {
			t.Errorf("예전 메뉴 %s 없음", h)
		}
	}
	exp := FilterNav(ctx, NavWorkRegister, NavBadges{})
	if !containsHref(exp.CurParents, "legacy_views") {
		t.Fatal("예전 화면 활성 시 그룹이 안 펼쳐짐")
	}

	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b13.db"))
	ck := jwtCookieUser(t, me)
	for _, path := range []string{"/workboard/register", "/work", "/work/all"} {
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusOK {
			t.Errorf("%s → %d", path, rec.Code)
		}
	}
}

func TestPersonalBoardIndexRedirect(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "b14.db"))
	rec := doGetWith(t, e, "/workboard", jwtCookieUser(t, me))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("Location") != "/workboard/board" {
		t.Fatalf("loc=%s", rec.Header().Get("Location"))
	}
}
