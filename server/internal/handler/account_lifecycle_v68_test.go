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

func v68App(t *testing.T) (*echo.Echo, *repository.UserRepo, *repository.AccountRepo, *repository.SettingsRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "v68.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	settings := repository.NewSettingsRepo(db)
	_ = settings.Set(repository.SettingVisionAdminPassword, HashPassword("vision-lock1"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.InjectAssignNotices)
	g.GET("/", h.Dashboard)
	sec := h.Auth.RequireAdminSection
	adminOnly := h.Auth.RequireAdminMW
	g.GET("/admin/users", h.Auth.UserList, sec, adminOnly)
	g.GET("/codes", h.Code.List, sec, adminOnly)
	g.GET("/admin/data", h.Backup.Page, sec, adminOnly)
	g.GET("/admin/orgs", h.Org.List, sec)
	g.GET("/admin/permissions", h.Auth.PermissionsPage, sec, h.Auth.RequireVisionOnly)
	g.POST("/users", h.Auth.UserCreate, sec, adminOnly)
	g.POST("/users/:id/delete", h.Auth.UserDelete, sec, adminOnly)
	g.GET("/admin/my-org/members", h.Auth.MembersList, sec, adminOnly)
	g.POST("/admin/my-org/members", h.Auth.MemberCreate, sec, adminOnly)
	g.GET("/admin/users/requests", h.Auth.AccountRequestList, sec, adminOnly)
	g.POST("/admin/users/requests/:id/decide", h.Auth.AccountRequestDecide, sec, adminOnly)
	g.GET("/admin/users/deleted", h.Auth.DeletedUsers, sec, adminOnly)
	g.POST("/admin/users/deleted/:id/restore", h.Auth.RestoreUser, sec, adminOnly)
	g.POST("/login", h.Auth.Login)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	return e, users, repository.NewAccountRepo(db), settings
}

func TestV68AccountRequestPopupOnHome(t *testing.T) {
	e, users, acc, _ := v68App(t)
	oa := &model.User{Username: "oa-pop", PasswordHash: HashPassword("password10"), FullName: "요청장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	vs := &model.User{Username: "vs-pop", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	_ = users.Create(oa)
	_ = users.Create(vs)

	empty := httptest.NewRecorder()
	ereq := httptest.NewRequest(http.MethodGet, "/", nil)
	ereq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(empty, ereq)
	if empty.Code != http.StatusOK {
		t.Fatalf("빈 홈 %d", empty.Code)
	}
	if strings.Contains(empty.Body.String(), "계정 등록 요청") {
		t.Fatal("0건인데 팝업 문구가 있다")
	}

	if err := acc.CreateRequest(&model.AccountRequest{
		OrgID: "O01", RequestedBy: oa.UserID, FullName: "신규", Username: "newpop68", Role: model.RoleTech,
	}); err != nil {
		t.Fatal(err)
	}

	vis := httptest.NewRecorder()
	vreq := httptest.NewRequest(http.MethodGet, "/", nil)
	vreq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(vis, vreq)
	if vis.Code != http.StatusOK {
		t.Fatalf("비젼 홈 %d loc=%s", vis.Code, vis.Header().Get("Location"))
	}
	if !strings.Contains(vis.Body.String(), "계정 등록 요청 1건") {
		t.Fatal("비젼 홈에 등록 요청이 없다")
	}

	org := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "/", nil)
	oreq.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(org, oreq)
	if org.Code != http.StatusOK {
		t.Fatalf("조직장 홈 %d loc=%s", org.Code, org.Header().Get("Location"))
	}
	if strings.Contains(org.Body.String(), "계정 등록 요청") {
		t.Fatal("조직관리자 홈에 등록 요청 팝업이 있다")
	}
}

func TestV68UnlockSkippedOnUserOrgPerms(t *testing.T) {
	e, users, _, _ := v68App(t)
	oa := &model.User{Username: "oa68", PasswordHash: HashPassword("password10"), FullName: "조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	vs := &model.User{Username: "vs68", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(vs); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		ck   *http.Cookie
		path string
	}{
		{jwtOrgCookie(t, oa), "/admin/users"},
		{jwtOrgCookie(t, oa), "/admin/orgs"},
		{visionCookie(t, vs), "/admin/users"},
		{visionCookie(t, vs), "/admin/orgs"},
		{visionCookie(t, vs), "/admin/permissions"},
	}
	for _, it := range checks {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, it.path, nil)
		req.AddCookie(it.ck)
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusSeeOther && strings.Contains(rec.Header().Get("Location"), "/admin/unlock") {
			t.Fatalf("%s 가 잠긴다 %s", it.path, rec.Header().Get("Location"))
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/codes", nil)
	req.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/admin/unlock") {
		t.Fatalf("코드 관리 잠금이 풀렸다 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestV68RequestApproveTempPasswordOnce(t *testing.T) {
	e, users, acc, _ := v68App(t)
	oa := &model.User{Username: "oa-req", PasswordHash: HashPassword("password10"), FullName: "요청장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	vs := &model.User{Username: "vs-req", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	_ = users.Create(oa)
	_ = users.Create(vs)
	rec := httptest.NewRecorder()
	form := url.Values{"username": {"newtech68"}, "full_name": {"신규기술"}, "job": {"tech"}, "reason": {"충원"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/my-org/members", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("요청 loc=%s", rec.Header().Get("Location"))
	}
	if u, _ := users.GetByUsername("newtech68"); u != nil {
		t.Fatal("승인 전에 계정이 생겼다")
	}
	list, _ := acc.ListRequests(model.AccountReqPending, "O01", oa.UserID)
	if len(list) != 1 {
		t.Fatalf("pending %d", len(list))
	}
	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/admin/users/requests/"+list[0].RequestID+"/decide", strings.NewReader(url.Values{"how": {"approve"}}.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(bad, breq)
	if bad.Code != http.StatusForbidden && bad.Code != http.StatusOK {
		t.Fatalf("조직관리자 승인 code=%d", bad.Code)
	}

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/admin/users/requests/"+list[0].RequestID+"/decide", strings.NewReader(url.Values{"how": {"approve"}}.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(ok, oreq)
	loc := ok.Header().Get("Location")
	if !strings.Contains(loc, "once=") {
		t.Fatalf("임시 비밀번호 토큰 없음 loc=%s", loc)
	}
	u, _ := users.GetByUsername("newtech68")
	if u == nil || u.Role != model.RoleTech || !u.MustChangePassword {
		t.Fatalf("계정 %+v", u)
	}
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, loc, nil)
	preq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "이 화면을 새로고침하면") {
		t.Fatalf("1회 표시 실패 %d", page.Code)
	}
	again := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodGet, loc, nil)
	areq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(again, areq)
	if strings.Contains(again.Body.String(), "이 화면을 새로고침하면") {
		t.Fatal("평문이 남았다")
	}
}

func TestV68SoftDeleteRestoreAndReservedUsername(t *testing.T) {
	e, users, _, _ := v68App(t)
	vs := &model.User{Username: "vs-sd", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	u := &model.User{Username: "gone68", PasswordHash: HashPassword("password10"), FullName: "떠날사람", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true, Mobile: "010-1111-2222"}
	mate := &model.User{Username: "stay68", PasswordHash: HashPassword("password10"), FullName: "남을사람", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	_ = users.Create(vs)
	_ = users.Create(u)
	_ = users.Create(mate)
	form := url.Values{"vision_password": {"vision-lock1"}, "reason": {"퇴사"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "ok=deleted") {
		t.Fatalf("delete loc=%s", rec.Header().Get("Location"))
	}
	if live, _ := users.GetByUsername("gone68"); live != nil {
		t.Fatal("삭제 아이디로 조회된다")
	}
	taken, _ := users.UsernameTaken("GONE68", "")
	if !taken {
		t.Fatal("예약이 안 막는다")
	}
	oa := &model.User{Username: "oa-rsv", PasswordHash: HashPassword("password10"), FullName: "장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	_ = users.Create(oa)
	deny := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "/admin/my-org/members", strings.NewReader(url.Values{"username": {"gone68"}, "full_name": {"다른사람"}, "job": {"tech"}}.Encode()))
	dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	dreq.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(deny, dreq)
	loc := deny.Header().Get("Location")
	if !strings.Contains(loc, url.QueryEscape(model.UsernameReservedMsg)) && !strings.Contains(loc, "%EC%9D%B4%EB%AF%B8") {
		t.Fatalf("예약 거부 loc=%s", loc)
	}

	rst := httptest.NewRecorder()
	rform := url.Values{"vision_password": {"vision-lock1"}, "reason": {"재입사"}, "job": {"support"}, "org_id": {"O01"}}
	rreq := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+u.UserID+"/restore", strings.NewReader(rform.Encode()))
	rreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rreq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rst, rreq)
	if !strings.Contains(rst.Header().Get("Location"), "ok=") {
		t.Fatalf("restore loc=%s", rst.Header().Get("Location"))
	}
	back, _ := users.GetByID(u.UserID)
	if back == nil || back.IsDeleted() || back.Role != model.RoleSupport {
		t.Fatalf("복구 %+v", back)
	}
}

func TestV68OrgAdminForeign404AndRejectAdminRole(t *testing.T) {
	e, users, _, _ := v68App(t)
	oa := &model.User{Username: "oa-f", PasswordHash: HashPassword("password10"), FullName: "일팀", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	away := &model.User{Username: "away68", PasswordHash: HashPassword("password10"), FullName: "이팀", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	_ = users.Create(oa)
	_ = users.Create(away)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+away.UserID+"/delete", strings.NewReader(url.Values{"reason": {"x"}, "vision_password": {"vision-lock1"}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/admin/my-org/members", strings.NewReader(url.Values{"username": {"boss68"}, "full_name": {"상승"}, "job": {"org_admin"}}.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(bad, breq)
	if !strings.Contains(bad.Header().Get("Location"), "err=") {
		t.Fatalf("권한 상승 요청이 통했다 loc=%s", bad.Header().Get("Location"))
	}
}

func TestV68FirstLoginMustChangePassword(t *testing.T) {
	e, users, _, _ := v68App(t)
	u := &model.User{Username: "must68", PasswordHash: HashPassword("password10"), FullName: "변경강제", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true, MustChangePassword: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"username": {"must68"}, "password": {"password10"}}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/account") {
		t.Fatalf("첫 로그인 변경 강제 loc=%s code=%d", rec.Header().Get("Location"), rec.Code)
	}
}

func TestV68DeletedListSameUserID(t *testing.T) {
	e, users, _, _ := v68App(t)
	vs := &model.User{Username: "vs-dl", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	u := &model.User{Username: "list68", PasswordHash: HashPassword("password10"), FullName: "목록대상", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	_ = users.Create(vs)
	_ = users.Create(u)
	id := u.UserID
	form := url.Values{"vision_password": {"vision-lock1"}, "reason": {"퇴사"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+id+"/delete", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rec, req)
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "/admin/users/deleted", nil)
	preq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "list68") {
		t.Fatalf("삭제 목록 %d", page.Code)
	}
	rst := httptest.NewRecorder()
	rreq := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+id+"/restore", strings.NewReader(url.Values{
		"vision_password": {"vision-lock1"}, "reason": {"재입사"}, "job": {"tech"}, "org_id": {"O01"},
	}.Encode()))
	rreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rreq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rst, rreq)
	back, _ := users.GetByID(id)
	if back == nil || back.UserID != id || back.IsDeleted() {
		t.Fatalf("같은 user_id 복구 %+v", back)
	}
}

func TestV68RejectNeedsNote(t *testing.T) {
	e, users, acc, _ := v68App(t)
	oa := &model.User{Username: "oa-rj", PasswordHash: HashPassword("password10"), FullName: "장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	vs := &model.User{Username: "vs-rj", PasswordHash: HashPassword("password10"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	_ = users.Create(oa)
	_ = users.Create(vs)
	_ = httptest.NewRequest(http.MethodPost, "/", nil)
	form := url.Values{"username": {"rj68"}, "full_name": {"반려대상"}, "job": {"sales"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/my-org/members", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(httptest.NewRecorder(), req)
	list, _ := acc.ListRequests(model.AccountReqPending, "", "")
	if len(list) == 0 {
		t.Fatal("요청 없음")
	}
	rec := httptest.NewRecorder()
	rreq := httptest.NewRequest(http.MethodPost, "/admin/users/requests/"+list[0].RequestID+"/decide", strings.NewReader(url.Values{"how": {"reject"}}.Encode()))
	rreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rreq.AddCookie(visionCookie(t, vs))
	e.ServeHTTP(rec, rreq)
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("사유 없이 반려 loc=%s", rec.Header().Get("Location"))
	}
}
