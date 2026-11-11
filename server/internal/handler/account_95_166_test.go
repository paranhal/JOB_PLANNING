package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newAccount166(t *testing.T) (*echo.Echo, *repository.UserRepo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "acct166.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO orgs(org_id, org_no, org_name, sort_order) VALUES ('O01','O01','도서관사업팀',1)
		ON CONFLICT(org_id) DO UPDATE SET org_name='도서관사업팀'`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/profile", h.Auth.AccountUpdateProfile)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	g.POST("/account/username", h.Auth.AccountRenameUsername)
	g.POST("/login", h.Auth.Login)
	return e, users, db
}

func postAccount(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func getAccount(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_166A_AccountInfoPasswordAndReadonly(t *testing.T) {
	e, users, db := newAccount166(t)
	me := &model.User{
		Username: "me166", PasswordHash: HashPassword("password10"), FullName: "본인",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", Mobile: "010-1111-1660",
		Email: "me@ex.com", Tel: "02-1", ProfileDone: true,
	}
	head := &model.User{
		Username: "head166", PasswordHash: HashPassword("password10"), FullName: "홍길동",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true, Email: "h@ex.com",
	}
	other := &model.User{
		Username: "other166", PasswordHash: HashPassword("password10"), FullName: "다른이",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", Mobile: "010-2", ProfileDone: true,
	}
	for _, u := range []*model.User{me, head, other} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO employee_profiles(user_id, org_id, hire_date, is_org_head) VALUES (?,?,?,0)`, me.UserID, "O01", "2020-04-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,1)`, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUserClaims(t, me)

	page := getAccount(t, e, "/account", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("GET %d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{"내 정보", "도서관사업팀", "기술", "2020-04-01", "홍길동", "10자 이상 · 현재 비밀번호와 달라야 함", ">내 계정<"} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	if strings.Index(body, `data-account-info="1"`) > strings.Index(body, `data-account-password="1"`) {
		t.Fatal("비밀번호 카드가 내 정보보다 위에 있다")
	}
	if !strings.Contains(body, `href="/account"`) || !strings.Contains(body, "nav-active") {
		t.Fatal("사이드바 내 정보 활성 없음")
	}

	if _, err := db.Exec(`UPDATE employee_profiles SET is_org_head=0 WHERE user_id=?`, head.UserID); err != nil {
		t.Fatal(err)
	}
	none := getAccount(t, e, "/account", ck).Body.String()
	if !strings.Contains(none, "미지정 — 조직관리자가 결재") {
		t.Fatal("팀장 미지정 안내 없음")
	}
	if _, err := db.Exec(`UPDATE employee_profiles SET is_org_head=1 WHERE user_id=?`, head.UserID); err != nil {
		t.Fatal(err)
	}

	badName := postAccount(t, e, "/account/profile", url.Values{
		"full_name": {""}, "mobile": {"010-9999-1661"}, "tel": {"02-9"}, "email": {"kept@ex.com"},
	}, ck)
	if badName.Code != http.StatusOK || !strings.Contains(badName.Body.String(), "이름을 입력하세요.") {
		t.Fatalf("이름 필수 %d", badName.Code)
	}
	kept := badName.Body.String()
	if !strings.Contains(kept, "010-9999-1661") || !strings.Contains(kept, "kept@ex.com") || !strings.Contains(kept, "02-9") {
		t.Fatal("이름 오류 후 연락처가 사라졌다")
	}

	badMobile := postAccount(t, e, "/account/profile", url.Values{
		"full_name": {"새이름"}, "mobile": {""}, "tel": {"02-8"}, "email": {"fmt@ex.com"},
	}, ck)
	if !strings.Contains(badMobile.Body.String(), "핸드폰") || !strings.Contains(badMobile.Body.String(), "새이름") || !strings.Contains(badMobile.Body.String(), "fmt@ex.com") {
		t.Fatal("형식 오류 후 입력값이 사라졌다")
	}
	still, _ := users.GetByID(me.UserID)
	if still.FullName != "본인" || still.Mobile != "010-1111-1660" {
		t.Fatalf("실패인데 저장됨 %+v", still)
	}

	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, status, assignee, assignee_user_id, org_id)
		VALUES ('WT166','admin','이름갱신','2026-11-11','2026-11-11','waiting','본인',?,'O01')`, me.UserID); err != nil {
		t.Fatal(err)
	}
	renamed := postAccount(t, e, "/account/profile", url.Values{
		"full_name": {"본인변경"}, "mobile": {"010-1111-1660"}, "tel": {"02-1"}, "email": {"me@ex.com"},
		"org_id": {"O02"}, "role": {"vision_admin"}, "hire_date": {"1999-01-01"}, "is_org_head": {"1"},
	}, ck)
	loc := renamed.Header().Get("Location")
	if renamed.Code != http.StatusSeeOther || !strings.Contains(loc, "ok=profile&n=") {
		t.Fatalf("이름 변경 loc=%s", loc)
	}
	n := loc[strings.LastIndex(loc, "n=")+2:]
	shown := getAccount(t, e, loc, ck)
	if n == "" || n == "0" || !strings.Contains(shown.Body.String(), n+"건") {
		t.Fatalf("기록 갱신 건수 loc=%s", loc)
	}
	after, _ := users.GetByID(me.UserID)
	if after.Role != model.RoleTech || after.OrgID != "O01" || after.FullName != "본인변경" {
		t.Fatalf("읽기 전용 값이 저장됨 %+v", after)
	}
	var hire string
	var headFlag int
	if err := db.QueryRow(`SELECT COALESCE(hire_date,''), COALESCE(is_org_head,0) FROM employee_profiles WHERE user_id=?`, me.UserID).Scan(&hire, &headFlag); err != nil {
		t.Fatal(err)
	}
	if hire != "2020-04-01" || headFlag != 0 {
		t.Fatalf("입사일·팀장 변조 hire=%s head=%d", hire, headFlag)
	}

	cases := []struct {
		form url.Values
		msg  string
	}{
		{url.Values{"current_password": {"wrong"}, "password": {"password11"}, "password_confirm": {"password11"}}, "현재 비밀번호가 올바르지 않습니다."},
		{url.Values{"current_password": {"password10"}, "password": {"123456789"}, "password_confirm": {"123456789"}}, "10자 이상"},
		{url.Values{"current_password": {"password10"}, "password": {"password11"}, "password_confirm": {"password12"}}, "일치하지 않습니다"},
		{url.Values{"current_password": {"password10"}, "password": {"password10"}, "password_confirm": {"password10"}}, "달라야 합니다"},
	}
	for _, cs := range cases {
		rec := postAccount(t, e, "/account/password", cs.form, ck)
		b := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(b, cs.msg) {
			t.Fatalf("%s → %d %s", cs.msg, rec.Code, b)
		}
		if !strings.Contains(b, "본인변경") || strings.Contains(b, `value="password11"`) || strings.Contains(b, `value="123456789"`) {
			t.Fatal("비밀번호 실패 후 다른 입력이 유지되지 않거나 비밀번호가 남았다")
		}
	}
	ok := postAccount(t, e, "/account/password", url.Values{
		"current_password": {"password10"}, "password": {"password11"}, "password_confirm": {"password11"},
	}, ck)
	if ok.Code != http.StatusSeeOther || !strings.Contains(ok.Header().Get("Location"), "ok=password") {
		t.Fatalf("비밀번호 성공 loc=%s", ok.Header().Get("Location"))
	}
	var next *http.Cookie
	for _, c := range ok.Result().Cookies() {
		if c.Name == "token" && c.Value != "" && c.MaxAge >= 0 {
			next = c
		}
	}
	if next == nil {
		t.Fatal("비밀번호 변경 후 세션이 없다")
	}
	done := getAccount(t, e, "/account?ok=password", next)
	if done.Code != http.StatusOK || !strings.Contains(done.Body.String(), "비밀번호가 변경되었습니다.") {
		t.Fatal("세션으로 성공 화면을 열지 못했다")
	}

	dup := postAccount(t, e, "/account/username", url.Values{"username": {""}}, next)
	if dup.Code != http.StatusOK || !strings.Contains(dup.Body.String(), "아이디를 입력하세요.") || !strings.Contains(dup.Body.String(), `name="username" required value=""`) {
		t.Fatal("아이디 오류 후 입력값이 없다")
	}
	same, _ := users.GetByID(me.UserID)
	if same.Username != "me166" {
		t.Fatalf("아이디가 바뀌었다 %s", same.Username)
	}
}

func Test95_166A_MustChangePasswordFirstAndSidebar(t *testing.T) {
	e, users, _ := newAccount166(t)
	u := &model.User{
		Username: "must166", PasswordHash: HashPassword("password10"), FullName: "임시",
		Role: model.RoleSales, IsActive: true, OrgID: "O01", ProfileDone: true, MustChangePassword: true,
		Mobile: "010-3",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	body := getAccount(t, e, "/account?must=1", jwtCookieUserClaims(t, u)).Body.String()
	if !strings.Contains(body, "임시 비밀번호입니다") || !strings.Contains(body, "ring-primary") {
		t.Fatal("임시 비밀번호 강조 없음")
	}
	if strings.Index(body, `data-account-password="1"`) > strings.Index(body, `data-account-info="1"`) {
		t.Fatal("임시 비밀번호 카드가 맨 위가 아니다")
	}
	for _, ctx := range []MenuCtx{
		{Role: model.RoleTech, LoginRole: model.RoleTech},
		{Role: model.RoleSales, LoginRole: model.RoleSales},
		{Role: model.RoleOrgAdmin, LoginRole: model.RoleOrgAdmin},
		{Role: model.RoleObserver, LoginRole: model.RoleObserver},
		{Role: model.RolePartner, LoginRole: model.RolePartner},
		cLevelMenuCtx(nil),
	} {
		res := FilterNav(ctx, NavAccount, NavBadges{})
		if !containsHref(collectNavHrefs(res.Groups), "/account") || !navHasLabel(res.Groups, "내 정보") {
			t.Fatalf("%s 사이드바에 내 정보가 없다", ctx.Role)
		}
		if n := countNavActive(res.Groups); n != 1 {
			t.Fatalf("%s 활성 %d", ctx.Role, n)
		}
	}
	if missing := oldSidebarUnreachable(visionMenuCtx()); len(missing) > 0 {
		t.Fatalf("도달 누락 %v", missing)
	}
}
