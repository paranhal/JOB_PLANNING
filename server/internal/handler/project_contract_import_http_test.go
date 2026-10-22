package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestContractImportHTTPPermAndTemplate(t *testing.T) {
	e, _, vs, oa, cl := newEquipCatHTTP(t)

	page := doGetWith(t, e, "/projects/import", vs)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "양식 다운로드") {
		t.Fatalf("page %d", page.Code)
	}
	tpl := doGetWith(t, e, "/projects/import/template.xlsx", vs)
	if tpl.Code != http.StatusOK {
		t.Fatalf("tpl %d", tpl.Code)
	}
	assertExcelizeRGB8(t, tpl.Body.Bytes())
	if doGetWith(t, e, "/projects/import/template.xlsx", cl).Code != http.StatusOK {
		t.Fatal("clevel download")
	}
	raw, err := repository.BuildContractWorkbook(repository.ContractGuideLines(), [][]string{
		{"사업명 *", "부서 *", "발주처 *", "최종수요처", "사업유형 *", "계약방식", "거래구분", "계약시작일 *", "계약종료일 *", "계약금액(VAT포함) *", "계약번호(선택)", "계약담당", "업무담당", "비고"},
		{"테스트", "도서관사업팀", "발주X", "", "공사", "", "매출", "2026-01-01", "2026-12-31", "1", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, cl).Code != http.StatusForbidden {
		t.Fatal("clevel preview")
	}
	oaPrev := postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, oa)
	if oaPrev.Code != http.StatusOK {
		t.Fatalf("oa %d", oaPrev.Code)
	}
	sales := jwtCookieUserClaims(t, &model.User{UserID: "USR-S95C2", Username: "s95c2", Role: model.RoleSales, IsActive: true, OrgID: model.OrgIDLibrary})
	if postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, sales).Code != http.StatusForbidden {
		t.Fatal("sales write")
	}
}

func TestContractImportHTTPRoutesOnProjectServer(t *testing.T) {
	e, _, _ := newProjectServer(t)
	rec := doGet(t, e, "/projects/import")
	if rec.Code != http.StatusOK {
		t.Fatalf("import page %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "엑셀 일괄 등록") {
		t.Fatal("form")
	}
	if !strings.Contains(rec.Body.String(), `formaction="/projects/import-apply"`) {
		t.Fatal("apply formaction")
	}
	if strings.Contains(rec.Body.String(), `formaction="/projects/import-errors.xlsx"`) {
		t.Fatal("errors on empty page")
	}
}

func TestContractImportHTTPBanner95_2B(t *testing.T) {
	e, _, vs, oa, _ := newEquipCatHTTP(t)
	page := doGetWith(t, e, "/projects/import", vs)
	if page.Code != http.StatusOK {
		t.Fatalf("page %d", page.Code)
	}
	lint := scanTemplateUILint(t)["project/import.html"]
	if !lint.zero() {
		t.Fatalf("import.html ui_lint %+v", lint)
	}
	raw, err := repository.BuildContractWorkbook(repository.ContractGuideLines(), [][]string{
		{"사업명 *", "부서 *", "발주처 *", "최종수요처", "사업유형 *", "계약방식", "거래구분", "계약시작일 *", "계약종료일 *", "계약금액(VAT포함) *", "계약번호(선택)", "계약담당", "업무담당", "비고"},
		{"테스트", "SI사업팀", "발주X", "", "공사", "", "매출", "2026-01-01", "2026-12-31", "1", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev := postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, oa)
	if prev.Code != http.StatusOK {
		t.Fatalf("oa preview %d", prev.Code)
	}
	body := prev.Body.String()
	if !strings.Contains(body, "권한 없는 부서") || !strings.Contains(body, "이 계정으로는") {
		t.Fatalf("banner %s", body)
	}
	apply := postEquipFile(t, e, "/projects/import-apply", "file", "t.xlsx", raw, oa)
	if apply.Code != http.StatusOK {
		t.Fatalf("oa apply %d", apply.Code)
	}
	if !strings.Contains(apply.Body.String(), "등록된 사업이 없습니다") {
		t.Fatal("empty apply")
	}
	vsPrev := postEquipFile(t, e, "/projects/import-preview", "file", "t.xlsx", raw, vs)
	if vsPrev.Code != http.StatusOK {
		t.Fatalf("vs %d", vsPrev.Code)
	}
	if !strings.Contains(vsPrev.Body.String(), "고객 없음") || !strings.Contains(vsPrev.Body.String(), "발주처 고객 마스터에 없습니다") {
		t.Fatalf("missing banner %s", vsPrev.Body.String())
	}
}

func TestContractImportSamplePathExists(t *testing.T) {
	p := filepath.Join("..", "..", "..", "docs", "양식", "사업등록_업로드_최종_47건.xlsx")
	if _, err := os.Stat(p); err != nil {
		t.Skip(err)
	}
}
