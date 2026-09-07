package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
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

func authTestSecret() []byte {
	return []byte("cs-system-jwt-secret-2026")
}

func authTestToken(t *testing.T, claims jwt.MapClaims) *http.Cookie {
	t.Helper()
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(authTestSecret())
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func newClosedUserRepo(t *testing.T) *repository.UserRepo {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserRepo(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return repo
}

func serveAuthedPage(t *testing.T, h *AuthHandler, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Renderer = NewRenderer()
	e.Use(h.AuthMiddleware)
	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "auth/forbidden.html", map[string]interface{}{
			"Title": "접근 권한 없음", "Active": NavNone,
		})
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestAuthMiddleware_DBErrorKeepsMenuAndShowsBanner(t *testing.T) {
	h := &AuthHandler{userRepo: newClosedUserRepo(t), jwtSecret: authTestSecret()}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	adminPerms := model.FormatPermissions(model.DefaultPermissions(model.RoleAdmin))
	ck := authTestToken(t, jwt.MapClaims{
		"user_id": "admin-id", "username": "admin", "role": model.RoleAdmin,
		"name": "관리자", "permissions": adminPerms,
		"verified_at": time.Now().Add(-6 * time.Minute).Unix(),
	})
	rec := serveAuthedPage(t, h, ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, label := range []string{"계획", "실행", "확인", "기준정보", "관리"} {
		if !strings.Contains(body, ">"+label+"<") && !strings.Contains(body, label) {
			t.Fatalf("메뉴 그룹 %q 가 없다", label)
		}
	}
	if !strings.Contains(body, "권한 정보를 불러오지 못했습니다") {
		t.Fatal("배너가 없다")
	}
	if !strings.Contains(buf.String(), "[auth] 사용자 조회 실패") {
		t.Fatalf("조회 실패 로그가 없다: %s", buf.String())
	}
}

func TestAuthMiddleware_EmptyRoleRedirectsLogin(t *testing.T) {
	h := &AuthHandler{userRepo: newClosedUserRepo(t), jwtSecret: authTestSecret()}
	ck := authTestToken(t, jwt.MapClaims{
		"user_id": "x", "username": "x", "role": "",
		"name": "x", "verified_at": time.Now().Unix(),
	})
	rec := serveAuthedPage(t, h, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want redirect", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location=%q", loc)
	}
}

func TestAuthMiddleware_FreshVerifiedAtSkipsDB(t *testing.T) {
	h := &AuthHandler{userRepo: newClosedUserRepo(t), jwtSecret: authTestSecret()}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	adminPerms := model.FormatPermissions(model.DefaultPermissions(model.RoleAdmin))
	ck := authTestToken(t, jwt.MapClaims{
		"user_id": "admin-id", "username": "admin", "role": model.RoleAdmin,
		"name": "관리자", "permissions": adminPerms,
		"verified_at": time.Now().Unix(),
	})
	rec := serveAuthedPage(t, h, ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "계획") || !strings.Contains(body, "기준정보") {
		t.Fatal("메뉴가 유지되지 않았다")
	}
	if strings.Contains(body, "권한 정보를 불러오지 못했습니다") {
		t.Fatal("재검증을 건너뛰었는데 배너가 있다")
	}
	if strings.Contains(buf.String(), "[auth] 사용자 조회 실패") {
		t.Fatalf("재검증을 건너뛰어야 하는데 DB를 조회했다: %s", buf.String())
	}
}

func TestIsAdminUsesNormalizedRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set("role", "관리자")
	h := &AuthHandler{}
	if !h.isAdmin(c) {
		t.Fatal("관리자 별칭을 관리자로 보지 않는다")
	}
}
