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

func TestCompanyWeeklyHTTPOrgIsolationAndNeedOrg(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/company-weekly/rows", h.Stats.ListCompanyWeeklyRows)
	g.POST("/stats/company-weekly/rows", h.Stats.CreateCompanyWeeklyRow)
	g.POST("/stats/company-weekly/rows/:key/update", h.Stats.UpdateCompanyWeeklyRow)
	g.POST("/stats/company-weekly", h.Stats.CreateCompanyWeekly)
	g.GET("/stats/weekly-report.xlsx", h.Stats.ExportWeeklyReport)
	g.GET("/meeting/weekly", h.Meeting.Weekly)

	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/rows", nil)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(list, req)
	if list.Code != 200 {
		t.Fatalf("rows %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "타 조직 사업") {
		t.Fatal("행 관리 사업 목록에 타 조직 사업 배지가 없다")
	}

	form := url.Values{"display_name": {"SI행"}, "row_kind": {"activity"}, "activity_kind": {"mnt_as"}}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows", strings.NewReader(form.Encode()))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	r.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create row %d %s", rec.Code, rec.Body.String())
	}

	upd := httptest.NewRecorder()
	u := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows/X02/update", strings.NewReader(form.Encode()))
	u.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	u.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(upd, u)
	if upd.Code != http.StatusNotFound {
		t.Fatalf("타 조직 행 want 404 got %d body=%s", upd.Code, upd.Body.String())
	}

	vis := httptest.NewRecorder()
	vr := httptest.NewRequest(http.MethodPost, "/stats/company-weekly", strings.NewReader("company_date=2026-08-17"))
	vr.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	vr.AddCookie(jwtCookieRole(t, model.RoleVisionAdmin))
	e.ServeHTTP(vis, vr)
	if vis.Code != http.StatusForbidden && vis.Code != http.StatusBadRequest {
		t.Fatalf("vision 전체보기 생성 want 금지 got %d %s", vis.Code, vis.Body.String())
	}
	if vis.Code == http.StatusForbidden && !strings.Contains(vis.Body.String(), "조직") {
		t.Log(vis.Body.String())
	}

	xlsx := httptest.NewRecorder()
	xr := httptest.NewRequest(http.MethodGet, "/stats/weekly-report.xlsx?date=2026-08-10", nil)
	xr.AddCookie(jwtCookieRole(t, model.RoleVisionAdmin))
	e.ServeHTTP(xlsx, xr)
	if xlsx.Code == http.StatusOK {
		t.Fatal("vision 전체보기로 통계 주간 엑셀이 내려갔다")
	}
}
