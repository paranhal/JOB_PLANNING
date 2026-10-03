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

func newRenameApp(t *testing.T) (*echo.Echo, *repository.UserRepo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "rename-http.db"))
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
	g.GET("/users", h.Auth.UserList)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.POST("/users/:id/username", h.Auth.UserRenameUsername)
	g.POST("/account/username", h.Auth.AccountRenameUsername)
	g.GET("/login", h.Auth.LoginPage)
	return e, users, db
}

func TestUserRenameUsernameKeepsUserIDAndLogsOutSelf(t *testing.T) {
	e, users, _ := newRenameApp(t)
	u := &model.User{
		Username: "hjlee", PasswordHash: HashPassword("pw"), FullName: "이해진",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	id := u.UserID
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+id+"/username", strings.NewReader(url.Values{"username": {"haejin"}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/users?ok=username") {
		t.Fatalf("관리자가 남을 바꿀 때 loc=%s", rec.Header().Get("Location"))
	}
	got, err := users.GetByID(id)
	if err != nil || got == nil || got.UserID != id || got.Username != "haejin" {
		t.Fatalf("승계 실패 %+v err=%v", got, err)
	}

	self := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "/account/username", strings.NewReader(url.Values{"username": {"haejin2"}}.Encode()))
	sreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	sreq.AddCookie(jwtCookieUserClaims(t, got))
	e.ServeHTTP(self, sreq)
	if self.Code != http.StatusSeeOther || self.Header().Get("Location") != "/login" {
		t.Fatalf("재로그인 안 함 loc=%s", self.Header().Get("Location"))
	}
	cleared := false
	for _, ck := range self.Result().Cookies() {
		if ck.Name == "token" && (ck.MaxAge < 0 || ck.Value == "") {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("쿠키가 안 지워졌다")
	}
}

func TestUserRenameUsernameRejectsDuplicate(t *testing.T) {
	e, users, _ := newRenameApp(t)
	a := &model.User{Username: "one", PasswordHash: "x", FullName: "하나", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1"}
	b := &model.User{Username: "two", PasswordHash: "x", FullName: "둘", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-2"}
	if err := users.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(b); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+a.UserID+"/username", strings.NewReader(url.Values{"username": {"two"}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("중복 통과 loc=%s", rec.Header().Get("Location"))
	}
}

func TestUserUpdateRewritesAssigneeNames(t *testing.T) {
	e, users, db := newRenameApp(t)
	u := &model.User{
		Username: "tech1", PasswordHash: "x", FullName: "옛이름",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, assigned_to, assigned_user_id, status, org_id)
		VALUES ('AS1','R-1','c1','옛이름',?,'open','O01')`, u.UserID); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"full_name": {"새이름"}, "role": {model.RoleTech}, "is_active": {"1"},
		"mobile": {"010-1"}, "org_id": {model.OrgIDLibrary},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "ok=renamed") {
		t.Fatalf("건수 안내 없음 loc=%s", loc)
	}
	var name string
	if err := db.QueryRow(`SELECT assigned_to FROM as_receipts WHERE as_id='AS1'`).Scan(&name); err != nil || name != "새이름" {
		t.Fatalf("이름 글자 %q err=%v", name, err)
	}
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, loc, nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "담당 글자") {
		t.Fatalf("건수 표시 없음 code=%d", page.Code)
	}
}

func TestUserListUnlocksFullNameAndUsernameForm(t *testing.T) {
	e, _ := newUserProfileApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, `type="hidden" name="full_name"`) {
		t.Fatal("이름이 아직 hidden 이다")
	}
	if !strings.Contains(body, "아이디 변경") {
		t.Fatal("아이디 변경 폼이 없다")
	}
}
