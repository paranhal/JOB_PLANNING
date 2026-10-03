package handler

import (
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
	"customer-support/internal/repository"
)

func jwtCookieUserClaims(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  u.UserID,
		"username": u.Username,
		"role":     u.Role,
		"name":     u.FullName,
		"org_id":   strings.TrimSpace(u.OrgID),
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func newObserverSimApp(t *testing.T) (*echo.Echo, *observerSimUsers) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "view-as-sim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "obs1", PasswordHash: HashPassword("pw"), FullName: "옵저버",
		Role: model.RoleObserver, IsActive: true, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "leehj", PasswordHash: HashPassword("pw"), FullName: "이해진",
		Role: model.RoleSales, IsActive: true, OrgID: "O02",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, org_id, is_active)
		VALUES ('C-O1','일팀고객','일팀고객','O01',1), ('C-O2','이팀고객','이팀고객','O02',1)`); err != nil {
		t.Fatal(err)
	}
	obs, err := users.GetByUsername("obs1")
	if err != nil || obs == nil {
		t.Fatal(err)
	}
	lee, err := users.GetByUsername("leehj")
	if err != nil || lee == nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.RequireActiveRole)
	g.Use(h.Auth.GuardViewAsWrite)
	sec := h.Auth.RequireAdminSection
	adminOnly := h.Auth.RequireAdminMW
	g.GET("/customers", h.Customer.List)
	g.POST("/customers", h.Customer.Create, h.Auth.RequireMasterWrite)
	g.GET("/users", h.Auth.UserList, sec, adminOnly)
	g.POST("/view-as", h.Auth.SetViewAs)
	g.POST("/view-as/clear", h.Auth.ClearViewAs)
	return e, &observerSimUsers{obs: obs, lee: lee}
}

type observerSimUsers struct {
	obs *model.User
	lee *model.User
}

func TestObserverDefaultSeesHomeOrgLikeOrgAdmin(t *testing.T) {
	e, ids := newObserverSimApp(t)
	ck := jwtCookieUserClaims(t, ids.obs)
	home := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodGet, "/customers?search="+url.QueryEscape("일팀고객"), nil)
	hreq.AddCookie(ck)
	e.ServeHTTP(home, hreq)
	if home.Code != http.StatusOK {
		t.Fatalf("status=%d loc=%s", home.Code, home.Header().Get("Location"))
	}
	if !strings.Contains(home.Body.String(), "일팀고객") || !strings.Contains(home.Body.String(), "전체 1개 기관") {
		t.Fatal("자기 조직 고객이 없다")
	}
	other := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "/customers?search="+url.QueryEscape("이팀고객"), nil)
	oreq.AddCookie(ck)
	e.ServeHTTP(other, oreq)
	if other.Code != http.StatusOK || strings.Contains(other.Body.String(), "전체 1개 기관") {
		t.Fatal("타 조직 고객이 보인다")
	}
	body := home.Body.String()
	if !strings.Contains(body, "조직관리자 (기본)") {
		t.Fatal("기본 시점 선택이 없다")
	}
	if strings.Contains(body, "시점으로 보고 있습니다") {
		t.Fatal("대상 없이 사람 시점 띠가 떴다")
	}

	write := httptest.NewRecorder()
	wreq := httptest.NewRequest(http.MethodPost, "/customers", strings.NewReader(url.Values{
		"org_name": {"신규"}, "official_name": {"신규"},
	}.Encode()))
	wreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	wreq.AddCookie(ck)
	e.ServeHTTP(write, wreq)
	if write.Code != http.StatusSeeOther && write.Code != http.StatusForbidden {
		t.Fatalf("쓰기 status=%d", write.Code)
	}
}

func TestObserverViewAsInheritsRoleOrgAndBlocksWrite(t *testing.T) {
	e, ids := newObserverSimApp(t)
	ck := jwtCookieUserClaims(t, ids.obs)

	set := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "/view-as", strings.NewReader(url.Values{
		"user_id": {ids.lee.UserID}, "return": {"/customers"},
	}.Encode()))
	sreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	sreq.AddCookie(ck)
	e.ServeHTTP(set, sreq)
	if set.Code != http.StatusSeeOther {
		t.Fatalf("set status=%d", set.Code)
	}
	vas := cookieNamed(set, viewAsCookie)
	if vas == nil || vas.Value != ids.lee.UserID {
		t.Fatal("view_as 쿠키가 없다")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/customers?search="+url.QueryEscape("이팀고객"), nil)
	req.AddCookie(ck)
	req.AddCookie(vas)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "이팀고객") || !strings.Contains(body, "전체 1개 기관") {
		t.Fatal("이해진 조직 고객이 없다")
	}
	home := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodGet, "/customers?search="+url.QueryEscape("일팀고객"), nil)
	hreq.AddCookie(ck)
	hreq.AddCookie(vas)
	e.ServeHTTP(home, hreq)
	if home.Code != http.StatusOK || strings.Contains(home.Body.String(), "전체 1개 기관") {
		t.Fatal("옵저버 홈 조직이 섞였다")
	}
	if !strings.Contains(body, "지금") || !strings.Contains(body, "이해진") || !strings.Contains(body, "시점으로 보고 있습니다") {
		t.Fatal("시점 띠가 없다")
	}

	write := httptest.NewRecorder()
	wreq := httptest.NewRequest(http.MethodPost, "/customers", strings.NewReader(url.Values{
		"org_name": {"신규"}, "official_name": {"신규"},
	}.Encode()))
	wreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	wreq.AddCookie(ck)
	wreq.AddCookie(vas)
	e.ServeHTTP(write, wreq)
	if write.Code != http.StatusForbidden && write.Code != http.StatusSeeOther {
		t.Fatalf("쓰기 status=%d", write.Code)
	}

	users := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodGet, "/users", nil)
	ureq.AddCookie(ck)
	ureq.AddCookie(vas)
	e.ServeHTTP(users, ureq)
	if users.Code != http.StatusOK {
		t.Fatalf("관리 섹션 읽기 %d loc=%s", users.Code, users.Header().Get("Location"))
	}
	if strings.Contains(users.Header().Get("Location"), "/admin/unlock") {
		t.Fatal("옵저버에게 관리 비밀번호를 물었다")
	}
}
