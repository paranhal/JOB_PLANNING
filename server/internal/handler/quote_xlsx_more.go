package handler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	quoteFormA2Path = "web/templates/quote/form_a2.xlsx"
	quoteFormB1Path = "web/templates/quote/form_b1.xlsx"
	quoteFormB2Path = "web/templates/quote/form_b2.xlsx"
)

func EnsureQuoteFormFile(path string, build func() (*excelize.File, error)) error {
	if path == "" {
		return fmt.Errorf("경로가 없습니다")
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := build()
	if err != nil {
		return err
	}
	defer f.Close()
	return f.SaveAs(path)
}

func openOrBuild(path string, build func() (*excelize.File, error)) (*excelize.File, error) {
	_ = EnsureQuoteFormFile(path, build)
	if _, err := os.Stat(path); err == nil {
		return excelize.OpenFile(path)
	}
	return build()
}

func writeWorkbook(f *excelize.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setPrintArea(f *excelize.File, sheet, ref string) {
	_ = f.DeleteDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: sheet})
	_ = f.SetDefinedName(&excelize.DefinedName{
		Name:     "_xlnm.Print_Area",
		RefersTo: sheet + "!" + ref,
		Scope:    sheet,
	})
}

func fillQuoteHeaderA(f *excelize.File, sheet string, q *model.SalesQuote, company model.QuoteCompany) {
	recipient := strings.TrimSpace(q.RecipientName)
	if recipient != "" {
		recipient += " 귀중"
	}
	_ = f.SetCellValue(sheet, "A4", recipient)
	_ = f.SetCellValue(sheet, "E4", "견적번호 : "+q.DisplayNo())
	attn := strings.TrimSpace(q.AttnName)
	if attn != "" {
		_ = f.SetCellValue(sheet, "A5", "▷ 수     신 : "+attn+" 귀하")
	} else {
		_ = f.SetCellValue(sheet, "A5", "▷ 수     신 :")
	}
	_ = f.SetCellValue(sheet, "A6", "▷ 참     조 : "+strings.TrimSpace(q.AttnTitle))
	_ = f.SetCellValue(sheet, "A7", "▷ 견 적 일 : "+model.FormatQuoteDateKorean(q.QuoteDate))
	_ = f.SetCellValue(sheet, "F5", company.BizNo)
	_ = f.SetCellValue(sheet, "F6", company.Name)
	_ = f.SetCellValue(sheet, "F7", company.CEO)
	_ = f.SetCellValue(sheet, "F8", company.BizType)
	_ = f.SetCellValue(sheet, "I8", company.Item)
	_ = f.SetCellValue(sheet, "F9", q.OwnerName)
	_ = f.SetCellValue(sheet, "G9", company.Address)
	_ = f.SetCellValue(sheet, "I9", q.OwnerPhone)
	_ = f.SetCellValue(sheet, "B12", q.Title)
	tot := q.Totals()
	_ = f.SetCellInt(sheet, "B13", tot.Total)
	_ = f.SetCellValue(sheet, "C13", model.VATModeLabel(q.VATMode))
	_ = f.SetCellValue(sheet, "F12", q.DueText)
	_ = f.SetCellValue(sheet, "I12", q.PlaceText)
	_ = f.SetCellValue(sheet, "F13", q.ValidUntilText)
	_ = f.SetCellValue(sheet, "I13", q.PaymentText)
}

func buildFormA2Template() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := quoteA1SheetName
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetColWidth(sheet, "A", "I", 12)
	_ = f.MergeCell(sheet, "A1", "I1")
	_ = f.SetCellValue(sheet, "A1", "견 적 서")
	_ = f.SetCellValue(sheet, "E4", "견적번호 :")
	_ = f.SetCellValue(sheet, "A5", "▷ 수     신 :")
	_ = f.SetCellValue(sheet, "A6", "▷ 참     조 :")
	_ = f.SetCellValue(sheet, "A7", "▷ 견 적 일 :")
	_ = f.SetCellValue(sheet, "A12", "견적건명")
	_ = f.SetCellValue(sheet, "A13", "견적금액")
	_ = f.SetCellValue(sheet, "A15", "구분")
	_ = f.SetCellValue(sheet, "B15", "내용")
	_ = f.SetCellValue(sheet, "F15", "M/M")
	_ = f.SetCellValue(sheet, "G15", "투입율")
	_ = f.SetCellValue(sheet, "H15", "단가")
	_ = f.SetCellValue(sheet, "I15", "금액")
	_ = f.SetCellValue(sheet, "H19", "직접인건비")
	_ = f.SetCellValue(sheet, "H20", "제경비")
	_ = f.SetCellValue(sheet, "H21", "기술료")
	_ = f.SetCellValue(sheet, "H22", "소계")
	_ = f.SetCellValue(sheet, "H23", "부가세")
	_ = f.SetCellValue(sheet, "H24", "합계")
	_ = f.SetCellValue(sheet, "A28", "유지보수부분")
	_ = f.SetCellValue(sheet, "A40", model.QuoteDocNoA1)
	setPrintArea(f, sheet, "$A$1:$I$42")
	return f, nil
}

func FillQuoteFormA2(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	f, err := openOrBuild(quoteFormA2Path, buildFormA2Template)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := quoteA1Sheet(f)
	fillQuoteHeaderA(f, sheet, q, company)
	lines := q.Lines
	if len(lines) == 0 {
		lines = []model.SalesQuoteLine{{}}
	}
	extra := len(lines) - 2
	if extra > 0 {
		_ = f.InsertRows(sheet, 18, extra)
	}
	shift := extra
	if shift < 0 {
		shift = 0
	}
	for i, ln := range lines {
		row := 16 + i
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ln.GroupLabel)
		content := ln.Name
		if strings.TrimSpace(ln.Spec) != "" {
			content += "\n" + ln.Spec
		}
		if b := ln.LaborYearBadge(q.QuoteYear()); b != "" {
			content += " (" + b + ")"
		}
		if ln.PriceOverridden {
			content += " [단가 수정]"
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), content)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), ln.Qty)
		mm := ln.MMRate
		if mm <= 0 {
			mm = 1
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), mm)
		_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", row), ln.UnitPrice)
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), model.LineAmount(ln))
	}
	tot := q.Totals()
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 19+shift), tot.Direct)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 20+shift), tot.Overhead)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 21+shift), tot.TechFee)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 22+shift), tot.Supply)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 23+shift), tot.VAT)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 24+shift), tot.Total)
	if q.MaintBlock {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", 28+shift), "유지보수부분")
	} else {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", 28+shift), "")
	}
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", 40+shift), model.QuoteDocNoA1)
	setPrintArea(f, sheet, fmt.Sprintf("$A$1:$I$%d", 42+shift))
	return writeWorkbook(f)
}

func buildFormB1Template() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := quoteA1SheetName
	f.SetSheetName("Sheet1", sheet)
	_ = f.MergeCell(sheet, "A1", "K1")
	_ = f.SetCellValue(sheet, "A1", "견 적 서 (조달)")
	_ = f.SetCellValue(sheet, "A3", "수신")
	_ = f.SetCellValue(sheet, "G3", "견적번호")
	_ = f.SetCellValue(sheet, "A4", "견적일")
	_ = f.SetCellValue(sheet, "A6", "구분")
	_ = f.SetCellValue(sheet, "B6", "품명")
	_ = f.SetCellValue(sheet, "C6", "규격")
	_ = f.SetCellValue(sheet, "D6", "수량")
	_ = f.SetCellValue(sheet, "E6", "단위")
	_ = f.SetCellValue(sheet, "F6", "조달단가")
	_ = f.SetCellValue(sheet, "G6", "물품식별번호")
	_ = f.SetCellValue(sheet, "H6", "단가")
	_ = f.SetCellValue(sheet, "I6", "공급가")
	_ = f.SetCellValue(sheet, "J6", "부가세")
	_ = f.SetCellValue(sheet, "K6", "공급금액")
	_ = f.SetCellValue(sheet, "H24", "소계")
	_ = f.SetCellValue(sheet, "H25", "합계")
	setPrintArea(f, sheet, "$A$1:$K$28")
	return f, nil
}

func FillQuoteFormB1(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	f, err := openOrBuild(quoteFormB1Path, buildFormB1Template)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := quoteA1Sheet(f)
	_ = f.SetCellValue(sheet, "B3", q.RecipientName)
	_ = f.SetCellValue(sheet, "H3", q.DisplayNo())
	_ = f.SetCellValue(sheet, "B4", model.FormatQuoteDateKorean(q.QuoteDate))
	_ = f.SetCellValue(sheet, "B5", q.Title)
	_ = f.SetCellValue(sheet, "J3", q.OwnerName)
	_ = f.SetCellValue(sheet, "J4", q.OwnerPhone)
	lines := q.Lines
	if len(lines) == 0 {
		lines = []model.SalesQuoteLine{{}}
	}
	extra := len(lines) - 2
	if extra > 0 {
		_ = f.InsertRows(sheet, 9, extra)
	}
	shift := extra
	if shift < 0 {
		shift = 0
	}
	tot := q.Totals()
	for i, ln := range lines {
		row := 7 + i
		amt := model.LineAmount(ln)
		supply, vat := lineVATSplit(amt, q.VATMode)
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ln.GroupLabel)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ln.Name)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), ln.Spec)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), ln.Qty)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), ln.Unit)
		_ = f.SetCellInt(sheet, fmt.Sprintf("F%d", row), ln.GovPrice)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), ln.Note)
		_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", row), ln.UnitPrice)
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), supply)
		_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", row), vat)
		_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", row), amt)
	}
	_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", 24+shift), tot.Supply)
	_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", 25+shift), tot.Total)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", 26+shift), model.RoundRuleLabel(q.RoundRule))
	setPrintArea(f, sheet, fmt.Sprintf("$A$1:$K$%d", 28+shift))
	_ = company
	return writeWorkbook(f)
}

func buildFormB2Template() (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := quoteA1SheetName
	f.SetSheetName("Sheet1", sheet)
	_ = f.MergeCell(sheet, "A1", "L1")
	_ = f.SetCellValue(sheet, "A1", "견 적 서 (조달 · 그룹)")
	_ = f.SetCellValue(sheet, "A3", "수신")
	_ = f.SetCellValue(sheet, "G3", "견적번호")
	_ = f.SetCellValue(sheet, "A6", "묶음")
	_ = f.SetCellValue(sheet, "B6", "품명")
	_ = f.SetCellValue(sheet, "C6", "규격")
	_ = f.SetCellValue(sheet, "D6", "수량")
	_ = f.SetCellValue(sheet, "E6", "단위")
	_ = f.SetCellValue(sheet, "F6", "조달단가")
	_ = f.SetCellValue(sheet, "G6", "물품식별번호")
	_ = f.SetCellValue(sheet, "H6", "단가")
	_ = f.SetCellValue(sheet, "I6", "공급가")
	_ = f.SetCellValue(sheet, "J6", "부가세")
	_ = f.SetCellValue(sheet, "K6", "공급금액")
	_ = f.SetCellValue(sheet, "L6", "묶음소계")
	_ = f.SetCellValue(sheet, "K28", "합계")
	setPrintArea(f, sheet, "$A$1:$L$32")
	return f, nil
}

func FillQuoteFormB2(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	f, err := openOrBuild(quoteFormB2Path, buildFormB2Template)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := quoteA1Sheet(f)
	_ = f.SetCellValue(sheet, "B3", q.RecipientName)
	_ = f.SetCellValue(sheet, "H3", q.DisplayNo())
	_ = f.SetCellValue(sheet, "B4", model.FormatQuoteDateKorean(q.QuoteDate))
	_ = f.SetCellValue(sheet, "B5", q.Title)
	groups := model.GroupSubtotals(q.Lines)
	row := 7
	prev := ""
	for _, ln := range q.Lines {
		label := strings.TrimSpace(ln.GroupLabel)
		if prev != "" && label != prev {
			writeGroupSubRow(f, sheet, row, prev, groupSum(groups, prev), q.VATMode)
			row++
		}
		amt := model.LineAmount(ln)
		supply, vat := lineVATSplit(amt, q.VATMode)
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ln.GroupLabel)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), ln.Name)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), ln.Spec)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), ln.Qty)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), ln.Unit)
		_ = f.SetCellInt(sheet, fmt.Sprintf("F%d", row), ln.GovPrice)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), ln.Note)
		_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", row), ln.UnitPrice)
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), supply)
		_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", row), vat)
		_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", row), amt)
		row++
		prev = label
	}
	if prev != "" || len(q.Lines) > 0 {
		writeGroupSubRow(f, sheet, row, prev, groupSum(groups, prev), q.VATMode)
		row++
	}
	tot := q.Totals()
	_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", row), "소계")
	_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", row), tot.Supply)
	row++
	_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", row), "합계")
	_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", row), tot.Total)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row+2), model.RoundRuleLabel(q.RoundRule))
	setPrintArea(f, sheet, fmt.Sprintf("$A$1:$L$%d", row+4))
	_ = company
	return writeWorkbook(f)
}

func writeGroupSubRow(f *excelize.File, sheet string, row int, label string, sum int, vatMode string) {
	supply, vat := lineVATSplit(sum, vatMode)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), label+" 소계")
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), supply)
	_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", row), vat)
	_ = f.SetCellInt(sheet, fmt.Sprintf("K%d", row), sum)
	_ = f.SetCellInt(sheet, fmt.Sprintf("L%d", row), sum)
}

func groupSum(groups []model.QuoteGroupSubtotal, label string) int {
	for _, g := range groups {
		if strings.TrimSpace(g.Label) == strings.TrimSpace(label) {
			return g.Sum
		}
	}
	return 0
}

func lineVATSplit(amount int, vatMode string) (supply, vat int) {
	if model.NormalizeVATMode(vatMode) == model.QuoteVATIncluded {
		supply = int(float64(amount) / 1.1)
		return supply, amount - supply
	}
	return amount, amount / 10
}
