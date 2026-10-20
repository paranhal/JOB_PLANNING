package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

func users94App(t *testing.T) (*echo.Echo, *sql.DB, *repository.UserRepo, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "u94.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("password10")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','이팀',2)`); err != nil {
		t.Fatal(err)
	}
	vs := &model.User{
		Username: "vs94", PasswordHash: HashPassword("password10"), FullName: "비젼관리자",
		Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true,
	}
	if err := users.Create(vs); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.GuardViewAsWrite)
	g.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") })
	g.GET("/admin/users", h.Auth.UserList)
	g.POST("/users", h.Auth.UserCreate)
	g.POST("/users/:id/password", h.Auth.UserChangePassword)
	g.POST("/users/:id/reset-password", h.Auth.UserResetPassword)
	g.GET("/admin/permissions", h.Auth.PermissionsPage, h.Auth.RequireVisionOnly)
	g.POST("/admin/permissions/save", h.Auth.PermissionsSave, h.Auth.RequireVisionOnly)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	g.POST("/login", h.Auth.Login)
	return e, db, users, vs
}

func doUsers94(t *testing.T, e *echo.Echo, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	for _, ck := range cookies {
		if ck != nil {
			req.AddCookie(ck)
		}
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestUsers94CreateButtonFollowsLoginRole(t *testing.T) {
	e, _, users, vs := users94App(t)
	oa := &model.User{
		Username: "oa94", PasswordHash: HashPassword("password10"), FullName: "조직장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	ck := visionCookie(t, vs)
	page := doUsers94(t, e, http.MethodGet, "/admin/users", nil, ck)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "아이디 생성") {
		t.Fatalf("비전 버튼 없음 %d", page.Code)
	}
	view := doUsers94(t, e, http.MethodGet, "/admin/users", nil, ck, &http.Cookie{Name: viewAsCookie, Value: oa.UserID, Path: "/"})
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), "아이디 생성") {
		t.Fatalf("보기 전환 중 버튼 없음 %d", view.Code)
	}
	pw := strings.Repeat("a", passwd.MinLen)
	created := doUsers94(t, e, http.MethodPost, "/users", url.Values{
		"username": {"u94a"}, "password": {pw}, "full_name": {"생성일"},
		"admin_grade": {"none"}, "job": {"tech"}, "org_id": {"O01"},
	}, ck, &http.Cookie{Name: viewAsCookie, Value: oa.UserID, Path: "/"})
	if created.Code != http.StatusSeeOther || strings.Contains(created.Header().Get("Location"), "err=") {
		t.Fatalf("보기 전환 중 생성 %d loc=%s", created.Code, created.Header().Get("Location"))
	}
	oaPage := doUsers94(t, e, http.MethodGet, "/admin/users", nil, jwtOrgCookie(t, oa))
	if oaPage.Code != http.StatusOK {
		t.Fatalf("조직관리자 목록 %d", oaPage.Code)
	}
	if strings.Contains(oaPage.Body.String(), `id="btn-new"`) || strings.Contains(oaPage.Body.String(), "＋ 아이디 생성") {
		t.Fatal("조직관리자에게 생성 버튼이 보인다")
	}
	deny := doUsers94(t, e, http.MethodPost, "/users", url.Values{
		"username": {"hack94"}, "password": {pw}, "full_name": {"막힘"},
		"admin_grade": {"none"}, "job": {"tech"},
	}, jwtOrgCookie(t, oa))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("조직관리자 생성 want 403 got %d", deny.Code)
	}
}

func TestUsers94CreateGrades(t *testing.T) {
	e, _, users, vs := users94App(t)
	ck := visionCookie(t, vs)
	pw := strings.Repeat("b", passwd.MinLen)
	cases := []struct {
		user, name, grade, job, org, wantRole, wantOrg string
	}{
		{"g94tech", "일반", "none", "tech", "O01", model.RoleTech, "O01"},
		{"g94oa", "조직", "org", "", "O01", model.RoleOrgAdmin, "O01"},
		{"g94ceo", "경영", "exec", "", "O01", model.RoleCLevel, ""},
		{"g94vs", "비전2", "vision", "", "O01", model.RoleVisionAdmin, ""},
	}
	for _, tc := range cases {
		rec := doUsers94(t, e, http.MethodPost, "/users", url.Values{
			"username": {tc.user}, "password": {pw}, "full_name": {tc.name},
			"admin_grade": {tc.grade}, "job": {tc.job}, "org_id": {tc.org},
		}, ck)
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("%s loc=%s", tc.user, rec.Header().Get("Location"))
		}
		u, _ := users.GetByUsername(tc.user)
		if u == nil || u.Role != tc.wantRole || strings.TrimSpace(u.OrgID) != tc.wantOrg {
			t.Fatalf("%s role=%s org=%s", tc.user, u.Role, u.OrgID)
		}
	}
	dup := doUsers94(t, e, http.MethodPost, "/users", url.Values{
		"username": {"g94tech"}, "password": {pw}, "full_name": {"중복"},
		"admin_grade": {"none"}, "job": {"tech"},
	}, ck)
	if dup.Code != http.StatusSeeOther || !strings.Contains(dup.Header().Get("Location"), "err=") {
		t.Fatal("중복 아이디를 막지 않았다")
	}
	short := doUsers94(t, e, http.MethodPost, "/users", url.Values{
		"username": {"g94short"}, "password": {"short"}, "full_name": {"짧음"},
		"admin_grade": {"none"}, "job": {"tech"},
	}, ck)
	if short.Code != http.StatusSeeOther || !strings.Contains(short.Header().Get("Location"), "err=") {
		t.Fatal("짧은 비밀번호를 막지 않았다")
	}
}

func TestUsers94PasswordChangeAndReset(t *testing.T) {
	e, db, users, vs := users94App(t)
	ck := visionCookie(t, vs)
	target := &model.User{
		Username: "pw94", PasswordHash: HashPassword("password10"), FullName: "비번대상",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	if err := users.Create(target); err != nil {
		t.Fatal(err)
	}
	next := "newpassw0rdX"
	chg := doUsers94(t, e, http.MethodPost, "/users/"+target.UserID+"/password", url.Values{
		"password": {next}, "password_confirm": {next}, "must_change_password": {"1"},
	}, ck)
	if chg.Code != http.StatusSeeOther {
		t.Fatalf("변경 %d", chg.Code)
	}
	got, _ := users.GetByID(target.UserID)
	if got == nil || !verifyPassword(got.PasswordHash, next) || !got.MustChangePassword {
		t.Fatal("변경 후 해시·강제변경")
	}

	reset := doUsers94(t, e, http.MethodPost, "/users/"+target.UserID+"/reset-password", nil, ck)
	if reset.Code != http.StatusOK {
		t.Fatalf("초기화 %d", reset.Code)
	}
	if strings.Contains(reset.Header().Get("Location"), "ok=") {
		t.Fatal("초기화 결과가 리다이렉트 쿼리에 있다")
	}
	m := regexp.MustCompile(`id="temp-pw"[^>]*>([^<]+)`).FindStringSubmatch(reset.Body.String())
	if len(m) < 2 {
		t.Fatal("임시 비밀번호 표시 없음")
	}
	temp := m[1]
	if err := assertResetPasswordShape(temp); err != nil {
		t.Fatal(err)
	}
	loc := reset.Header().Get("Location")
	if strings.Contains(loc, temp) || strings.Contains(reset.Header().Get("Set-Cookie"), temp) {
		t.Fatal("임시 비밀번호가 헤더에 있다")
	}
	again := doUsers94(t, e, http.MethodGet, "/admin/users", nil, ck)
	if strings.Contains(again.Body.String(), temp) {
		t.Fatal("새로고침 후에도 임시 비밀번호가 남았다")
	}
	after, _ := users.GetByID(target.UserID)
	if after == nil || !verifyPassword(after.PasswordHash, temp) || !after.MustChangePassword {
		t.Fatal("초기화 후 DB")
	}
	if rowsContainTemp(t, db, temp) {
		t.Fatal("감사·로그에 임시 비밀번호가 있다")
	}

	self := doUsers94(t, e, http.MethodPost, "/users/"+vs.UserID+"/reset-password", nil, ck)
	if self.Code != http.StatusSeeOther || !strings.Contains(self.Header().Get("Location"), "err=") {
		t.Fatalf("자기 초기화 %d loc=%s", self.Code, self.Header().Get("Location"))
	}

	login := doUsers94(t, e, http.MethodPost, "/login", url.Values{"username": {"pw94"}, "password": {temp}})
	if login.Code != http.StatusSeeOther {
		t.Fatalf("임시 로그인 %d", login.Code)
	}
	var loginCk *http.Cookie
	for _, c := range login.Result().Cookies() {
		if c.Name == "token" {
			loginCk = c
		}
	}
	must := doUsers94(t, e, http.MethodGet, "/", nil, loginCk)
	if must.Code != http.StatusSeeOther || !strings.Contains(must.Header().Get("Location"), "/account") {
		acc := doUsers94(t, e, http.MethodGet, "/account?must=1", nil, loginCk)
		if acc.Code != http.StatusOK || !strings.Contains(acc.Body.String(), "임시 비밀번호입니다") {
			t.Fatalf("강제 변경 화면 없음 loc=%s acc=%d", must.Header().Get("Location"), acc.Code)
		}
	}
}

func TestUsers94ResetScope(t *testing.T) {
	e, _, users, _ := users94App(t)
	oa := &model.User{
		Username: "oa-rst", PasswordHash: HashPassword("password10"), FullName: "일팀장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	home := &model.User{
		Username: "home-rst", PasswordHash: HashPassword("password10"), FullName: "일팀원",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	away := &model.User{
		Username: "away-rst", PasswordHash: HashPassword("password10"), FullName: "이팀원",
		Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true,
	}
	ceo := &model.User{
		Username: "ceo-rst", PasswordHash: HashPassword("password10"), FullName: "대표",
		Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true,
	}
	vs2 := &model.User{
		Username: "vs-rst", PasswordHash: HashPassword("password10"), FullName: "비전2",
		Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true,
	}
	for _, u := range []*model.User{oa, home, away, ceo, vs2} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	ck := jwtOrgCookie(t, oa)
	foreign := doUsers94(t, e, http.MethodPost, "/users/"+away.UserID+"/reset-password", nil, ck)
	if foreign.Code != http.StatusNotFound && foreign.Code != http.StatusForbidden {
		t.Fatalf("타 조직 %d", foreign.Code)
	}
	cl := doUsers94(t, e, http.MethodPost, "/users/"+ceo.UserID+"/reset-password", nil, ck)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", cl.Code)
	}
	vs := doUsers94(t, e, http.MethodPost, "/users/"+vs2.UserID+"/reset-password", nil, ck)
	if vs.Code != http.StatusForbidden {
		t.Fatalf("비전 %d", vs.Code)
	}
}

func TestUsers94PermissionsCLevelColumn(t *testing.T) {
	e, _, _, vs := users94App(t)
	ck := visionCookie(t, vs)
	page := doUsers94(t, e, http.MethodGet, "/admin/permissions?tab=grade", nil, ck)
	if page.Code != http.StatusOK {
		t.Fatalf("권한 %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "C-Level(경영진)") {
		t.Fatal("C-Level 열 없음")
	}
	if !strings.Contains(body, `colspan="5"`) {
		t.Fatal("등급 탭 colspan")
	}
	if !strings.Contains(body, "C-Level은 조회만 가능합니다") {
		t.Fatal("안내 문구")
	}
	if !strings.Contains(body, "disabled>본인 것만") || !strings.Contains(body, "disabled>전부") {
		t.Fatal("C-Level 쓰기 선택지 비활성 아님")
	}
	form := url.Values{"tab": {"grade"}}
	form.Add("role", model.RoleCLevel)
	form.Add("perm_key", model.PermASCreate)
	form.Add("access", "3")
	form.Add("role", model.RoleCLevel)
	form.Add("perm_key", model.PermUsersEdit)
	form.Add("access", "3")
	save := doUsers94(t, e, http.MethodPost, "/admin/permissions/save", form, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", save.Code)
	}
	if model.PermAccess(model.RoleCLevel, model.PermASCreate) > model.AccessView {
		t.Fatalf("AS 생성 %d", model.PermAccess(model.RoleCLevel, model.PermASCreate))
	}
	if model.PermAccess(model.RoleCLevel, model.PermUsersEdit) != model.AccessNone {
		t.Fatalf("계정 수정 %d", model.PermAccess(model.RoleCLevel, model.PermUsersEdit))
	}
	t.Cleanup(func() { model.SetAccessOverrides(nil) })
}

func TestRandomResetPasswordShape(t *testing.T) {
	for i := 0; i < 20; i++ {
		pw, err := randomResetPassword()
		if err != nil {
			t.Fatal(err)
		}
		if err := assertResetPasswordShape(pw); err != nil {
			t.Fatal(err)
		}
	}
}

func assertResetPasswordShape(pw string) error {
	if len([]rune(pw)) != resetPasswordLen {
		return errStr("len")
	}
	if strings.ContainsAny(pw, "0O1lI") {
		return errStr("confusable")
	}
	var low, up, dig, sym bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			low = true
		case unicode.IsUpper(r):
			up = true
		case unicode.IsDigit(r):
			dig = true
		default:
			sym = true
		}
	}
	if !low || !up || !dig || !sym {
		return errStr("classes")
	}
	return nil
}

type errStr string

func (e errStr) Error() string { return string(e) }

func rowsContainTemp(t *testing.T, db *sql.DB, temp string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		return false
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			tables = append(tables, n)
		}
	}
	for _, table := range tables {
		q := `SELECT * FROM "` + strings.ReplaceAll(table, `"`, "") + `"`
		r2, err := db.Query(q)
		if err != nil {
			continue
		}
		cols, _ := r2.Columns()
		for r2.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if r2.Scan(ptrs...) != nil {
				continue
			}
			for i, v := range raw {
				if cols[i] == "password_hash" {
					continue
				}
				s, _ := v.(string)
				if s == "" {
					if b, ok := v.([]byte); ok {
						s = string(b)
					}
				}
				if s != "" && strings.Contains(s, temp) {
					r2.Close()
					return true
				}
			}
		}
		r2.Close()
	}
	return false
}
