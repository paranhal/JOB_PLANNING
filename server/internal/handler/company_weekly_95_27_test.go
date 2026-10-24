package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func mountWeeklyUI(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/stats/company-weekly", h.Stats.ShowWeeklyReport)
	g.GET("/stats/company-weekly/export", h.Stats.ExportWeeklyReportSheet)
	g.POST("/stats/company-weekly/generate", h.Stats.GenerateWeeklyReport)
	g.POST("/stats/company-weekly/save", h.Stats.SaveWeeklyReport)
	g.POST("/stats/company-weekly/progress", h.Stats.SaveWeeklyProgress)
	g.POST("/stats/company-weekly/ui-confirm", h.Stats.ConfirmWeeklyReportUI)
}

func TestWeekInfoMatchesSheetAndScreen(t *testing.T) {
	mon := time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local)
	w := repository.WeekInfoOf(mon)
	if w.Month != 10 || w.WeekN != 4 {
		t.Fatalf("%d월 %d주차", w.Month, w.WeekN)
	}
}

func TestWeeklyReportToSheetHeadersAndBullets(t *testing.T) {
	v := weeklyReportView{
		WeekInfo: model.WeekInfo{Month: 10, WeekN: 4, Year: 2026,
			Monday: time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local),
			Sunday: time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local)},
		OrgShort: "SI사업팀",
		Cards: []weeklyReportCard{{
			HasContent: true, Title: "테크센터", Customer: "충남교육청", Kind: "project",
			Done: []repository.CompanyWeeklyLine{{Text: "점검 1건"}},
			Plan: []repository.CompanyWeeklyLine{{Text: "산출물"}},
		}, {
			HasContent: true, Title: "업무지원활동", Kind: "activity",
			Done: []repository.CompanyWeeklyLine{{Text: "지원"}},
		}, {
			HasContent: false, Title: "빈행",
		}},
	}
	rows := weeklyReportToSheetRows(v)
	if len(rows) != 2 {
		t.Fatalf("빈 행 제외 want 2 got %d", len(rows))
	}
	if rows[1].Name != "## 업무지원활동" {
		t.Fatalf("활동 접두 %q", rows[1].Name)
	}
	if !strings.HasPrefix(rows[0].Done, "* ") || !strings.Contains(rows[0].Start, "(월)") {
		t.Fatalf("줄/날짜 %q %q", rows[0].Done, rows[0].Start)
	}
	data, err := buildCompanyWeeklyTeamWorkbook([]weeklyReportView{v})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	if sheet != "SI사업팀" {
		t.Fatalf("시트 %s", sheet)
	}
	for i, h := range companyWeeklySheetHeads {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		got, _ := f.GetCellValue(sheet, cell)
		if got != h {
			t.Fatalf("머리글 %s=%q want %q", cell, got, h)
		}
	}
	name, _ := f.GetCellValue(sheet, "I3")
	if name != "## 업무지원활동" {
		t.Fatalf("I3=%q", name)
	}
	if form, _ := f.GetCellFormula(sheet, "K2"); form != "" {
		t.Fatal("수식이 들어갔다")
	}
}

func TestWeeklyExcelFormulaInjectionIsString(t *testing.T) {
	v := weeklyReportView{
		WeekInfo: model.WeekInfo{Month: 10, WeekN: 4, Year: 2026,
			Monday: time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local),
			Sunday: time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local)},
		OrgShort: "도서관사업팀",
		Cards: []weeklyReportCard{{
			HasContent: true, Title: "사업", Kind: "project",
			Done: []repository.CompanyWeeklyLine{{Text: "=SUM(A1)"}},
		}},
	}
	data, err := buildCompanyWeeklyTeamWorkbook([]weeklyReportView{v})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	if form, _ := f.GetCellFormula(sheet, "K2"); form != "" {
		t.Fatal("주입 수식")
	}
	val, _ := f.GetCellValue(sheet, "K2")
	if !strings.Contains(val, "SUM") {
		t.Fatalf("값 %q", val)
	}
}

func TestWeeklyProgressManualAndAutoHTTP(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw27.db"))
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
	mountWeeklyUI(e, h)

	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "done": {"120"}, "total": {"199"}, "cycle": {"반기"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("O03 진행율 %d %s", rec.Code, rec.Body.String())
	}
	saved, err := h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
	if err != nil || saved.ReportID == "" {
		t.Fatalf("저장 안 됨 %+v %v", saved, err)
	}
	doc := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	if doc.Progress == nil || doc.Progress.Pct == nil || *doc.Progress.Pct != 60.3 {
		t.Fatalf("자동 %% %+v", doc.Progress)
	}

	bad := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "pct": {"101"}}
	br := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(bad.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(br, breq)
	if br.Code != http.StatusBadRequest {
		t.Fatalf("101 want 400 got %d", br.Code)
	}

	auto := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(form.Encode()))
	areq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	areq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(auto, areq)
	if auto.Code != http.StatusForbidden {
		t.Fatalf("O01 자동 입력 want 403 got %d", auto.Code)
	}

	oth := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(form.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O02"))
	e.ServeHTTP(oth, oreq)
	if oth.Code != http.StatusForbidden && oth.Code != http.StatusNotFound {
		t.Fatalf("타 조직 팀장 %d", oth.Code)
	}

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O03", nil)
	greq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(get, greq)
	if get.Code != 200 || !strings.Contains(get.Body.String(), "작성자 입력") {
		t.Fatalf("화면 %d %s", get.Code, get.Body.String()[:min(400, len(get.Body.String()))])
	}
	if strings.Contains(get.Body.String(), "png") && strings.Contains(strings.ToLower(get.Body.String()), "png 저장") {
		t.Fatal("PNG 저장")
	}

	lib := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O01", nil)
	lreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	e.ServeHTTP(lib, lreq)
	if lib.Code != 200 || !strings.Contains(lib.Body.String(), "자동 집계") {
		t.Fatalf("도서관 자동 %d", lib.Code)
	}
	if strings.Contains(lib.Body.String(), `name="done"`) && strings.Contains(lib.Body.String(), "정기점검 진행율 (작성자 입력)") {
		t.Fatal("도서관에 진행율 입력 칸")
	}

	x := httptest.NewRecorder()
	xreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O01", nil)
	xreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(x, xreq)
	if x.Code != http.StatusNotFound && x.Code != http.StatusForbidden {
		t.Fatalf("O03이 O01 탭 %d", x.Code)
	}

	cl := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(form.Encode()))
	creq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	creq.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	e.ServeHTTP(cl, creq)
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level 입력 %d", cl.Code)
	}
}

func TestWeeklyReportViewEqualsExcelRows(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw27v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	v, err := buildWeeklyReportView(repository.NewStatsRepo(db), model.OrgIDLibrary, time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local), false)
	if err != nil {
		t.Fatal(err)
	}
	rows := weeklyReportToSheetRows(v)
	n := 0
	for _, c := range v.Cards {
		if c.HasContent {
			n++
		}
	}
	if n != len(rows) {
		t.Fatalf("화면 내용행 %d 엑셀 %d", n, len(rows))
	}
}

func TestOldJSONWeeklyDocCompat(t *testing.T) {
	doc := repository.ParseCompanyWeeklyDoc(`{"401":{"prev":"옛줄","plan":"계획","help":"지원","decision":"결정"}}`)
	if doc.Rows["401"].Prev != "옛줄" || doc.Progress != nil {
		t.Fatalf("%+v", doc)
	}
}

func TestWeeklyReportMemberReadAndExport403(t *testing.T) {
	accessPath := initTestAccessDB(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw27m.db"))
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
	mountWeeklyUI(e, h)

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O03", nil)
	greq.AddCookie(jwtCookieOrg(t, model.RoleTech, "O03"))
	e.ServeHTTP(get, greq)
	if get.Code != 200 {
		t.Fatalf("팀원 읽기 %d", get.Code)
	}
	if strings.Contains(get.Body.String(), "이 주차 보고서 자동 생성") && strings.Contains(get.Body.String(), `name="reaggregate"`) {
		t.Fatal("팀원에게 편집")
	}
	if !strings.Contains(get.Body.String(), "weekly-card") && !strings.Contains(get.Body.String(), "이 주차에 보고 내용이 없습니다") {
		t.Fatalf("빈 주차 렌더 없음")
	}
	if !strings.Contains(get.Body.String(), "weekly-card-cols") && !strings.Contains(get.Body.String(), "grid-cols-2") {
		t.Fatal("모바일 폭 마크업 없음")
	}

	x := httptest.NewRecorder()
	xreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/export?week=2026-10-19&org=O01&scope=team", nil)
	xreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(x, xreq)
	if x.Code != http.StatusForbidden && x.Code != http.StatusNotFound {
		t.Fatalf("타 조직 엑셀 %d", x.Code)
	}

	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly/export?week=2026-10-19&org=O03&scope=team", nil)
	oreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(ok, oreq)
	if ok.Code != 200 {
		t.Fatalf("팀 엑셀 %d %s", ok.Code, ok.Body.String())
	}
	logs := readAccessLogs(t, accessPath)
	foundLog := false
	for _, row := range logs {
		if row.Action == auditlog.ActionDownload && (row.Target == "company_weekly_report" || strings.Contains(row.Reason, "주간업무보고")) {
			foundLog = true
		}
	}
	if !foundLog {
		t.Fatalf("감사 로그 없음 %+v", logs)
	}

	cl := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week=2026-10-19&org=O03", nil)
	creq.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	e.ServeHTTP(cl, creq)
	if cl.Code != 200 {
		t.Fatalf("C-Level 읽기 %d", cl.Code)
	}

	save := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/save", strings.NewReader(url.Values{
		"week": {"2026-10-19"}, "org": {"O03"},
	}.Encode()))
	sreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	sreq.AddCookie(jwtCookieOrg(t, model.RoleTech, "O03"))
	e.ServeHTTP(save, sreq)
	if save.Code != http.StatusForbidden {
		t.Fatalf("팀원 저장 %d", save.Code)
	}
}

func TestWeeklyProgressModeSwitch(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw27mode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewStatsRepo(db)
	if err := repo.SetWeeklyProgressMode("O03", repository.WeeklyProgressModeAuto); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeeklyUI(e, h)
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "pct": {"50"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/progress", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("auto 전환 후 입력 %d", rec.Code)
	}
}

func TestWeeklyHideRestoreAndConfirmExtras(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw27h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := repository.NewStatsRepo(db)
	anchor := "2026-08-17"
	auto := []repository.CompanyWeeklyLine{{Key: "task:1", Text: "자동A"}, {Key: "task:2", Text: "자동B"}}
	lines, hidden := applyEditedLines("고친A\n자동B", auto, nil, nil)
	saw := false
	for _, l := range lines {
		if l.Key == "task:1" && l.Manual && l.Text == "고친A" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("수정 전환 %+v", lines)
	}
	if len(hidden) != 0 {
		t.Fatalf("수정만 했는데 숨김 %+v", hidden)
	}
	_, hidden = applyEditedLines("자동A", auto, nil, nil)
	if len(hidden) != 1 || hidden[0] != "task:2" {
		t.Fatalf("숨김 %+v", hidden)
	}
	lines = []repository.CompanyWeeklyLine{{Key: "task:1", Text: "고친A", Manual: true}}
	doc := repository.CompanyWeeklyDoc{Rows: map[string]repository.CompanyWeeklySavedRow{
		"401": {DoneLines: lines, HiddenDone: hidden, PlanLines: auto},
	}}
	if err := repo.UpsertCompanyWeeklyReport(model.OrgIDLibrary, anchor, "s",
		repository.EncodeCompanyWeeklyDoc(doc), "테스터", repository.CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}
	saved, _ := repo.GetCompanyWeeklyReport(model.OrgIDLibrary, anchor)
	if saved.Status != repository.CompanyWeeklyStatusConfirmed {
		t.Fatalf("status %s", saved.Status)
	}
	parsed := repository.ParseCompanyWeeklyDoc(saved.ContentJSON)
	parsed.Rows["401"] = repository.CompanyWeeklySavedRow{
		DoneLines: parsed.Rows["401"].DoneLines, HiddenDone: parsed.Rows["401"].HiddenDone,
		PlanLines: parsed.Rows["401"].PlanLines, SupportNeeds: "회의 지원", Decision: "결정",
		DecisionLog: []repository.CompanyWeeklyLog{{At: "2026-10-24 10:00:00", By: "테스터", Text: "주요 의사결정"}},
	}
	if err := repo.UpsertCompanyWeeklyReport(model.OrgIDLibrary, anchor, "s",
		repository.EncodeCompanyWeeklyDoc(parsed), "테스터", repository.CompanyWeeklyUpsertConfirm); err != nil {
		t.Fatal(err)
	}
	again, _ := repo.GetCompanyWeeklyReport(model.OrgIDLibrary, anchor)
	if again.Status != repository.CompanyWeeklyStatusConfirmed {
		t.Fatalf("지원/결정 후 상태 %s", again.Status)
	}
	got := repository.MergeWeeklyLinesKeyed(auto, parsed.Rows["401"].DoneLines, parsed.Rows["401"].HiddenDone)
	if len(got) != 1 {
		t.Fatalf("재집계 숨김 유지 %+v", got)
	}
}
