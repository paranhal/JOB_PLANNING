package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestContractImportHoldForbiddenUsedExpired(t *testing.T) {
	h := newContractImportHold()
	tok := h.Put("U1", "O01", "a.xlsx", []byte("abc"))
	if _, _, err := h.Get("U2", "O01", tok, false); err != errImportHoldForbidden {
		t.Fatalf("user %v", err)
	}
	if _, _, err := h.Get("U1", "O02", tok, false); err != errImportHoldForbidden {
		t.Fatalf("org %v", err)
	}
	if _, name, err := h.Get("U1", "O01", tok, true); err != nil || name != "a.xlsx" {
		t.Fatalf("first %v", err)
	}
	if _, _, err := h.Get("U1", "O01", tok, true); err != errImportHoldUsed {
		t.Fatalf("used %v", err)
	}
	tok2 := h.Put("U1", "O01", "b.xlsx", []byte("x"))
	h.mu.Lock()
	b := h.m[tok2]
	b.Until = time.Now().Add(-time.Minute)
	h.m[tok2] = b
	h.mu.Unlock()
	if _, _, err := h.Get("U1", "O01", tok2, false); err != errImportHoldGone {
		t.Fatalf("exp %v", err)
	}
}

func TestContractImportHTTPPipelineTokenAndPopup(t *testing.T) {
	e, db, vs, oa, _ := newEquipCatHTTP(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('C-L','발주L','발주L',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/projects/import", vs)
	if page.Code != http.StatusOK {
		t.Fatalf("page %d", page.Code)
	}
	if strings.Contains(page.Body.String(), `formaction="/projects/import-errors.xlsx"`) {
		t.Fatal("errors on start")
	}
	lint := scanTemplateUILint(t)["project/import.html"]
	if !lint.zero() {
		t.Fatalf("ui_lint %+v", lint)
	}
	raw, err := repository.BuildContractWorkbook(repository.ContractGuideLines(), [][]string{
		{"사업명 *", "부서 *", "발주처 *", "최종수요처", "사업유형 *", "계약방식", "거래구분", "계약시작일 *", "계약종료일 *", "계약금액(VAT포함) *", "계약번호(선택)", "계약담당", "업무담당", "비고"},
		{"제로사업", "도서관사업팀", "발주L", "", "공사", "", "매출", "2026-01-01", "2026-12-31", "0", "", "", "", ""},
		{"유료사업", "도서관사업팀", "발주L", "", "공사", "", "매출", "2026-01-01", "2026-12-31", "10", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev := postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, oa)
	if prev.Code != http.StatusOK {
		t.Fatalf("prev %d", prev.Code)
	}
	body := prev.Body.String()
	if !strings.Contains(body, "영업 파이프라인 후보") || !strings.Contains(body, "name=\"pipeline_line\"") {
		t.Fatalf("candidate %s", body)
	}
	if !strings.Contains(body, `role="dialog"`) {
		t.Fatal("dialog")
	}
	tok := importPreviewToken(body)
	if tok == "" {
		t.Fatal("token")
	}
	oa2 := postImportFields(t, e, "/projects/import-apply", oa, map[string]string{"preview_token": tok})
	if oa2.Code != http.StatusOK {
		t.Fatalf("need confirm %d", oa2.Code)
	}
	if !strings.Contains(oa2.Body.String(), "영업 파이프라인에 등록") {
		t.Fatal("confirm copy")
	}
	stolen := postImportFields(t, e, "/projects/import-apply", vs, map[string]string{
		"preview_token": tok, "pipeline": "yes", "pipeline_stage": "discover", "pipeline_lines_present": "1", "pipeline_line": "2",
	})
	if stolen.Code != http.StatusForbidden {
		t.Fatalf("stolen %d", stolen.Code)
	}
	ok := postImportFields(t, e, "/projects/import-apply", oa, map[string]string{
		"preview_token": tok, "pipeline": "yes", "pipeline_stage": "discover", "pipeline_lines_present": "1", "pipeline_line": "2",
	})
	if ok.Code != http.StatusOK {
		t.Fatalf("apply %d %s", ok.Code, ok.Body.String())
	}
	ob := ok.Body.String()
	if !strings.Contains(ob, "사업 업로드 결과") || !strings.Contains(ob, "importResultDialog") {
		t.Fatalf("result popup %s", ob)
	}
	var wp, sp int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE name='제로사업'`).Scan(&wp)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE name='제로사업'`).Scan(&sp)
	if wp != 0 || sp != 1 {
		t.Fatalf("wp=%d sp=%d", wp, sp)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE name='유료사업'`).Scan(&wp)
	if wp != 1 {
		t.Fatalf("paid %d", wp)
	}
	again := postImportFields(t, e, "/projects/import-apply", oa, map[string]string{
		"preview_token": tok, "pipeline": "yes", "pipeline_lines_present": "1",
	})
	if again.Code != http.StatusForbidden {
		t.Fatalf("reuse %d", again.Code)
	}
}

func importPreviewToken(body string) string {
	m := regexp.MustCompile(`name="preview_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func postImportFields(t *testing.T, e *echo.Echo, path string, ck *http.Cookie, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}
