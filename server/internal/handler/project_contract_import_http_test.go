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
}

func TestContractImportSamplePathExists(t *testing.T) {
	p := filepath.Join("..", "..", "..", "docs", "양식", "사업등록_업로드_작성본.xlsx")
	if _, err := os.Stat(p); err != nil {
		t.Skip(err)
	}
}
