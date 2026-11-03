package handler

import (
	"bytes"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func mountAsset62(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/assets", h.Asset.List)
	g.GET("/assets/import", h.Asset.ImportForm)
	g.GET("/assets/import/template.xlsx", h.Asset.ImportTemplateXLSX)
	g.POST("/assets/import-preview", h.Asset.ImportPreview)
	g.POST("/assets/import-apply", h.Asset.ImportApply)
	g.GET("/assets/import/result.csv", h.Asset.ImportResultCSV)
}

func seedGanamO03(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C-GNAM','가남초등학교','가남초등학교',1,'O03','customer')`); err != nil {
		t.Fatal(err)
	}
}

func ganamXLSX(t *testing.T) []byte {
	t.Helper()
	raw, err := repository.BuildGanam27Workbook()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeGanamFixtures(t *testing.T, raw []byte) {
	t.Helper()
	root := findTemplateRoot(t)
	paths := []string{
		filepath.Join(root, "..", "..", "testdata", "SI사업장비_업로드_가남초_27건.xlsx"),
		filepath.Join(root, "..", "..", "..", "testdata", "SI사업장비_업로드_가남초_27건.xlsx"),
		filepath.Join(root, "..", "..", "..", "docs", "SI사업장비_업로드_가남초_27건.xlsx"),
		filepath.Join(root, "..", "..", "..", "docs", "양식", "설치자산_업로드양식.xlsx"),
	}
	tpl, err := repository.BuildAssetImportTemplate()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		body := raw
		if strings.HasSuffix(p, "설치자산_업로드양식.xlsx") {
			body = tpl
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func postAssetXLSX(t *testing.T, e *echo.Echo, path string, raw []byte, ck *http.Cookie, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "ganam.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(raw); err != nil {
		t.Fatal(err)
	}
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_62_ListHasUploadButtonsAndTemplate(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a62list.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountAsset62(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("list %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "엑셀 업로드") || !strings.Contains(body, "양식 내려받기") {
		t.Fatal("목록 버튼 없음")
	}
	tpl := httptest.NewRecorder()
	treq := httptest.NewRequest(http.MethodGet, "/assets/import/template.xlsx", nil)
	treq.AddCookie(ck)
	e.ServeHTTP(tpl, treq)
	if tpl.Code != 200 {
		t.Fatalf("양식 %d", tpl.Code)
	}
	raw := tpl.Body.Bytes()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hdr, err := f.GetRows("자산입력")
	if err != nil || len(hdr) == 0 {
		t.Fatalf("자산입력 %v", err)
	}
	head := strings.Join(hdr[0], " ")
	if !strings.Contains(head, "건물명") || !strings.Contains(head, "층") || !strings.Contains(head, "호실") {
		t.Fatalf("열 없음 %s", head)
	}
	codes, _ := f.GetRows("코드표")
	blob := ""
	for _, r := range codes {
		blob += strings.Join(r, " ")
	}
	if strings.Contains(blob, "직접유지보수") || strings.Contains(blob, "요청시지원") {
		t.Fatal("코드표에 구 관리유형이 남아 있다")
	}
	if !strings.Contains(blob, "장애대응") || !strings.Contains(blob, "정기점검") || !strings.Contains(blob, "타사장비") {
		t.Fatal("관리유형 3종이 없다")
	}
}

func Test95_62_Ganam27PreviewApplySkipAndMissingCustomer(t *testing.T) {
	raw := ganamXLSX(t)
	writeGanamFixtures(t, raw)

	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a62g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedGanamO03(t, db)
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountAsset62(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")

	prev := postAssetXLSX(t, e, "/assets/import-preview", raw, ck, map[string]string{"create_spaces": "1"})
	if prev.Code != 200 {
		t.Fatalf("미리보기 %d %s", prev.Code, prev.Body.String()[:min(400, prev.Body.Len())])
	}
	pb := prev.Body.String()
	if strings.Contains(pb, `data-asset-import-result`) && strings.Count(pb, ">error<") >= 27 {
		t.Fatal("고객이 있는데 미리보기가 전부 오류")
	}
	if !strings.Contains(pb, "검증 미리보기") || !strings.Contains(pb, "확인하고 등록") {
		t.Fatal("미리보기 화면 아님")
	}
	if countAssets(t, db) != 0 {
		t.Fatalf("미리보기 저장 %d", countAssets(t, db))
	}

	tok := previewToken(pb)
	if tok == "" {
		t.Fatal("preview_token 없음")
	}
	apply := httptest.NewRecorder()
	form := strings.NewReader("preview_token=" + tok + "&create_spaces=1")
	areq := httptest.NewRequest(http.MethodPost, "/assets/import-apply", form)
	areq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	areq.AddCookie(ck)
	e.ServeHTTP(apply, areq)
	if apply.Code != 200 {
		t.Fatalf("적용 %d %s", apply.Code, apply.Body.String()[:min(400, apply.Body.Len())])
	}
	ab := apply.Body.String()
	if !strings.Contains(ab, "등록 결과") || !strings.Contains(ab, "설치자산 업로드 결과") {
		t.Fatal("결과 팝업 문구 없음")
	}
	if countAssets(t, db) != 27 {
		t.Fatalf("등록 %d", countAssets(t, db))
	}
	assertGanamCounts(t, db)

	prev2 := postAssetXLSX(t, e, "/assets/import-preview", raw, ck, nil)
	if strings.Count(prev2.Body.String(), "이미 등록") < 27 {
		t.Fatalf("재업로드 건너뜀 부족: %s", prev2.Body.String()[:min(800, prev2.Body.Len())])
	}

	db2, err := repository.InitDB(filepath.Join(t.TempDir(), "a62miss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	repository.NewUserRepo(db2).EnsureAdmin(HashPassword("admin"))
	if _, err := db2.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	e2 := echo.New()
	e2.Renderer = NewRenderer()
	h2 := New(db2)
	mountAsset62(e2, h2)
	miss := postAssetXLSX(t, e2, "/assets/import-preview", raw, ck, nil)
	if strings.Count(miss.Body.String(), "등록된 기관 없음") < 27 {
		t.Fatalf("고객 없을 때 오류 부족 %s", miss.Body.String()[:min(600, miss.Body.Len())])
	}
	if !strings.Contains(miss.Body.String(), "등록된 기관 없음 27") {
		t.Fatal("사유별 건수 요약 없음")
	}
}

func countAssets(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func previewToken(body string) string {
	i := strings.Index(body, `name="preview_token"`)
	if i < 0 {
		return ""
	}
	s := body[i:]
	j := strings.Index(s, `value="`)
	if j < 0 {
		return ""
	}
	s = s[j+7:]
	k := strings.Index(s, `"`)
	if k < 0 {
		return ""
	}
	return s[:k]
}

func assertGanamCounts(t *testing.T, db *sql.DB) {
	t.Helper()
	var periodic, third, other, unknown, net, split, locOnly int
	rows, err := db.Query(`
		SELECT COALESCE(management_type,''), COALESCE(installer_type,''), COALESCE(product_type,''),
		       COALESCE(building_id,''), COALESCE(floor_id,''), COALESCE(room_id,''), COALESCE(install_location,'')
		  FROM assets`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var mt, it, pt, bid, fid, rid, loc string
		if err := rows.Scan(&mt, &it, &pt, &bid, &fid, &rid, &loc); err != nil {
			t.Fatal(err)
		}
		if mt == model.ManagementTypePeriodic {
			periodic++
		}
		if mt == model.ManagementTypeThirdParty {
			third++
		}
		if it == "other" {
			other++
		}
		if it == "unknown" {
			unknown++
		}
		if pt == "NETWORK" {
			net++
		}
		if bid != "" && fid != "" && rid != "" {
			split++
		} else if strings.Contains(loc, "창의융합동") {
			locOnly++
		}
	}
	if periodic != 19 || third != 8 || other != 19 || unknown != 8 || net != 27 {
		t.Fatalf("집계 periodic=%d third=%d other=%d unknown=%d net=%d", periodic, third, other, unknown, net)
	}
	if split != 26 || locOnly != 1 {
		t.Fatalf("위치 split=%d locOnly=%d", split, locOnly)
	}
}

func Test95_62_SlashSplitAndFileDupWarn(t *testing.T) {
	b, f, r, ok := model.SplitSlashLocation("본관 / 1층 / 교무실")
	if !ok || b != "본관" || f != "1층" || r != "교무실" {
		t.Fatalf("split %v %s %s %s", ok, b, f, r)
	}
	if model.JoinAssetInstallLocation("본관", "1층", "교무실") != "본관 1층 교무실" {
		t.Fatal("join")
	}
	if _, _, _, ok := model.SplitSlashLocation("창의융합동 1층 다목적실"); ok {
		t.Fatal("슬래시 없는 위치는 나누면 안 됨")
	}
}

func Test95_62_DoesNotCreateCustomerAndCLevelForbidden(t *testing.T) {
	raw := ganamXLSX(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a62nc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountAsset62(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	prev := postAssetXLSX(t, e, "/assets/import-preview", raw, ck, nil)
	if strings.Count(prev.Body.String(), "등록된 기관 없음") < 27 {
		t.Fatal("고객 없을 때 오류가 아니다")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("고객을 만들었다 %d", n)
	}
	cl := jwtCookieOrg(t, model.RoleCLevel, "O03")
	denied := postAssetXLSX(t, e, "/assets/import-preview", raw, cl, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("c_level %d", denied.Code)
	}
}

func Test95_62A_HTTPSharedCustomerAcrossOrg(t *testing.T) {
	raw := ganamXLSX(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a62a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O01','O01','도서관사업팀','도서관',1,1),
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C-GNAM','가남초등학교','가남초등학교',1,'O01','customer')`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountAsset62(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	prev := postAssetXLSX(t, e, "/assets/import-preview", raw, ck, nil)
	if prev.Code != 200 {
		t.Fatalf("미리보기 %d", prev.Code)
	}
	body := prev.Body.String()
	if !strings.Contains(body, "등록 27") {
		t.Fatalf("O01 고객을 SI가 써야 한다 %s", body[:min(600, len(body))])
	}
	if strings.Contains(body, "다른 조직 소속 27") {
		t.Fatal("고객 org_id 로 거절하면 안 된다")
	}
	if strings.Contains(body, "기관을 찾을 수 없습니다") {
		t.Fatal("예전 문구가 남아 있다")
	}
	if countAssets(t, db) != 0 {
		t.Fatal("미리보기가 저장했다")
	}
}

func Test95_62A_EmptySheetMessage(t *testing.T) {
	raw, err := repository.BuildAssetImportTemplate()
	if err != nil {
		t.Fatal(err)
	}
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "a62e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountAsset62(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	prev := postAssetXLSX(t, e, "/assets/import-preview", raw, ck, nil)
	if prev.Code != http.StatusSeeOther {
		t.Fatalf("빈 시트 %d", prev.Code)
	}
	loc := prev.Header().Get("Location")
	if !strings.Contains(loc, "자산입력") && !strings.Contains(prev.Body.String(), "2행") {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, loc, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "2행") {
			t.Fatalf("빈 시트 안내 %s body=%s", loc, rec.Body.String()[:min(400, rec.Body.Len())])
		}
	}
}

func Test95_62_ReuseAssetBusinessRulesNotCopy(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("internal", "handler", "asset_import.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte("applyAssetBusinessRules")) {
		t.Fatal("저장 규칙을 재사용하지 않는다")
	}
	if bytes.Contains(src, []byte("func applyThirdPartyRules")) {
		t.Fatal("타사장비 규칙을 복제했다")
	}
	_ = io.EOF
}
