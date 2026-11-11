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

func newHead166(t *testing.T) (*echo.Echo, *repository.UserRepo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "head166.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware, h.Auth.InjectViewAs)
	g.GET("/admin/users", h.Auth.UserList)
	g.GET("/admin/approval-settings", h.Approval.Page)
	g.POST("/admin/approval-settings/head", h.Approval.HeadSave)
	return e, users, db
}

func Test95_166B1_UserListHeadUsesSameSave(t *testing.T) {
	enableViewAsForTest(t)
	e, users, db := newHead166(t)
	oa := &model.User{Username: "oa166", PasswordHash: HashPassword("pw"), FullName: "일팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "a@ex.com", ProfileDone: true}
	oa2 := &model.User{Username: "oa166b", PasswordHash: HashPassword("pw"), FullName: "이팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "b@ex.com", ProfileDone: true}
	tech := &model.User{Username: "tech166", PasswordHash: HashPassword("pw"), FullName: "기술", Role: model.RoleTech, IsActive: true, OrgID: "O01", Mobile: "010-1", ProfileDone: true}
	ro := &model.User{Username: "ro166", PasswordHash: HashPassword("pw"), FullName: "읽기", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01", Email: "r@ex.com", ProfileDone: true}
	off := &model.User{Username: "off166", PasswordHash: HashPassword("pw"), FullName: "비활성", Role: model.RoleOrgAdmin, IsActive: false, OrgID: "O01", Email: "o@ex.com", ProfileDone: true}
	away := &model.User{Username: "away166", PasswordHash: HashPassword("pw"), FullName: "타조직", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", Email: "z@ex.com", ProfileDone: true}
	for _, u := range []*model.User{oa, oa2, tech, ro, off, away} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	ck := jwtCookieUserClaims(t, oa)
	page := getAccount(t, e, "/admin/users", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("목록 %d", page.Code)
	}
	body := page.Body.String()
	if strings.Count(body, ">팀장으로 지정</button>") != 2 {
		t.Fatalf("지정 버튼 %d", strings.Count(body, "팀장으로 지정"))
	}
	for _, id := range []string{tech.UserID, ro.UserID, off.UserID, away.UserID} {
		if strings.Contains(body, `name="head_user_id" value="`+id+`"`) {
			t.Fatalf("후보 아닌 버튼 %s", id)
		}
	}
	if strings.Contains(body, ">해제<") {
		t.Fatal("팀장 없는데 해제 버튼")
	}
	csrfCk, csrf := csrfFromRec(t, page)
	set := url.Values{"csrf": {csrf}, "next": {"users"}, "head_user_id": {oa.UserID}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(set.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/admin/users") {
		t.Fatalf("목록 복귀 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	again := getAccount(t, e, "/admin/users", ck).Body.String()
	if strings.Count(again, ">팀장으로 지정</button>") != 1 || strings.Count(again, ">해제</button>") != 1 || !strings.Contains(again, ">팀장</span>") {
		t.Fatalf("지정 후 버튼 지정=%d 해제=%d", strings.Count(again, ">팀장으로 지정</button>"), strings.Count(again, ">해제</button>"))
	}

	set2 := url.Values{"csrf": {csrf}, "next": {"users"}, "head_user_id": {oa2.UserID}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(set2.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	var n int
	var head string
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(user_id),'') FROM employee_profiles WHERE org_id='O01' AND is_org_head=1`).Scan(&n, &head); err != nil {
		t.Fatal(err)
	}
	if n != 1 || head != oa2.UserID {
		t.Fatalf("교체 후 n=%d head=%s", n, head)
	}

	settings := getAccount(t, e, "/admin/approval-settings", ck)
	sc, st := csrfFromRec(t, settings)
	clear := url.Values{"csrf": {st}, "head_user_id": {""}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(clear.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(sc)
	e.ServeHTTP(rec, req)
	_ = db.QueryRow(`SELECT COUNT(*) FROM employee_profiles WHERE org_id='O01' AND is_org_head=1`).Scan(&n)
	if n != 0 {
		t.Fatalf("설정 화면 해제 후 %d", n)
	}
	setBack := url.Values{"csrf": {st}, "head_user_id": {oa.UserID}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(setBack.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(sc)
	e.ServeHTTP(rec, req)
	_ = db.QueryRow(`SELECT user_id FROM employee_profiles WHERE org_id='O01' AND is_org_head=1`).Scan(&head)
	if head != oa.UserID {
		t.Fatalf("두 화면 결과가 다름 %s", head)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(url.Values{"next": {"users"}, "head_user_id": {oa2.UserID}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("csrf %d", rec.Code)
	}

	roCk := jwtCookieUserClaims(t, ro)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(url.Values{"csrf": {csrf}, "head_user_id": {oa.UserID}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(roCk)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("읽기 전용 %d", rec.Code)
	}
	roPage := getAccount(t, e, "/admin/users", roCk).Body.String()
	if strings.Contains(roPage, ">팀장으로 지정</button>") || strings.Contains(roPage, ">해제</button>") {
		t.Fatal("읽기 전용에 버튼이 있다")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(url.Values{"csrf": {csrf}, "head_user_id": {away.UserID}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("타 조직 %d %s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings/head", strings.NewReader(url.Values{"csrf": {csrf}, "head_user_id": {oa2.UserID}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(csrfCk)
	req.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: tech.UserID, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("대리 보기 %d", rec.Code)
	}
}
