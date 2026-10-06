package handler

import (
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

func TestReadOnlyAllowsGetBlocksPostExceptPassword(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	u := &model.User{
		Username: "ro1", PasswordHash: HashPassword("password10"), FullName: "읽기",
		Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, Mobile: "010-1111-2222",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/", h.Dashboard)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	ck := jwtCookieUserClaims(t, u)

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/account", nil)
	greq.AddCookie(ck)
	e.ServeHTTP(get, greq)
	if get.Code != http.StatusOK && get.Code != http.StatusSeeOther {
		t.Fatalf("GET %d", get.Code)
	}

	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader(url.Values{"title": {"x"}}.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(ck)
	e.ServeHTTP(post, preq)
	if post.Code == http.StatusOK {
		t.Fatal("읽기전용 POST 가 통과했다")
	}

	pw := httptest.NewRecorder()
	pwreq := httptest.NewRequest(http.MethodPost, "/account/password", strings.NewReader(url.Values{
		"current_password": {"password10"}, "new_password": {"password11"}, "confirm_password": {"password11"},
	}.Encode()))
	pwreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	pwreq.AddCookie(ck)
	e.ServeHTTP(pw, pwreq)
	if pw.Code == http.StatusForbidden {
		t.Fatalf("자기 비밀번호 변경이 막혔다 %d", pw.Code)
	}
}
