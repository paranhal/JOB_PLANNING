package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	quoteFormPPath = "web/templates/quote/form_p.xlsx"
	quoteFormDPath = "web/templates/quote/form_d.xlsx"
)

type quoteCells struct {
	Recipient, QuoteNo, Attn, Ref, Date     string
	Owner, Phone                            string
	Title, Due, Place                       string
	Amount, VATLabel, Valid, Payment        string
	LineStart, DefaultLines                 int
	SubtotalRow, VATRow, TotalRow           int
	MaintStart, MaintEnd                    int
	Remarks, Footer                         string
	PrintFirst, PrintLastCol                string
	ColStart, ColEnd                        int
}

var quoteFormP = quoteCells{
	Recipient: "A4", QuoteNo: "E4", Attn: "A5", Ref: "A6", Date: "A7",
	Owner: "F9", Phone: "I9", Title: "B12", Due: "F12", Place: "I12",
	Amount: "B13", VATLabel: "C13", Valid: "F13", Payment: "I13",
	LineStart: 16, DefaultLines: 1,
	SubtotalRow: 17, VATRow: 18, TotalRow: 19,
	MaintStart: 21, MaintEnd: 24,
	Remarks: "A27", Footer: "A30",
	PrintFirst: "A", PrintLastCol: "I",
	ColStart: 1, ColEnd: 9,
}

var quoteFormD = quoteCells{
	Recipient: "B4", QuoteNo: "F4", Attn: "C5", Ref: "C6", Date: "C7",
	Title: "D12", Place: "J12", Amount: "D13", Payment: "J13",
	LineStart: 17, DefaultLines: 3,
	SubtotalRow: 24, VATRow: 24, TotalRow: 25,
	MaintStart: 26, MaintEnd: 29,
	Remarks: "B33", Footer: "B36",
	PrintFirst: "B", PrintLastCol: "J",
	ColStart: 2, ColEnd: 10,
}

func QuoteOutputForm(form string) string {
	if model.NormalizeQuoteForm(form) == model.QuoteFormA2 {
		return "D"
	}
	return "P"
}

func FillQuoteFormP(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	if err := EnsureQuoteFormPD(); err != nil {
		return nil, err
	}
	f, err := excelize.OpenFile(quoteFormPPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_ = company
	sheet := quoteA1Sheet(f)
	vis := quotePVisualRows(q)
	extra := len(vis) - quoteFormP.DefaultLines
	if extra < 0 {
		extra = 0
	}
	if extra > 0 {
		if err := insertStyledRows(f, sheet, quoteFormP.LineStart+quoteFormP.DefaultLines, extra, quoteFormP.LineStart, quoteFormP.ColStart, quoteFormP.ColEnd); err != nil {
			return nil, err
		}
	}
	fillQuoteHeaderPD(f, sheet, quoteFormP, q)
	row := quoteFormP.LineStart
	for _, vr := range vis {
		if vr.spec != "" {
			_ = f.MergeCell(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("E%d", row))
			_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), vr.spec)
			row++
			continue
		}
		ln := vr.line
		_ = f.MergeCell(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("E%d", row))
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), ln.GroupLabel)
		content := ln.Name
		if strings.TrimSpace(ln.Spec) != "" && !strings.Contains(ln.Spec, "\n") {
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
		row++
	}
	shift := extra
	tot := q.Totals()
	subRow := quoteFormP.SubtotalRow + shift
	vatRow := quoteFormP.VATRow + shift
	totRow := quoteFormP.TotalRow + shift
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", subRow), tot.Supply)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", vatRow), tot.VAT)
	_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", totRow), tot.Total)
	applyMaintVisible(f, sheet, quoteFormP.MaintStart+shift, quoteFormP.MaintEnd+shift, q.MaintBlock)
	_ = f.SetCellValue(sheet, shiftedCell(quoteFormP.Remarks, shift), q.Remarks)
	last := quoteFormP.footerRow() + shift
	setPrintArea(f, sheet, fmt.Sprintf("$A$1:$I$%d", last))
	setFitToWidth1(f, sheet)
	return writeWorkbook(f)
}

func FillQuoteFormD(q *model.SalesQuote, company model.QuoteCompany) ([]byte, error) {
	if err := EnsureQuoteFormPD(); err != nil {
		return nil, err
	}
	f, err := excelize.OpenFile(quoteFormDPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_ = company
	sheet := quoteA1Sheet(f)
	labor, exp := splitLaborExpense(q)
	if tot := q.Totals(); tot.Overhead != 0 || tot.TechFee != 0 {
		if tot.Overhead != 0 {
			exp = append(exp, model.SalesQuoteLine{Name: "제경비", Qty: 1, UnitPrice: tot.Overhead, MMRate: 1})
		}
		if tot.TechFee != 0 {
			exp = append(exp, model.SalesQuoteLine{Name: "기술료", Qty: 1, UnitPrice: tot.TechFee, MMRate: 1})
		}
	}
	laborExtra := len(labor) - 3
	if laborExtra < 0 {
		laborExtra = 0
	}
	expExtra := len(exp) - 3
	if expExtra < 0 {
		expExtra = 0
	}
	if laborExtra > 0 {
		if err := insertStyledRows(f, sheet, 20, laborExtra, 17, quoteFormD.ColStart, quoteFormD.ColEnd); err != nil {
			return nil, err
		}
	}
	if expExtra > 0 {
		if err := insertStyledRows(f, sheet, 23+laborExtra, expExtra, 21+laborExtra, quoteFormD.ColStart, quoteFormD.ColEnd); err != nil {
			return nil, err
		}
	}
	fillQuoteHeaderPD(f, sheet, quoteFormD, q)
	writeDLines(f, sheet, 17, labor, true)
	clearUnusedDLines(f, sheet, 17, len(labor), 3)
	writeDLines(f, sheet, 21+laborExtra, exp, false)
	clearUnusedDLines(f, sheet, 21+laborExtra, len(exp), 3)
	shift := laborExtra + expExtra
	tot := q.Totals()
	laborSum, laborVAT := sumDLines(labor)
	expSum, expVAT := sumDLines(exp)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 16), laborSum)
	_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", 16), laborVAT)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 20+laborExtra), expSum)
	_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", 20+laborExtra), expVAT)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 24+shift), tot.Supply)
	_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", 24+shift), tot.VAT)
	_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", 25+shift), tot.Total)
	applyMaintVisible(f, sheet, quoteFormD.MaintStart+shift, quoteFormD.MaintEnd+shift, q.MaintBlock)
	_ = f.SetCellValue(sheet, shiftedCell(quoteFormD.Remarks, shift), q.Remarks)
	last := quoteFormD.footerRow() + shift
	setPrintArea(f, sheet, fmt.Sprintf("$B$1:$J$%d", last))
	setFitToWidth1(f, sheet)
	return writeWorkbook(f)
}

type pVisRow struct {
	line model.SalesQuoteLine
	spec string
}

func quotePVisualRows(q *model.SalesQuote) []pVisRow {
	lines := q.Lines
	if len(lines) == 0 {
		lines = []model.SalesQuoteLine{{}}
	}
	var out []pVisRow
	for _, ln := range lines {
		out = append(out, pVisRow{line: ln})
		if strings.Contains(ln.Spec, "\n") {
			for _, part := range strings.Split(strings.ReplaceAll(ln.Spec, "\r\n", "\n"), "\n") {
				if s := strings.TrimSpace(part); s != "" {
					out = append(out, pVisRow{spec: s})
				}
			}
		}
	}
	return out
}

func QuotePSumRows(visualN int) (sub, vat, total int) {
	extra := visualN - quoteFormP.DefaultLines
	if extra < 0 {
		extra = 0
	}
	return quoteFormP.SubtotalRow + extra, quoteFormP.VATRow + extra, quoteFormP.TotalRow + extra
}

func fillQuoteHeaderPD(f *excelize.File, sheet string, cells quoteCells, q *model.SalesQuote) {
	recipient := strings.TrimSpace(q.RecipientName)
	if recipient != "" {
		recipient += "  귀중"
	}
	_ = f.SetCellValue(sheet, cells.Recipient, recipient)
	_ = f.SetCellValue(sheet, cells.QuoteNo, "견적번호 : "+quoteSheetNo(q))
	attn := strings.TrimSpace(q.AttnName)
	if attn == "" {
		attn = "담당자님"
	}
	if strings.HasPrefix(cells.Attn, "C") {
		_ = f.SetCellValue(sheet, cells.Attn, " 수     신  : "+attn+" 귀하")
		_ = f.SetCellValue(sheet, cells.Ref, " 참     조 : "+strings.TrimSpace(q.AttnTitle))
		_ = f.SetCellValue(sheet, cells.Date, " 견 적 일 :  "+model.FormatQuoteDateKorean(q.QuoteDate))
	} else {
		_ = f.SetCellValue(sheet, cells.Attn, "▷ 수     신 : "+attn+" 귀하")
		_ = f.SetCellValue(sheet, cells.Ref, "▷ 참     조 : "+strings.TrimSpace(q.AttnTitle))
		_ = f.SetCellValue(sheet, cells.Date, "▷ 견 적 일 :  "+model.FormatQuoteDateKorean(q.QuoteDate))
	}
	if cells.Owner != "" {
		_ = f.SetCellValue(sheet, cells.Owner, q.OwnerName)
		_ = f.SetCellValue(sheet, cells.Phone, q.OwnerPhone)
	}
	_ = f.SetCellValue(sheet, cells.Title, q.Title)
	if cells.Due != "" {
		_ = f.SetCellValue(sheet, cells.Due, q.DueText)
	}
	_ = f.SetCellValue(sheet, cells.Place, q.PlaceText)
	tot := q.Totals()
	_ = f.SetCellInt(sheet, cells.Amount, tot.Total)
	if cells.VATLabel != "" {
		if model.NormalizeVATMode(q.VATMode) == model.QuoteVATIncluded {
			_ = f.SetCellValue(sheet, cells.VATLabel, "(VAT포함)")
		} else {
			_ = f.SetCellValue(sheet, cells.VATLabel, "(VAT별도)")
		}
	}
	if cells.Valid != "" {
		_ = f.SetCellValue(sheet, cells.Valid, q.ValidUntilText)
	}
	_ = f.SetCellValue(sheet, cells.Payment, q.PaymentText)
}

func writeDLines(f *excelize.File, sheet string, start int, lines []model.SalesQuoteLine, labor bool) {
	for i, ln := range lines {
		row := start + i
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), i+1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), ln.Name)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), ln.Spec)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), ln.Unit)
		if ln.Qty != 0 {
			_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), ln.Qty)
		}
		g := quoteLineG(ln, labor)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), g)
		_ = f.SetCellInt(sheet, fmt.Sprintf("H%d", row), ln.UnitPrice)
		amt := model.LineAmount(ln)
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), amt)
		_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", row), amt/10)
	}
}

func quoteLineG(ln model.SalesQuoteLine, labor bool) float64 {
	if labor {
		if ln.MMRate > 0 {
			return ln.MMRate
		}
		return 1
	}
	apply := 1.0
	if ln.MMRate > 0 {
		apply = ln.MMRate
	}
	disc := ln.DiscountRate
	if disc < 0 {
		disc = 0
	}
	if disc > 1 {
		disc = 1
	}
	return apply * (1 - disc)
}

func clearUnusedDLines(f *excelize.File, sheet string, start, filled, capacity int) {
	for i := filled; i < capacity; i++ {
		row := start + i
		for _, col := range []string{"C", "D", "E", "F", "G", "H"} {
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), "")
		}
		_ = f.SetCellInt(sheet, fmt.Sprintf("I%d", row), 0)
		_ = f.SetCellInt(sheet, fmt.Sprintf("J%d", row), 0)
	}
}

func splitLaborExpense(q *model.SalesQuote) (labor, exp []model.SalesQuoteLine) {
	for _, ln := range q.Lines {
		if strings.TrimSpace(ln.GroupLabel) == "경비" {
			exp = append(exp, ln)
		} else {
			labor = append(labor, ln)
		}
	}
	return
}

func sumDLines(lines []model.SalesQuoteLine) (sum, vat int) {
	for _, ln := range lines {
		amt := model.LineAmount(ln)
		sum += amt
		vat += amt / 10
	}
	return
}

func applyMaintVisible(f *excelize.File, sheet string, from, to int, show bool) {
	for r := from; r <= to; r++ {
		_ = f.SetRowVisible(sheet, r, show)
	}
}

func insertStyledRows(f *excelize.File, sheet string, at, n, styleFrom, colStart, colEnd int) error {
	if n <= 0 {
		return nil
	}
	if err := f.InsertRows(sheet, at, n); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		copyRowStyle(f, sheet, styleFrom, at+i, colStart, colEnd)
		_ = f.MergeCell(sheet, fmt.Sprintf("B%d", at+i), fmt.Sprintf("E%d", at+i))
	}
	return nil
}

func copyRowStyle(f *excelize.File, sheet string, src, dst, colStart, colEnd int) {
	h, err := f.GetRowHeight(sheet, src)
	if err == nil && h > 0 {
		_ = f.SetRowHeight(sheet, dst, h)
	}
	for c := colStart; c <= colEnd; c++ {
		from, _ := excelize.CoordinatesToCellName(c, src)
		to, _ := excelize.CoordinatesToCellName(c, dst)
		st, err := f.GetCellStyle(sheet, from)
		if err == nil {
			_ = f.SetCellStyle(sheet, to, to, st)
		}
	}
}

func setFitToWidth1(f *excelize.File, sheet string) {
	one, zero := 1, 0
	tru := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &tru})
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{FitToWidth: &one, FitToHeight: &zero})
}

func shiftedCell(addr string, shift int) string {
	if shift == 0 || addr == "" {
		return addr
	}
	col, row, err := excelize.CellNameToCoordinates(addr)
	if err != nil {
		return addr
	}
	out, _ := excelize.CoordinatesToCellName(col, row+shift)
	return out
}

func (c quoteCells) footerRow() int {
	_, row, err := excelize.CellNameToCoordinates(c.Footer)
	if err != nil {
		return 30
	}
	return row
}

func zipMediaCountFile(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return zipMediaCountBytes(raw), nil
}

func zipMediaCountBytes(raw []byte) int {
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return 0
	}
	n := 0
	for _, f := range r.File {
		name := f.Name
		if strings.HasPrefix(name, "xl/media/") && !strings.HasSuffix(name, "/") {
			n++
		}
	}
	return n
}

func quoteDocsDir() string {
	cands := []string{
		filepath.Join("docs", "견적양식"),
		filepath.Join("..", "docs", "견적양식"),
		filepath.Join("..", "..", "docs", "견적양식"),
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append([]string{filepath.Join(wd, "docs", "견적양식"), filepath.Join(wd, "..", "docs", "견적양식")}, cands...)
	}
	for _, d := range cands {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return filepath.Join("docs", "견적양식")
}
