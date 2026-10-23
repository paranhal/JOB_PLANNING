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

	"customer-support/internal/auditlog"
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

	obs := httptest.NewRecorder()
	oreq2 := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/confirm", strings.NewReader("company_date=2026-08-17"))
	oreq2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq2.AddCookie(jwtCookieOrg(t, model.RoleObserver, "O02"))
	e.ServeHTTP(obs, oreq2)
	if obs.Code != http.StatusForbidden {
		t.Fatalf("observer want 403 got %d", obs.Code)
	}
}

func TestCompanyWeekly24BSavedContentDownloadAudit(t *testing.T) {
	accessPath := initTestAccessDB(t)
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "cw10b.db"))
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
	h.Stats.dataDir = dir
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/reports", h.Stats.Reports)
	g.POST("/stats/company-weekly", h.Stats.CreateCompanyWeekly)
	g.POST("/stats/company-weekly/confirm", h.Stats.ConfirmCompanyWeekly)
	g.GET("/stats/company-weekly/download", h.Stats.DownloadCompanyWeekly)

	if _, err := db.Exec(`INSERT INTO weekly_report_rows(row_key, sheet_row, display_name, row_kind, org_id, activity_kind, is_active, sort_order)
		VALUES ('M1', 20, '미디어행', 'activity', 'O02', 'mnt_as', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	secret := "미디어비밀문구"
	content := repository.EncodeCompanyWeeklyContent([]model.CompanyWeeklyRow{
		{RowKey: "M1", PrevText: secret, PlanText: "계획"},
	})
	if err := h.Stats.repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "미디어주", content, "팀장", repository.CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	if err := h.Stats.repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "미디어주", content, "팀장", repository.CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}

	o02 := httptest.NewRecorder()
	r02 := httptest.NewRequest(http.MethodGet, "/stats/reports?company_date=2026-08-17", nil)
	r02.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(o02, r02)
	if o02.Code != http.StatusOK || !strings.Contains(o02.Body.String(), secret) || !strings.Contains(o02.Body.String(), "확정본") {
		t.Fatalf("O02 조회 code=%d", o02.Code)
	}

	o03 := httptest.NewRecorder()
	r03 := httptest.NewRequest(http.MethodGet, "/stats/reports?company_date=2026-08-17", nil)
	r03.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(o03, r03)
	if strings.Contains(o03.Body.String(), secret) {
		t.Fatal("O03 화면에 O02 편집 내용")
	}

	o01 := httptest.NewRecorder()
	r01 := httptest.NewRequest(http.MethodGet, "/stats/reports?company_date=2026-08-17", nil)
	r01.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(o01, r01)
	if strings.Contains(o01.Body.String(), secret) {
		t.Fatal("O01 화면에 O02 편집 내용")
	}

	cl := httptest.NewRecorder()
	rCL := httptest.NewRequest(http.MethodGet, "/stats/reports?company_date=2026-08-17&view_org_id=O02", nil)
	rCL.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	e.ServeHTTP(cl, rCL)
	if cl.Code != http.StatusOK || !strings.Contains(cl.Body.String(), secret) {
		t.Fatalf("C-Level 읽기 code=%d", cl.Code)
	}
	if strings.Contains(cl.Body.String(), ">시트 생성<") {
		t.Fatal("C-Level에게 생성 버튼")
	}

	changed := repository.EncodeCompanyWeeklyContent([]model.CompanyWeeklyRow{
		{RowKey: "M1", PrevText: "수정후문구", PlanText: "계획"},
	})
	if err := h.Stats.repo.UpsertCompanyWeeklyReport("O02", "2026-08-17", "미디어주", changed, "팀장", repository.CompanyWeeklyUpsertSave); err != nil {
		t.Fatal(err)
	}
	after, _ := h.Stats.repo.GetCompanyWeeklyReport("O02", "2026-08-17")
	if after.Status != repository.CompanyWeeklyStatusDraft {
		t.Fatalf("수정 후 draft 복귀 %s", after.Status)
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
	greq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(gen, greq)
	if gen.Code != http.StatusOK {
		t.Fatalf("generate status=%d %s", gen.Code, gen.Body.String()[:min(200, gen.Body.Len())])
	}
	token := downloadTokenFrom(gen.Body.String())
	if token == "" {
		t.Fatal("token")
	}

	tech := httptest.NewRecorder()
	treq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/download?token="+token, nil)
	treq.AddCookie(jwtCookieOrg(t, model.RoleTech, "O02"))
	e.ServeHTTP(tech, treq)
	if tech.Code != http.StatusForbidden {
		t.Fatalf("팀원 download want 403 got %d", tech.Code)
	}

	steal := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/download?token="+token, nil)
	sreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(steal, sreq)
	if steal.Code != http.StatusForbidden {
		t.Fatalf("O03 download want 403 got %d", steal.Code)
	}

	ok := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/download?token="+token, nil)
	dreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(ok, dreq)
	if ok.Code != http.StatusOK {
		t.Fatalf("O02 download %d", ok.Code)
	}

	un := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/confirm", strings.NewReader("company_date=2026-08-17&unconfirm=1"))
	ureq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ureq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(un, ureq)
	if un.Code != http.StatusSeeOther {
		t.Fatalf("unconfirm %d", un.Code)
	}

	logs := readAccessLogs(t, accessPath)
	want := map[string]bool{"전사 주간업무보고 편집 저장": false, "전사 주간업무보고 내려받기": false, "전사 주간업무보고 확인 취소": false}
	for _, row := range logs {
		if row.Target == "company_weekly_report" {
			if _, ok := want[row.Reason]; ok && row.Result == auditlog.ResultOK {
				want[row.Reason] = true
			}
			if strings.Contains(row.Before, "미디어") || strings.Contains(row.Reason, "password") {
				t.Fatalf("로그에 민감 내용: %+v", row)
			}
		}
	}
	for reason, hit := range want {
		if !hit {
			t.Fatalf("감사 로그 없음: %s / %+v", reason, logs)
		}
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
