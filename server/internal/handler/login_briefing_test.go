package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func briefingCookie() *http.Cookie {
	return &http.Cookie{Name: loginBriefingCookie, Value: "1", Path: "/"}
}

func briefingOffCookie(day string) *http.Cookie {
	return &http.Cookie{Name: loginBriefingOffCookie, Value: day, Path: "/"}
}

func newLoginBriefingServer(t *testing.T) (*echo.Echo, *sql.DB, *Handler) {
	t.Helper()
	enableViewAsForTest(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "brief50.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','SI사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.GuardViewAsWrite)
	g.Use(h.InjectAssignNotices)
	g.Use(h.InjectLoginBriefing)
	e.POST("/login", h.Auth.Login)
	e.GET("/logout", h.Auth.Logout)
	g.POST("/login-briefing/off", h.LoginBriefingOff)
	g.GET("/", h.Dashboard)
	g.GET("/work", h.Work.List)
	g.GET("/sales/dashboard", h.Sales.Dashboard)
	g.GET("/workboard", h.Workboard.Index)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/meeting", h.Meeting.Show)
	g.POST("/view-as", h.Auth.SetViewAs)
	return e, db, h
}

func mustUser(t *testing.T, users *repository.UserRepo, u *model.User) *model.User {
	t.Helper()
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	got, err := users.GetByUsername(u.Username)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	return got
}

func TestBriefingGreetingAndHint(t *testing.T) {
	am := time.Date(2026, 10, 25, 8, 0, 0, 0, time.Local)
	noon := time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local)
	eve := time.Date(2026, 10, 25, 19, 0, 0, 0, time.Local)
	if got := briefingGreeting("김현우", am); got != "좋은 아침이에요, 김현우 님" {
		t.Fatalf("am=%q", got)
	}
	if got := briefingGreeting("김현우", noon); got != "좋은 오후예요, 김현우 님" {
		t.Fatalf("noon=%q", got)
	}
	if got := briefingGreeting("김현우", eve); got != "오늘도 수고 많으셨어요, 김현우 님" {
		t.Fatalf("eve=%q", got)
	}
	k, s := briefingHint(0, am)
	if k != briefingHintNone || !strings.Contains(s, "없습니다") {
		t.Fatalf("zero hint %s %s", k, s)
	}
	k, s = briefingHint(2, am)
	if k != briefingHintBefore10 || !strings.Contains(s, "오전 10시까지") {
		t.Fatalf("before10 %s %s", k, s)
	}
	k, s = briefingHint(2, time.Date(2026, 10, 25, 10, 0, 0, 0, time.Local))
	if k != briefingHintAfter10 || !strings.Contains(s, "오늘 안에") {
		t.Fatalf("after10 %s %s", k, s)
	}
}

func TestLoginIssuesBriefingCookieAndLogoutClears(t *testing.T) {
	e, _, _ := newLoginBriefingServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader("username=admin&password=admin"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login %d", rec.Code)
	}
	ck := cookieNamed(rec, loginBriefingCookie)
	if ck == nil || ck.Value != "1" {
		t.Fatalf("login_briefing cookie=%v", rec.Result().Header.Values("Set-Cookie"))
	}
	line := ""
	for _, h := range rec.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, loginBriefingCookie+"=") {
			line = strings.ToLower(h)
		}
	}
	if strings.Contains(line, "max-age") || strings.Contains(line, "expires=") {
		t.Fatalf("세션 쿠키가 아니다: %s", line)
	}

	out := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "http://localhost/logout", nil)
	oreq.AddCookie(tokenCookieFrom(rec))
	e.ServeHTTP(out, oreq)
	cleared := cookieNamed(out, loginBriefingCookie)
	if cleared == nil || (cleared.MaxAge >= 0 && cleared.Value != "") {
		t.Fatalf("logout 이 브리핑 쿠키를 안 지움 %v", out.Result().Header.Values("Set-Cookie"))
	}
}

func TestLoginFailDoesNotIssueBriefingCookie(t *testing.T) {
	e, _, _ := newLoginBriefingServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/login", strings.NewReader("username=admin&password=wrong"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(rec, req)
	if cookieNamed(rec, loginBriefingCookie) != nil {
		t.Fatal("실패 로그인에 브리핑 쿠키")
	}
}

func TestBriefingOncePerSessionAndHTMXSkip(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	me := mustUser(t, users, &model.User{
		Username: "kim50", PasswordHash: HashPassword("password10"), FullName: "김현우",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	ck := jwtCookieUser(t, me)
	first := doGetWith(t, e, "/", ck, briefingCookie())
	if first.Code != http.StatusOK {
		t.Fatalf("status=%d", first.Code)
	}
	if !strings.Contains(first.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("첫 홈에 팝업 없음")
	}
	cleared := cookieNamed(first, loginBriefingCookie)
	if cleared == nil || (cleared.MaxAge >= 0 && cleared.Value == "1") {
		t.Fatal("쿠키가 소비되지 않음")
	}
	second := doGetWith(t, e, "/", ck)
	if strings.Contains(second.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("같은 세션 재요청에 팝업")
	}
	hx := httptest.NewRecorder()
	hxReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	hxReq.Header.Set("HX-Request", "true")
	hxReq.AddCookie(ck)
	hxReq.AddCookie(briefingCookie())
	e.ServeHTTP(hx, hxReq)
	if strings.Contains(hx.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("HTMX 요청에 팝업")
	}
	if c := cookieNamed(hx, loginBriefingCookie); c != nil && c.MaxAge < 0 {
		t.Fatal("HTMX 에서 쿠키 소비")
	}
}

func TestBriefingStaffCountsMatchBoard(t *testing.T) {
	e, db, h := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	me := mustUser(t, users, &model.User{
		Username: "staff50", PasswordHash: HashPassword("password10"), FullName: "직원갑",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	other := mustUser(t, users, &model.User{
		Username: "si50", PasswordHash: HashPassword("password10"), FullName: "SI을",
		Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true,
	})
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, h.Workboard.repo, "내오늘", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "내완료", today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "내미배치", "", "", "", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "타조직", today, "09:00", "10:00", "O02", other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	ck := jwtCookieUser(t, me)
	board := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0", ck)
	if board.Code != http.StatusOK {
		t.Fatalf("board %d", board.Code)
	}
	if !strings.Contains(board.Body.String(), "총 2개의 업무") {
		t.Fatalf("리스트 건수 불일치 %s", board.Body.String())
	}
	if !strings.Contains(board.Body.String(), `id="unplanned"`) {
		t.Fatal("미배치 앵커 없음")
	}

	home := doGetWith(t, e, "/", ck, briefingCookie())
	body := home.Body.String()
	i := strings.Index(body, `id="login-briefing-modal"`)
	if i < 0 {
		t.Fatal("브리핑 팝업 없음")
	}
	popup := body[i:]
	if j := strings.Index(popup, `id="assign-notice-modal"`); j > 0 {
		popup = popup[:j]
	}
	if !strings.Contains(popup, "일일 업무") || !strings.Contains(popup, ">2건<") {
		t.Fatalf("일일 건수 불일치: %s", popup)
	}
	if !strings.Contains(popup, "class=\"brief-start\"") || !strings.Contains(popup, "/workboard?new=1") {
		t.Fatal("직원 시작 링크 없음")
	}
	if strings.Contains(popup, "brief-badge") {
		t.Fatal("직원 화면에 조직 전체 배지")
	}
	if strings.Contains(popup, `href="/meeting"`) {
		t.Fatal("직원이 회의로 감")
	}
}

func TestBriefingOrgAdminCountsAndMeetingHref(t *testing.T) {
	e, db, h := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	oa := mustUser(t, users, &model.User{
		Username: "md_admin50", PasswordHash: HashPassword("password10"), FullName: "미디어관리",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	me := mustUser(t, users, &model.User{
		Username: "libtech50", PasswordHash: HashPassword("password10"), FullName: "도서관기술",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	si := mustUser(t, users, &model.User{
		Username: "si_admin50", PasswordHash: HashPassword("password10"), FullName: "SI관리",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true,
	})
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, h.Workboard.repo, "도서관오늘A", today, "09:00", "10:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "도서관오늘B", today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "담당없음", today, "13:00", "14:00", model.OrgIDLibrary, "", "", model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, h.Workboard.repo, "SI오늘", today, "09:00", "10:00", "O02", si.FullName, si.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	home := doGetWith(t, e, "/", jwtCookieUser(t, oa), briefingCookie())
	body := home.Body.String()
	if !strings.Contains(body, "조직 전체") {
		t.Fatal("조직 배지 없음")
	}
	if !strings.Contains(body, `href="/meeting"`) {
		t.Fatal("조직관리자 시작이 회의가 아님")
	}
	if strings.Contains(body, "SI오늘") {
		t.Fatal("타 조직 업무가 브리핑에 노출")
	}
	if !strings.Contains(body, ">3건<") {
		t.Fatalf("조직 일일 건수 예상 3: %s", body)
	}
	if !strings.Contains(body, "미배정 업무") || !strings.Contains(body, ">1건<") {
		t.Fatalf("미배정(담당없음) 1건 아님: %s", body)
	}

	siHome := doGetWith(t, e, "/", jwtCookieUser(t, si), briefingCookie())
	if strings.Contains(siHome.Body.String(), "도서관오늘A") {
		t.Fatal("SI 관리자에게 도서관 업무")
	}
}

func TestBriefingOffCSRFAndNextDay(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	me := mustUser(t, users, &model.User{
		Username: "off50", PasswordHash: HashPassword("password10"), FullName: "오프",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	ck := jwtCookieUser(t, me)

	deny := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "http://localhost/login-briefing/off", nil)
	dreq.AddCookie(ck)
	e.ServeHTTP(deny, dreq)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("csrf 없이 %d", deny.Code)
	}

	day := time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local)
	old := briefingNow
	briefingNow = func() time.Time { return day }
	t.Cleanup(func() { briefingNow = old })

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "http://localhost/login-briefing/off", nil)
	oreq.AddCookie(ck)
	oreq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "testcsrf0123456789ab", Path: "/"})
	oreq.Header.Set("X-CSRF-Token", "testcsrf0123456789ab")
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("off %d", ok.Code)
	}
	off := cookieNamed(ok, loginBriefingOffCookie)
	if off == nil || off.Value != "2026-10-25" {
		t.Fatalf("off cookie=%v", off)
	}

	hidden := doGetWith(t, e, "/", ck, briefingCookie(), briefingOffCookie("2026-10-25"))
	if strings.Contains(hidden.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("오늘은 다시 보지 않기가 무시됨")
	}

	briefingNow = func() time.Time { return day.AddDate(0, 0, 1) }
	shown := doGetWith(t, e, "/", ck, briefingCookie(), briefingOffCookie("2026-10-25"))
	if !strings.Contains(shown.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("다음 날에도 숨김")
	}
}

func TestBriefingSuppressesAssignNoticeWithoutConsumingShown(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	me := mustUser(t, users, &model.User{
		Username: "notice50", PasswordHash: HashPassword("password10"), FullName: "최혜영",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','부여군립도서관','부여군립도서관',1)`); err != nil {
		t.Fatal(err)
	}
	insertUnseenNotice(t, db, me.UserID, "R50-001", "예약오류", time.Now().AddDate(0, 0, -1))
	ck := jwtCookieUser(t, me)
	first := doGetWith(t, e, "/", ck, briefingCookie())
	body := first.Body.String()
	if !strings.Contains(body, "오늘의 업무 브리핑") {
		t.Fatal("브리핑 없음")
	}
	if strings.Contains(body, "id=\"assign-notice-modal\"") {
		t.Fatal("배정 알림이 같이 뜸")
	}
	if cookieNamed(first, assignNoticeShownCookie) != nil {
		t.Fatal("assign_notice_shown 이 소비됨")
	}
	if !strings.Contains(body, "bg-red-500") {
		t.Fatal("사이드바 뱃지가 사라짐")
	}
}

func TestBriefingSalesRedirectDoesNotConsumeCookie(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	sales := mustUser(t, users, &model.User{
		Username: "sale50", PasswordHash: HashPassword("password10"), FullName: "영업오십",
		Role: model.RoleSales, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	ck := jwtCookieUser(t, sales)
	home := doGetWith(t, e, "/", ck, briefingCookie())
	if home.Code != http.StatusFound {
		t.Fatalf("status=%d", home.Code)
	}
	if c := cookieNamed(home, loginBriefingCookie); c != nil && c.MaxAge < 0 {
		t.Fatal("302 에서 브리핑 쿠키 소비")
	}
	dash := doGetWith(t, e, pathSalesDashboard, ck, briefingCookie())
	if dash.Code != http.StatusOK {
		t.Fatalf("dash %d", dash.Code)
	}
	if !strings.Contains(dash.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("영업 대시보드에 브리핑 없음")
	}
	if !strings.Contains(dash.Body.String(), "/workboard?new=1") {
		t.Fatal("영업도 직원 흐름이어야 함")
	}
}

func TestBriefingCLevelAndViewAsSkip(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	cl := mustUser(t, users, &model.User{
		Username: "cl50", PasswordHash: HashPassword("password10"), FullName: "임원",
		Role: model.RoleCLevel, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, IsReadOnly: true,
	})
	home := doGetWith(t, e, "/", jwtCookieUser(t, cl), briefingCookie())
	if strings.Contains(home.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("임원 홈에 브리핑")
	}
	if c := cookieNamed(home, loginBriefingCookie); c != nil && c.MaxAge < 0 {
		t.Fatal("임원 요청에서 쿠키 소비")
	}

	tech := mustUser(t, users, &model.User{
		Username: "va50", PasswordHash: HashPassword("password10"), FullName: "대리대상",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	admin, _ := users.GetByUsername("admin")
	req := httptest.NewRequest(http.MethodPost, "http://localhost/view-as", strings.NewReader("user_id="+tech.UserID))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	req.AddCookie(jwtCookieUser(t, admin))
	e.ServeHTTP(rec, req)
	va := cookieNamed(rec, viewAsCookie)
	if va == nil || va.Value == "" {
		t.Fatalf("view-as 쿠키 없음 %v", rec.Result().Header.Values("Set-Cookie"))
	}
	shown := doGetWith(t, e, "/", jwtCookieUser(t, admin), va, briefingCookie())
	if strings.Contains(shown.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("대리 입력 중 브리핑")
	}
}

func TestBriefingReadOnlyStartViewAndNewQuery(t *testing.T) {
	e, db, _ := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	obs := mustUser(t, users, &model.User{
		Username: "obs50", PasswordHash: HashPassword("password10"), FullName: "열람자",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, IsReadOnly: true,
	})
	home := doGetWith(t, e, "/", jwtCookieUser(t, obs), briefingCookie())
	body := home.Body.String()
	if !strings.Contains(body, "업무 보기") {
		t.Fatal("읽기 전용 라벨 아님")
	}
	if strings.Contains(body, "/workboard?new=1") {
		t.Fatal("읽기 전용에 등록 링크")
	}

	tech := mustUser(t, users, &model.User{
		Username: "new50", PasswordHash: HashPassword("password10"), FullName: "등록가능",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	idx := doGetWith(t, e, "/workboard?new=1", jwtCookieUser(t, tech))
	if idx.Header().Get("Location") != "/workboard/board?new=1" {
		t.Fatalf("index loc=%s", idx.Header().Get("Location"))
	}
	page := doGetWith(t, e, "/workboard/board?new=1", jwtCookieUser(t, tech))
	if !strings.Contains(page.Body.String(), "maybeOpenNewFromQuery") {
		t.Fatal("자동 열기 스니펫 없음")
	}
	hidden := doGetWith(t, e, "/workboard/board?new=1", jwtCookieUser(t, obs))
	if strings.Contains(hidden.Body.String(), "maybeOpenNewFromQuery") {
		t.Fatal("CanWrite=false 에 자동 열기")
	}
}

func TestBriefingCountErrorSkipsPopup(t *testing.T) {
	e, db, h := newLoginBriefingServer(t)
	users := repository.NewUserRepo(db)
	me := mustUser(t, users, &model.User{
		Username: "err50", PasswordHash: HashPassword("password10"), FullName: "오류",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	})
	h.Workboard = nil
	rec := doGetWith(t, e, "/", jwtCookieUser(t, me), briefingCookie())
	if rec.Code != http.StatusOK {
		t.Fatalf("home %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "오늘의 업무 브리핑") {
		t.Fatal("오류 시에도 팝업")
	}
}

func TestBriefingCSSInBundle(t *testing.T) {
	root := filepath.Join(findTemplateRoot(t), "..")
	css, err := os.ReadFile(filepath.Join(root, "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	for _, want := range []string{".brief-panel", ".brief-head", ".brief-start", ".brief-card-daily"} {
		if !strings.Contains(bundle, want) {
			t.Fatalf("번들에 %s 없음 — npm run build:css", want)
		}
	}
}
