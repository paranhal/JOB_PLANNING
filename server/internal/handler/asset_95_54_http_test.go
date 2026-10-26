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

func newAsset54Server(t *testing.T) (*echo.Echo, *repository.AssetRepo, *http.Cookie) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a54.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	admin, err := users.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin")
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-a54','자산기관','자산기관',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/assets", h.Asset.List)
	g.GET("/assets/new", h.Asset.New)
	g.POST("/assets", h.Asset.Create)
	g.GET("/assets/:id/edit", h.Asset.Edit)
	g.POST("/assets/:id/update", h.Asset.Update)
	g.GET("/assets/export.xlsx", h.Asset.ExportExcel)
	return e, h.Asset.repo, jwtCookieUserClaims(t, admin)
}

func TestAssetFormManagementTypesOnly3(t *testing.T) {
	e, _, ck := newAsset54Server(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/new", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("new %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="fault"`) || !strings.Contains(body, `value="periodic"`) || !strings.Contains(body, `value="third_party"`) {
		t.Fatal("활성 3종이 없다")
	}
	if strings.Contains(body, `value="direct"`) || strings.Contains(body, `value="on_demand"`) {
		t.Fatal("비활성 관리유형이 선택지에 있다")
	}
	if !strings.Contains(body, `value="call"`) {
		t.Fatal("점검주기 CALL 코드 없음")
	}
	if !strings.Contains(body, `cyclePresets: ['monthly','quarterly','semi','odd_bimonthly','even_bimonthly','call']`) {
		t.Fatal("Alpine presets 에 call 없음")
	}
}

func TestAssetCreateNoneEmptyCycleBecomesCall(t *testing.T) {
	e, repo, ck := newAsset54Server(t)
	form := url.Values{
		"customer_id":         {"c-a54"},
		"product_name":        {"게이트"},
		"install_location":    {"1층"},
		"maint_contract_type": {"none"},
		"management_type":     {"fault"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d", rec.Code)
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/assets/")
	got, err := repo.GetByID(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.MaintCycle != "call" {
		t.Fatalf("주기 %q", got.MaintCycle)
	}
}

func TestAssetCreateNoneMonthlyKept(t *testing.T) {
	e, repo, ck := newAsset54Server(t)
	form := url.Values{
		"customer_id":         {"c-a54"},
		"product_name":        {"게이트월"},
		"install_location":    {"1층"},
		"maint_contract_type": {"none"},
		"maint_cycle_code":    {"monthly"},
		"management_type":     {"periodic"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/assets/")
	got, _ := repo.GetByID(id)
	if got == nil || got.MaintCycle != "monthly" {
		t.Fatalf("직접 월 유지 %+v", got)
	}
}

func TestAssetEditShowsLegacyAndKeepsOnInvalidSave(t *testing.T) {
	e, repo, ck := newAsset54Server(t)
	a := &model.Asset{
		CustomerID: "c-a54", ProductName: "구유형", InstallLocation: "2층",
		ManagementType: "direct", MaintContractType: "paid", MaintCycle: "monthly",
	}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	edit := httptest.NewRecorder()
	ereq := httptest.NewRequest(http.MethodGet, "/assets/"+a.AssetID+"/edit", nil)
	ereq.AddCookie(ck)
	e.ServeHTTP(edit, ereq)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit %d", edit.Code)
	}
	if !strings.Contains(edit.Body.String(), "직접유지보수 (구)") {
		t.Fatal("(구) 표시 없음")
	}
	if !strings.Contains(edit.Body.String(), `value="monthly"`) {
		t.Fatal("저장 주기 유지")
	}

	form := url.Values{
		"customer_id":         {"c-a54"},
		"product_name":        {"구유형"},
		"install_location":    {"2층"},
		"management_type":     {"direct"},
		"maint_contract_type": {"paid"},
		"maint_cycle_code":    {"monthly"},
	}
	up := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodPost, "/assets/"+a.AssetID+"/update", strings.NewReader(form.Encode()))
	ureq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ureq.AddCookie(ck)
	e.ServeHTTP(up, ureq)
	got, _ := repo.GetByID(a.AssetID)
	if got.ManagementType != "direct" {
		t.Fatalf("구값 유지 실패 %q", got.ManagementType)
	}
	flash := false
	for _, c := range up.Result().Cookies() {
		if c.Name == flashErrCookie && c.Value != "" {
			flash = true
		}
	}
	if !flash {
		t.Fatal("안내 메시지 쿠키 없음")
	}

	form.Set("management_type", "periodic")
	up2 := httptest.NewRecorder()
	ureq2 := httptest.NewRequest(http.MethodPost, "/assets/"+a.AssetID+"/update", strings.NewReader(form.Encode()))
	ureq2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ureq2.AddCookie(ck)
	e.ServeHTTP(up2, ureq2)
	got, _ = repo.GetByID(a.AssetID)
	if got.ManagementType != "periodic" {
		t.Fatalf("3종으로 저장 %q", got.ManagementType)
	}
}

func TestAssetListAndExportCallLabel(t *testing.T) {
	e, repo, ck := newAsset54Server(t)
	a := &model.Asset{
		CustomerID: "c-a54", ProductName: "콜장비", InstallLocation: "로비",
		ManagementType: "fault", MaintContractType: "none", MaintCycle: "Call ",
	}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/assets?search=콜장비", nil)
	lreq.AddCookie(ck)
	e.ServeHTTP(list, lreq)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "콜장비") {
		t.Fatalf("목록 %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "CALL") {
		t.Fatal("목록 CALL 라벨 없음")
	}
	if excelMaintCycleLabel("Call ") != "CALL" {
		t.Fatal("엑셀 라벨")
	}
}

func TestApplyAssetBusinessRulesExcelPathNoneCycle(t *testing.T) {
	a := &model.Asset{MaintContractType: "none", ManagementType: "fault"}
	applyAssetBusinessRules(a)
	if a.MaintCycle != "call" {
		t.Fatalf("엑셀·저장 공용 헬퍼 %q", a.MaintCycle)
	}
	b := &model.Asset{ManagementType: model.ManagementTypeThirdParty, IsManaged: true}
	applyAssetBusinessRules(b)
	if b.IsManaged {
		t.Fatal("타사장비 규칙")
	}
}

func TestCustomerExcelCallLabel(t *testing.T) {
	items := []model.CustomerListItem{{CustomerID: "C1", OrgName: "도서관", IsActive: true, AssetCount: 1}}
	maint := []model.Asset{{
		AssetID: "A1", CustomerID: "C1", OrgName: "도서관", ProductName: "게이트",
		MaintContractType: "none", MaintCycle: "Call ",
	}}
	f, err := buildCustomerExcelWorkbook(items, maint)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cycle, err := f.GetCellValue("유지보수계약", "J2")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "CALL" {
		t.Fatalf("고객 엑셀 주기 %q", cycle)
	}
}
