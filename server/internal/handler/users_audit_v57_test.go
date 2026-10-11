package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newUsersAuditApp(t *testing.T) (*echo.Echo, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "users-audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','이팀',2)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.POST("/users/:id/work", h.Auth.UserSetWork)
	g.POST("/users/bulk/org", h.Auth.UsersBulkOrg)
	g.POST("/users/bulk/job", h.Auth.UsersBulkJob)
	g.POST("/users/bulk/readonly", h.Auth.UsersBulkReadonly)
	return e, users
}

func postUsersAudit(t *testing.T, e *echo.Echo, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func mustCreateJobUser(t *testing.T, users *repository.UserRepo, name, role string) *model.User {
	t.Helper()
	u := &model.User{
		Username: name, PasswordHash: HashPassword("password10"), FullName: name,
		Role: role, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func logHas(t *testing.T, needle string) bool {
	t.Helper()
	logs, err := audit.ListLogs(50)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		blob := l.Summary + l.Reason + l.BeforeJSON + l.AfterJSON
		if strings.Contains(blob, needle) {
			if strings.Contains(strings.ToLower(blob), "password") && !strings.Contains(blob, "***") {
				t.Fatal("비밀번호가 로그에 있다")
			}
			return true
		}
	}
	return false
}

func TestUserUpdateLogsRoleOrgReadonlyTester(t *testing.T) {
	e, users := newUsersAuditApp(t)
	u := mustCreateJobUser(t, users, "jobone", model.RoleSales)
	form := url.Values{
		"full_name": {u.FullName}, "admin_grade": {"none"}, "job": {"tech"},
		"org_id": {"O02"}, "is_readonly": {"1"}, "is_test": {"1"}, "is_active": {"1"},
		"prev_role": {model.RoleSales}, "reason": {"점검"},
	}
	rec := postUsersAudit(t, e, "/users/"+u.UserID+"/update", form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("loc=%s", rec.Header().Get("Location"))
	}
	got, _ := users.GetByID(u.UserID)
	if got.Role != model.RoleTech || got.OrgID != "O02" || !got.IsReadOnly || !got.IsTest {
		t.Fatalf("saved %+v", got)
	}
	if !logHas(t, "등급·업무") || !logHas(t, "조직") || !logHas(t, "읽기전용") || !logHas(t, "테스터") {
		t.Fatal("개별 저장 감사 내용이 빠졌다")
	}
}

func TestUserUpdateReadonlyOffRequiresReason(t *testing.T) {
	e, users := newUsersAuditApp(t)
	u := mustCreateJobUser(t, users, "rooff", model.RoleTech)
	u.IsReadOnly = true
	if err := users.Update(u); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"full_name": {u.FullName}, "admin_grade": {"none"}, "job": {"tech"},
		"org_id": {u.OrgID}, "is_readonly": {"0"}, "is_active": {"1"}, "prev_role": {model.RoleTech},
	}
	rec := postUsersAudit(t, e, "/users/"+u.UserID+"/update", form)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("사유 없이 끄기가 됐다 loc=%s", rec.Header().Get("Location"))
	}
	form.Set("reason", "현장 조치 필요")
	ok := postUsersAudit(t, e, "/users/"+u.UserID+"/update", form)
	if strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("loc=%s", ok.Header().Get("Location"))
	}
	if !logHas(t, "읽기전용 켜기 → 끄기") || !logHas(t, "현장 조치 필요") {
		t.Fatal("끄기 사유 로그가 없다")
	}
}

func TestUsersBulkOrgAndJobLogFromTo(t *testing.T) {
	e, users := newUsersAuditApp(t)
	a := mustCreateJobUser(t, users, "bulka", model.RoleTech)
	form := url.Values{"user_ids": {a.UserID}, "org_id": {"O02"}}
	rec := postUsersAudit(t, e, "/users/bulk/org", form)
	if strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("org loc=%s", rec.Header().Get("Location"))
	}
	if !logHas(t, "조직") || !logHas(t, "O02") {
		t.Fatal("일괄 조직 무엇에서 무엇으로가 없다")
	}
	job := url.Values{"user_ids": {a.UserID}, "job": {"sales"}}
	jrec := postUsersAudit(t, e, "/users/bulk/job", job)
	if strings.Contains(jrec.Header().Get("Location"), "err=") {
		t.Fatalf("job loc=%s", jrec.Header().Get("Location"))
	}
	if !logHas(t, "기술") || !logHas(t, "영업") {
		t.Fatal("일괄 업무 무엇이 없다")
	}
}

func TestUsersBulkReadonlyOffRequiresReason(t *testing.T) {
	e, users := newUsersAuditApp(t)
	u := mustCreateJobUser(t, users, "bulkro", model.RoleTech)
	u.IsReadOnly = true
	_ = users.Update(u)
	bad := postUsersAudit(t, e, "/users/bulk/readonly", url.Values{"user_ids": {u.UserID}, "on": {"0"}})
	if !strings.Contains(bad.Header().Get("Location"), "err=") {
		t.Fatal("일괄 끄기가 사유 없이 됐다")
	}
	ok := postUsersAudit(t, e, "/users/bulk/readonly", url.Values{
		"user_ids": {u.UserID}, "on": {"0"}, "reason": {"현장 복귀"},
	})
	if strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("loc=%s", ok.Header().Get("Location"))
	}
	got, _ := users.GetByID(u.UserID)
	if got.IsReadOnly {
		t.Fatal("끄기가 안 됐다")
	}
	if !logHas(t, "읽기전용 켜기 → 끄기") {
		t.Fatal("일괄 끄기 로그가 없다")
	}
}

