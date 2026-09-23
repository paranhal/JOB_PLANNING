package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const quoteFormA1Path = "web/templates/quote/form_a1.xlsx"

const (
	quoteA1SheetName = "견적서"
)

func quoteSheetNo(q *model.SalesQuote) string {
	if q == nil {
		return ""
	}
	no := q.DisplayNo()
	if s := q.SalesDisplayNo(); s != "" {
		return s + " / " + no
	}
	return no
}

func quoteA1Sheet(f *excelize.File) string {
	if f == nil {
		return quoteA1SheetName
	}
	n := f.GetSheetName(0)
	if n == "" {
		return quoteA1SheetName
	}
	return n
}

func buildFormA1Template() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := quoteA1SheetName
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetColWidth(sheet, "A", "A", 12)
	_ = f.SetColWidth(sheet, "B", "E", 14)
	_ = f.SetColWidth(sheet, "F", "I", 12)
	_ = f.MergeCell(sheet, "A1", "L1")
	_ = f.SetCellValue(sheet, "A1", "견 적 서")
	_ = f.SetCellValue(sheet, "E4", "견적번호 :")
	_ = f.SetCellValue(sheet, "A5", "▷ 수     신 :")
	_ = f.SetCellValue(sheet, "A6", "▷ 참     조 :")
	_ = f.SetCellValue(sheet, "A7", "▷ 견 적 일 :")
	_ = f.SetCellValue(sheet, "E5", "사업자등록번호")
	_ = f.SetCellValue(sheet, "E6", "상호")
	_ = f.SetCellValue(sheet, "E7", "대표이사")
	_ = f.SetCellValue(sheet, "E8", "업태")
	_ = f.SetCellValue(sheet, "H8", "종목")
	_ = f.SetCellValue(sheet, "E9", "주소")
	_ = f.SetCellValue(sheet, "A12", "견적건명")
	_ = f.SetCellValue(sheet, "F12", "작업기한")
	_ = f.SetCellValue(sheet, "I12", "작업장소")
	_ = f.SetCellValue(sheet, "A13", "견적금액")
	_ = f.SetCellValue(sheet, "F13", "유효기간")
	_ = f.SetCellValue(sheet, "I13", "결제조건")
	_ = f.SetCellValue(sheet, "A15", "구분")
	_ = f.SetCellValue(sheet, "B15", "내용")
	_ = f.SetCellValue(sheet, "F15", "수량")
	_ = f.SetCellValue(sheet, "G15", "단위")
	_ = f.SetCellValue(sheet, "H15", "단가")
	_ = f.SetCellValue(sheet, "I15", "공급가")
	for r := 16; r <= 17; r++ {
		_ = f.MergeCell(sheet, fmt.Sprintf("B%d", r), fmt.Sprintf("E%d", r))
	}
	_ = f.SetCellValue(sheet, "H19", "소계")
	_ = f.SetCellValue(sheet, "H20", "부가세")
	_ = f.SetCellValue(sheet, "H21", "합계")
	_ = f.SetCellValue(sheet, "A31", model.QuoteDocNoA1)
	_ = f.SetDefinedName(&excelize.DefinedName{
		Name:     "_xlnm.Print_Area",
		RefersTo: sheet + "!$A$1:$L$33",
		Scope:    sheet,
	})
	return f, nil
}

func FillQuoteWorkbook(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	if q == nil {
		return nil, fmt.Errorf("견적이 필요합니다")
	}
	if QuoteOutputForm(q.FormType) == "D" {
		return FillQuoteFormD(q, company)
	}
	return FillQuoteFormP(q, company)
}

func EnsureQuoteTemplates() error {
	if err := EnsureQuoteFormPD(); err != nil {
		return err
	}
	if err := EnsureQuoteFormA1(""); err != nil {
		return err
	}
	if err := EnsureQuoteFormFile(quoteFormA2Path, buildFormA2Template); err != nil {
		return err
	}
	if err := EnsureQuoteFormFile(quoteFormB1Path, buildFormB1Template); err != nil {
		return err
	}
	return EnsureQuoteFormFile(quoteFormB2Path, buildFormB2Template)
}

func EnsureQuoteFormA1(path string) error {
	if path == "" {
		path = quoteFormA1Path
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := buildFormA1Template()
	if err != nil {
		return err
	}
	defer f.Close()
	return f.SaveAs(path)
}

func FillQuoteFormA1(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	return FillQuoteFormP(q, company)
}

func formatSalesWon(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = fmt.Sprintf("%d", -n)
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if n < 0 {
		return "-" + b.String()
	}
	return b.String()
}
