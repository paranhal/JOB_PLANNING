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

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

func visionAuthApp(t *testing.T) (*echo.Echo, *sql.DB, *repository.UserRepo, *repository.SettingsRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "vision-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	settings := repository.NewSettingsRepo(db)
	if err := settings.Set(repository.SettingVisionAdminPassword, HashPassword("vision-lock1")); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login", h.Auth.Login)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/", h.Dashboard)
	g.GET("/account", h.Auth.AccountPage)
	g.GET("/account/vision-password", h.Auth.AccountVisionPasswordForm)
	g.POST("/account/vision-password", h.Auth.AccountVisionPassword)
	g.POST("/users", h.Auth.UserCreate)
	g.POST("/users/:id/delete", h.Auth.UserDelete)
	return e, db, users, settings
}

func visionCookie(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func TestLoginPageHidesDefaultAccount(t *testing.T) {
	e, _, _, _ := visionAuthApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "admin / admin") {
		t.Fatal("로그인 화면에 초기 계정이 남아 있다")
	}
}

func TestLoginCookieSecureInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	e, _, _, _ := visionAuthApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{
		"username": {"admin"}, "password": {"admin"},
	}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("로그인 status=%d", rec.Code)
	}
	found := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "token" {
			found = true
			if !ck.Secure {
				t.Fatal("production 쿠키에 Secure가 없다")
			}
		}
	}
	if !found {
		t.Fatal("token 쿠키가 없다")
	}
}

func TestUserCreateRejectsShortPassword(t *testing.T) {
	e, _, _, _ := visionAuthApp(t)
	form := url.Values{
		"username": {"tech1"}, "password": {"short"}, "full_name": {"기술일"},
		"role": {"tech"}, "org_id": {model.OrgIDLibrary}, "mobile": {"010-1111-2222"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("짧은 비밀번호 통과 loc=%s", rec.Header().Get("Location"))
	}
}

func TestVisionPasswordChangeOnlyAsVisionAdmin(t *testing.T) {
	e, _, users, settings := visionAuthApp(t)
	v := &model.User{
		Username: "vision1", PasswordHash: HashPassword("login-secret"),
		FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true, Email: "v@local",
	}
	if err := users.Create(v); err != nil {
		t.Fatal(err)
	}
	bad := httptest.NewRecorder()
	form := url.Values{
		"current_password": {"vision-lock1"},
		"password":         {"new-vision1"},
		"password_confirm": {"new-vision1"},
	}
	breq := httptest.NewRequest(http.MethodPost, "/account/vision-password", strings.NewReader(form.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookie(t))
	e.ServeHTTP(bad, breq)
	if bad.Code != http.StatusForbidden && bad.Code != http.StatusOK {
		t.Fatalf("조직관리자 변경 status=%d", bad.Code)
	}
	got, _ := settings.Get(repository.SettingVisionAdminPassword)
	if !passwd.Verify(got, "vision-lock1") {
		t.Fatal("조직관리자가 비젼 비밀번호를 바꿨다")
	}

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/account/vision-password", strings.NewReader(form.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(visionCookie(t, v))
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("비젼 변경 status=%d body=%s", ok.Code, ok.Body.String())
	}
	got, _ = settings.Get(repository.SettingVisionAdminPassword)
	if !passwd.Verify(got, "new-vision1") {
		t.Fatal("비젼 비밀번호가 안 바뀌었다")
	}
}

func TestVisionMustChangeRedirectsUntilUpdated(t *testing.T) {
	e, _, users, settings := visionAuthApp(t)
	if err := settings.Set(repository.SettingVisionAdminMustChange, "1"); err != nil {
		t.Fatal(err)
	}
	v := &model.User{
		Username: "vision2", PasswordHash: HashPassword("login-secret"),
		FullName: "비젼이", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true, Email: "v2@local",
	}
	if err := users.Create(v); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(visionCookie(t, v))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/account/vision-password" {
		t.Fatalf("강제 변경 미적용 loc=%s", rec.Header().Get("Location"))
	}
}

func TestUserDeleteNeedsVisionPasswordAndZeroWork(t *testing.T) {
	e, db, users, _ := visionAuthApp(t)
	u := &model.User{
		Username: "gone", PasswordHash: HashPassword("login-secret"),
		FullName: "지울사람", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary,
		Mobile: "010-3333-4444", ProfileDone: true,
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, title, assignee, assignee_user_id) VALUES ('t1','할일','지울사람',?)`, u.UserID); err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	form := url.Values{"vision_password": {"vision-lock1"}}
	breq := httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/delete", strings.NewReader(form.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookie(t))
	e.ServeHTTP(blocked, breq)
	if blocked.Code != http.StatusSeeOther || !strings.Contains(blocked.Header().Get("Location"), "err=") {
		t.Fatalf("업무 있는 계정 삭제 loc=%s", blocked.Header().Get("Location"))
	}

	_, _ = db.Exec(`DELETE FROM work_task_members WHERE task_id='t1'`)
	if _, err := db.Exec(`DELETE FROM work_tasks WHERE task_id='t1'`); err != nil {
		t.Fatal(err)
	}
	badpw := httptest.NewRecorder()
	breq = httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/delete", strings.NewReader(url.Values{"vision_password": {"admin"}}.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookie(t))
	e.ServeHTTP(badpw, breq)
	if !strings.Contains(badpw.Header().Get("Location"), "err=") {
		t.Fatal("로그인 비밀번호로 삭제됐다")
	}

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/delete", strings.NewReader(form.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusSeeOther || !strings.Contains(ok.Header().Get("Location"), "ok=deleted") {
		t.Fatalf("삭제 실패 loc=%s", ok.Header().Get("Location"))
	}
	if got, _ := users.GetByID(u.UserID); got != nil {
		t.Fatal("계정이 남아 있다")
	}
}
