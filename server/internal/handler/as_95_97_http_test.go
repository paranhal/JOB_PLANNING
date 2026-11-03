package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/appclock"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newCauseCatHTTP(t *testing.T) (*echo.Echo, *repository.ASRepo, *http.Cookie, *http.Cookie, *http.Cookie, *http.Cookie) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "c97.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("admin"))
	oa := &model.User{Username: "oa97", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	oa2 := &model.User{Username: "oa97b", PasswordHash: "x", FullName: "미디장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	vs := &model.User{Username: "vs97", PasswordHash: "x", FullName: "비전", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	tech := &model.User{Username: "tech97", PasswordHash: "x", FullName: "기술", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl97", PasswordHash: "x", FullName: "씨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{oa, oa2, vs, tech, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c97", "SI고객", "SI고객"); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as", h.AS.List)
	g.GET("/as/cause-categories", h.AS.CauseCats)
	g.GET("/as/cause-categories/template.xlsx", h.AS.CauseCatTemplateXLSX)
	g.GET("/as/cause-categories/export.xlsx", h.AS.CauseCatExportXLSX)
	g.POST("/as/cause-categories/import-preview", h.AS.CauseCatImportPreview)
	g.POST("/as/cause-categories/import-apply", h.AS.CauseCatImportApply)
	g.POST("/as/cause-categories/import-errors.xlsx", h.AS.CauseCatImportErrors)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/stats", h.AS.StatsDashboard)
	return e, repository.NewASRepo(db), jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, tech), jwtCookieUserClaims(t, cl)
}

func createOrgAS(t *testing.T, repo *repository.ASRepo, orgID string) string {
	t.Helper()
	src := &model.ASReceipt{
		CustomerID: "c97", ReceiptChannel: "phone", Requester: "홍",
		Symptom: "원인분류 점검", Urgency: "normal", Priority: "normal",
		AssignedTo: "SI장", ReceiptDatetime: appclock.Now(), Status: "in_progress", OrgID: orgID,
	}
	if err := repo.Create(src); err != nil {
		t.Fatal(err)
	}
	return src.ASID
}

func cause97Book(t *testing.T, rows [][]string) []byte {
	t.Helper()
	raw, err := repository.BuildCauseCatWorkbook(repository.CauseCatGuideLines(), rows, repository.CauseTypeSheetRows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func Test95_97_T2_EmptyCatalogFreeInput(t *testing.T) {
	e, asRepo, oa, _, _, _ := newCauseCatHTTP(t)
	asID := createOrgAS(t, asRepo, "O03")
	form := url.Values{
		"s1_work_place":         {"office"},
		"s1_process_type":       {"remote"},
		"s1_cause_cat1":         {"custom"},
		"s1_cause_cat1_custom":  {"직접장비97"},
		"s1_action_taken":       {"자유입력 조치 내용을 저장합니다"},
		"s1_result_code":        {model.ResultDone},
		"s1_leave_same":         {"1"},
		"work_place":            {"office"},
		"process_type":          {"remote"},
		"action_short_ok":       {"1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/as/"+asID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oa)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T2 저장 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID("O03", asID)
	if got.CauseCat1 != model.CauseCatCustom || got.CauseCat1Custom != "직접장비97" {
		t.Fatalf("%s %s", got.CauseCat1, got.CauseCat1Custom)
	}

	asID2 := createOrgAS(t, asRepo, "O03")
	form2 := cloneForm97(form)
	form2.Del("s1_cause_cat1_custom")
	form2.Set("s1_cause_cat1", "custom")
	form2.Set("s1_action_taken", "1차 비움 조치 내용입니다")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/as/"+asID2+"/update", strings.NewReader(form2.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oa)
	e.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "cause_cat1_input") {
		t.Fatalf("T2 필수 loc=%s", loc)
	}
	page := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, loc, nil)
	if !strings.HasPrefix(loc, "http") {
		greq = httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	}
	greq.AddCookie(oa)
	e.ServeHTTP(page, greq)
	if !strings.Contains(page.Body.String(), "조치 1:") || !strings.Contains(page.Body.String(), "1차 분류를 입력하세요") {
		t.Fatal("안내 문구")
	}
}

func cloneForm97(v url.Values) url.Values {
	out := url.Values{}
	for k, vals := range v {
		out[k] = append([]string{}, vals...)
	}
	return out
}

func Test95_97_T6_Perm(t *testing.T) {
	e, _, oa, vs, tech, cl := newCauseCatHTTP(t)
	if doGetWith(t, e, "/as/cause-categories/template.xlsx", oa).Code != http.StatusOK {
		t.Fatal("조직관리자 양식")
	}
	for _, ck := range []*http.Cookie{vs, tech, cl} {
		if doGetWith(t, e, "/as/cause-categories", ck).Code != http.StatusForbidden {
			t.Fatalf("403 페이지 %v", ck.Value[:8])
		}
		if doGetWith(t, e, "/as/cause-categories/template.xlsx", ck).Code != http.StatusForbidden {
			t.Fatal("403 양식")
		}
	}
	listOA := doGetWith(t, e, "/as", oa)
	if listOA.Code != http.StatusOK || !strings.Contains(listOA.Body.String(), "/as/cause-categories") {
		t.Fatal("메뉴 조직관리자")
	}
	listTech := doGetWith(t, e, "/as", tech)
	if listTech.Code == http.StatusOK && strings.Contains(listTech.Body.String(), "/as/cause-categories") {
		t.Fatal("기술 계정에 메뉴")
	}
	listVS := doGetWith(t, e, "/as", vs)
	if listVS.Code == http.StatusOK && strings.Contains(listVS.Body.String(), "/as/cause-categories") {
		t.Fatal("비전에 메뉴")
	}
}

func Test95_97_T5_HTTPPreviewApply(t *testing.T) {
	e, asRepo, oa, _, _, _ := newCauseCatHTTP(t)
	raw := cause97Book(t, [][]string{
		{"1차 분류", "2차 분류", "3차 분류", "장애유형(선택)", "장애 여부(Y/N, 선택)", "정렬순서(선택)", "사용여부(Y/N, 기본 Y)"},
		{"방송", "송출", "신호", "sw", "Y", "10", "Y"},
		{"", "잘못", "", "", "", "", ""},
	})
	prev := postEquipFile(t, e, "/as/cause-categories/import-preview", "file", "t.xlsx", raw, oa)
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), "미리보기") {
		t.Fatalf("미리보기 %d", prev.Code)
	}
	if asRepoCountOrgs(t, asRepo) != 0 {
		t.Fatal("미리보기 쓰기")
	}
	app := postEquipFile(t, e, "/as/cause-categories/import-apply", "file", "t.xlsx", raw, oa)
	if app.Code != http.StatusOK {
		t.Fatalf("적용 %d", app.Code)
	}
	if asRepoCountOrgs(t, asRepo) == 0 {
		t.Fatal("적용 0")
	}
}

func asRepoCountOrgs(t *testing.T, r *repository.ASRepo) int {
	t.Helper()
	cats, _ := r.ListCauseCategoriesForOrgAll("O03")
	return len(cats)
}

func Test95_97_T7_ActionCascadeAndInactiveLabel(t *testing.T) {
	e, asRepo, oa, _, _, _ := newCauseCatHTTP(t)
	raw := cause97Book(t, [][]string{
		{"1차 분류", "2차 분류", "3차 분류", "장애유형(선택)", "장애 여부(Y/N, 선택)", "정렬순서(선택)", "사용여부(Y/N, 기본 Y)"},
		{"방송", "송출", "신호", "sw", "Y", "10", "Y"},
	})
	if postEquipFile(t, e, "/as/cause-categories/import-apply", "file", "t.xlsx", raw, oa).Code != http.StatusOK {
		t.Fatal("적용")
	}
	asID := createOrgAS(t, asRepo, "O03")
	page := doGetWith(t, e, "/as/"+asID+"/action", oa)
	if page.Code != http.StatusOK {
		t.Fatalf("action %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "방송") || !strings.Contains(body, `data-cause-wrap="2"`) {
		t.Fatal("연쇄 선택 없음")
	}
	if strings.Contains(body, "조직관리자가 원인분류표를 등록하면") {
		t.Fatal("자유 입력 안내가 남아 있다")
	}
	c1 := model.CauseOrgPathCode("O03", "방송")
	c2 := model.CauseOrgPathCode("O03", "방송", "송출")
	c3 := model.CauseOrgPathCode("O03", "방송", "송출", "신호")
	form := url.Values{
		"s1_work_place":   {"office"},
		"s1_process_type": {"remote"},
		"s1_cause_cat1":   {c1},
		"s1_cause_cat2":   {c2},
		"s1_cause_cat3":   {c3},
		"s1_action_taken": {"분류표로 저장한 조치 내용입니다"},
		"s1_result_code":  {model.ResultDone},
		"s1_leave_same":   {"1"},
		"work_place":      {"office"},
		"process_type":    {"remote"},
		"action_short_ok": {"1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/as/"+asID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oa)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %s", rec.Header().Get("Location"))
	}
	off := cause97Book(t, [][]string{
		{"1차 분류", "2차 분류", "3차 분류", "장애유형(선택)", "장애 여부(Y/N, 선택)", "정렬순서(선택)", "사용여부(Y/N, 기본 Y)"},
		{"방송", "송출", "신호", "sw", "Y", "10", "N"},
	})
	if postEquipFile(t, e, "/as/cause-categories/import-apply", "file", "off.xlsx", off, oa).Code != http.StatusOK {
		t.Fatal("비활성")
	}
	show := doGetWith(t, e, "/as/"+asID, oa)
	act := doGetWith(t, e, "/as/"+asID+"/action", oa)
	if !strings.Contains(show.Body.String()+act.Body.String(), "방송") {
		t.Fatalf("비활성 뒤 라벨 show=%d action=%d", show.Code, act.Code)
	}
}

func Test95_97_T8_CountCauseCat1Org(t *testing.T) {
	e, asRepo, oa, _, _, _ := newCauseCatHTTP(t)
	raw := cause97Book(t, [][]string{
		{"1차 분류", "2차 분류", "3차 분류", "장애유형(선택)", "장애 여부(Y/N, 선택)", "정렬순서(선택)", "사용여부(Y/N, 기본 Y)"},
		{"집계1차", "집계2차", "", "hw", "Y", "1", "Y"},
	})
	if postEquipFile(t, e, "/as/cause-categories/import-apply", "file", "t.xlsx", raw, oa).Code != http.StatusOK {
		t.Fatal("적용")
	}
	asID := createOrgAS(t, asRepo, "O03")
	c1 := model.CauseOrgPathCode("O03", "집계1차")
	c2 := model.CauseOrgPathCode("O03", "집계1차", "집계2차")
	form := url.Values{
		"s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_cause_cat1": {c1}, "s1_cause_cat2": {c2},
		"s1_action_taken": {"집계용 조치 내용을 남깁니다"}, "s1_result_code": {model.ResultDone},
		"s1_leave_same": {"1"}, "work_place": {"office"}, "process_type": {"remote"}, "action_short_ok": {"1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/as/"+asID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oa)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %s", rec.Header().Get("Location"))
	}
	counts, err := asRepo.CountCauseCat1("O03")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range counts {
		if c.Label == "집계1차" && c.N >= 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", counts)
	}
}
