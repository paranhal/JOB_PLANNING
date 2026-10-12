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

func TestApprovalSettingsHTTP(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "apset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	org := &model.User{Username: "oa1", PasswordHash: HashPassword("pw"), FullName: "조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	other := &model.User{Username: "oa2", PasswordHash: HashPassword("pw"), FullName: "다른장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02"}
	ro := &model.User{Username: "roa", PasswordHash: HashPassword("pw"), FullName: "읽기장", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01"}
	for _, u := range []*model.User{org, other, ro} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른',2)`)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/approval-settings", h.Approval.Page, h.Auth.RequireAdminOnly)
	g.POST("/admin/approval-settings", h.Approval.Save, h.Auth.RequireAdminOnly)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/approval-settings", nil)
	req.AddCookie(jwtCookieUserClaims(t, org))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "팀장 부재") {
		t.Fatal("form")
	}

	form := url.Values{
		"deleg_by_head": {"1"}, "deleg_by_admin": {"1"},
		"self_approve_leave": {"1"}, "self_approve_quote": {"1"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, org))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d", rec.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='approval_policies'`).Scan(&n)
	if n == 0 {
		t.Fatal("audit")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/approval-settings", nil)
	req.AddCookie(jwtCookieUserClaims(t, other))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("other org own settings %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/approval-settings", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, ro))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusSeeOther {
		t.Fatalf("readonly save %d", rec.Code)
	}
	if rec.Code == http.StatusSeeOther {
		t.Fatal("readonly must not save")
	}

	vis := &model.User{Username: "va1", PasswordHash: HashPassword("pw"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, OrgID: "O01"}
	if err := users.Create(vis); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/approval-settings", nil)
	req.AddCookie(jwtCookieUserClaims(t, vis))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("vision all-org %d", rec.Code)
	}
}
