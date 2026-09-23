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

const quoteFormA1Path = "web/templates/quote/form_a1.xlsx"

const (
	quoteA1LineStart    = 16
	quoteA1DefaultLines = 2
	quoteA1SheetName    = "견적서"
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
	switch model.NormalizeQuoteForm(q.FormType) {
	case model.QuoteFormA2:
		return FillQuoteFormA2(q, company)
	case model.QuoteFormB1:
		return FillQuoteFormB1(q, company)
	case model.QuoteFormB2:
		return FillQuoteFormB2(q, company)
	default:
		return FillQuoteFormA1(q, company)
	}
}

func EnsureQuoteTemplates() error {
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
	if q == nil {
		return nil, fmt.Errorf("견적이 필요합니다")
	}
	_ = EnsureQuoteFormA1(quoteFormA1Path)
	var f *excelize.File
	var err error
	if _, statErr := os.Stat(quoteFormA1Path); statErr == nil {
		f, err = excelize.OpenFile(quoteFormA1Path)
	} else {
		f, err = buildFormA1Template()
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := quoteA1Sheet(f)
	lines := q.Lines
	if len(lines) == 0 {
		lines = []model.SalesQuoteLine{{}}
	}
	extra := len(lines) - quoteA1DefaultLines
	if extra > 0 {
		if err := f.InsertRows(sheet, 18, extra); err != nil {
			return nil, err
		}
		for i := 0; i < extra; i++ {
			row := quoteA1LineStart + quoteA1DefaultLines + i
			_ = f.MergeCell(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("E%d", row))
		}
	}
	shift := extra
	if shift < 0 {
		shift = 0
	}
	recipient := strings.TrimSpace(q.RecipientName)
	if recipient != "" {
		recipient += " 귀중"
	}
	_ = f.SetCellValue(sheet, "A4", recipient)
	_ = f.SetCellValue(sheet, "E4", "견적번호 : "+quoteSheetNo(q))
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
	for i, ln := range lines {
		row := quoteA1LineStart + i
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ln.GroupLabel)
		content := ln.Name
		if strings.TrimSpace(ln.Spec) != "" {
			if content != "" {
				content += "\n"
			}
			content += ln.Spec
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), content)
		if ln.Qty != 0 {
			_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), ln.Qty)
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), ln.Unit)
		_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", row), ln.UnitPrice)
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), model.LineAmount(ln))
	}
	subRow := 19 + shift
	vatRow := 20 + shift
	totRow := 21 + shift
	roundRow := 22 + shift
	footRow := 31 + shift
	_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", subRow), "소계")
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", subRow), tot.Supply)
	_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", vatRow), "부가세")
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", vatRow), tot.VAT)
	_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", totRow), "합계")
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", totRow), tot.Total)
	roundLabel := model.RoundRuleLabel(q.RoundRule)
	if tot.RoundRule != model.QuoteRoundNone && tot.TotalBeforeRound != tot.Total {
		roundLabel += " (절사 전 " + formatSalesWon(tot.TotalBeforeRound) + ")"
	}
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", roundRow), roundLabel)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", footRow), model.QuoteDocNoA1)
	last := footRow + 2
	if last < 33+shift {
		last = 33 + shift
	}
	_ = f.DeleteDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: sheet})
	_ = f.SetDefinedName(&excelize.DefinedName{
		Name:     "_xlnm.Print_Area",
		RefersTo: fmt.Sprintf("%s!$A$1:$L$%d", sheet, last),
		Scope:    sheet,
	})
	if p := strings.TrimSpace(company.StampPath); p != "" {
		if _, err := os.Stat(p); err == nil {
			_ = f.AddPicture(sheet, "J5", p, &excelize.GraphicOptions{LockAspectRatio: true, AutoFit: true})
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
