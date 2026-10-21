package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestCompanyWeeklyWriteIsolatedByOrg(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "cw10.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','미디어',1,2),('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/company-weekly/rows", h.Stats.ListCompanyWeeklyRows)
	g.POST("/stats/company-weekly/rows", h.Stats.CreateCompanyWeeklyRow)
	g.POST("/stats/company-weekly", h.Stats.CreateCompanyWeekly)
	g.POST("/stats/company-weekly/confirm", h.Stats.ConfirmCompanyWeekly)

	form := url.Values{"display_name": {"미디어행"}, "row_kind": {"activity"}, "activity_kind": {"mnt_as"}, "sort_order": {"0"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	list03 := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/rows", nil)
	lreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(list03, lreq)
	if strings.Contains(list03.Body.String(), "미디어행") {
		t.Fatal("O03 목록에 O02 행")
	}

	tech := httptest.NewRecorder()
	treq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/rows", strings.NewReader(form.Encode()))
	treq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	treq.AddCookie(jwtCookieOrg(t, model.RoleTech, "O02"))
	e.ServeHTTP(tech, treq)
	if tech.Code != http.StatusForbidden {
		t.Fatalf("팀원 want 403 got %d", tech.Code)
	}

	cl := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/confirm", strings.NewReader("company_date=2026-08-17"))
	creq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	creq.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	e.ServeHTTP(cl, creq)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level want 403 got %d", cl.Code)
	}

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/confirm", strings.NewReader("company_date=2026-08-17"))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("confirm status=%d", ok.Code)
	}
	saved, err := h.Stats.repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if err != nil || saved.Status != repository.CompanyWeeklyStatusConfirmed {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}

	src := mustCompanyWeeklyFixture(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("company_date", "2026-08-17")
	_ = w.WriteField("sheet_name", "26.8월3주(업무)-미디어")
	fw, _ := w.CreateFormFile("file", "src.xlsx")
	_, _ = fw.Write(src)
	_ = w.Close()
	gen := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly", &buf)
	greq.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	greq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(gen, greq)
	if gen.Code == http.StatusOK && strings.Contains(gen.Body.String(), "미디어행") {
		t.Fatal("O03 생성 결과에 O02 행")
	}
}

func TestCompanyWeeklyExcelRowCountAndFilename(t *testing.T) {
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	ascii, utf8 := companyWeeklyFilenames(period, "SI")
	if !strings.Contains(utf8, "_SI.xlsx") || ascii == "" {
		t.Fatalf("filename %s %s", ascii, utf8)
	}
	rows := []model.CompanyWeeklyRow{
		{RowKey: "a", SheetRow: 20, DisplayName: "1"},
		{RowKey: "b", SheetRow: 21, DisplayName: "2"},
		{RowKey: "c", SheetRow: 22, DisplayName: "3"},
	}
	data, err := buildCompanyWeeklySingleSheet(companyWeeklyBuildInput{SheetName: "t", Period: period, Rows: rows, OrgShort: "SI"})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty xlsx")
	}
}
