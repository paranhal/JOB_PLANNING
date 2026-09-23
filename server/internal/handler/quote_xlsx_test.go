package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func TestFillQuoteFormA1_ValuesNotFormulas(t *testing.T) {
	q := &model.SalesQuote{
		QuoteNo:        "VI-견적-20260816-001",
		QuoteDate:      "2026-08-16",
		RecipientName:  "세종시교육청평생교육원",
		AttnName:       "윤희",
		Title:          "감열지",
		VATMode:        model.QuoteVATIncluded,
		RoundRule:      model.QuoteRoundNone,
		OwnerName:      "최혜영",
		OwnerPhone:     "010-0000-0000",
		DueText:        "납품 후 2주",
		PlaceText:      "현장",
		ValidUntilText: "견적 후 2주 한",
		PaymentText:    "검수 후",
		Lines: []model.SalesQuoteLine{
			{GroupLabel: "소모품", Name: "감열지", Spec: "59x80", Qty: 1, Unit: "EA", UnitPrice: 5_700_000},
		},
	}
	raw, err := FillQuoteFormA1(q, model.QuoteCompany{
		BizNo: "307-81-15849", Name: "㈜비젼아이티", CEO: "이 지 환",
	})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	tot := q.Totals()
	b13, _ := f.GetCellValue(sheet, "B13")
	if atoiCell(b13) != tot.Total {
		t.Fatalf("B13=%s want=%d", b13, tot.Total)
	}
	sub, vat, totRow := QuotePSumRows(len(quotePVisualRows(q)))
	for _, addr := range []string{"B13", fmt.Sprintf("I%d", sub), fmt.Sprintf("I%d", vat), fmt.Sprintf("H%d", totRow)} {
		fm, _ := f.GetCellFormula(sheet, addr)
		if fm != "" {
			t.Fatalf("%s 산식=%s", addr, fm)
		}
	}
	iSub, _ := f.GetCellValue(sheet, fmt.Sprintf("I%d", sub))
	iVAT, _ := f.GetCellValue(sheet, fmt.Sprintf("I%d", vat))
	hTot, _ := f.GetCellValue(sheet, fmt.Sprintf("H%d", totRow))
	if atoiCell(iSub) != tot.Supply || atoiCell(iVAT) != tot.VAT || atoiCell(hTot) != tot.Total {
		t.Fatalf("합계칸=%s/%s/%s", iSub, iVAT, hTot)
	}
	a7, _ := f.GetCellValue(sheet, "A7")
	if !strings.Contains(a7, "2026년 08월 16일") {
		t.Fatalf("A7=%q", a7)
	}
}

func TestFillQuoteFormP_FiveLinesTwoSpecs(t *testing.T) {
	if err := EnsureQuoteFormPD(); err != nil {
		t.Fatal(err)
	}
	srcN, err := zipMediaCountFile(quoteFormPPath)
	if err != nil {
		t.Fatal(err)
	}
	q := &model.SalesQuote{
		QuoteNo: "VI-견적-20260923-001", QuoteDate: "2026-09-23",
		RecipientName: "고객사", Title: "테스트", VATMode: model.QuoteVATExcluded,
		OwnerName: "담당", OwnerPhone: "010",
		Lines: []model.SalesQuoteLine{
			{Name: "품목1", Qty: 1, Unit: "EA", UnitPrice: 1000},
			{Name: "품목2", Qty: 1, Unit: "EA", UnitPrice: 2000, Spec: "상세A\n상세B"},
			{Name: "품목3", Qty: 1, Unit: "EA", UnitPrice: 3000},
			{Name: "품목4", Qty: 1, Unit: "EA", UnitPrice: 4000},
			{Name: "품목5", Qty: 1, Unit: "EA", UnitPrice: 5000},
		},
	}
	raw, err := FillQuoteFormP(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	if zipMediaCountBytes(raw) != srcN {
		t.Fatalf("media 원본=%d 출력=%d", srcN, zipMediaCountBytes(raw))
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	vis := quotePVisualRows(q)
	if len(vis) != 7 {
		t.Fatalf("시각행=%d", len(vis))
	}
	sub, vat, totRow := QuotePSumRows(len(vis))
	tot := q.Totals()
	got, _ := f.GetCellValue(sheet, fmt.Sprintf("I%d", sub))
	if atoiCell(got) != tot.Supply {
		t.Fatalf("소계행 %d =%s want %d", sub, got, tot.Supply)
	}
	got, _ = f.GetCellValue(sheet, fmt.Sprintf("I%d", vat))
	if atoiCell(got) != tot.VAT {
		t.Fatalf("부가세행")
	}
	got, _ = f.GetCellValue(sheet, fmt.Sprintf("H%d", totRow))
	if atoiCell(got) != tot.Total {
		t.Fatalf("합계행")
	}
	for _, addr := range []string{"F6", "I6", "F7", "I7", "H5"} {
		v, _ := f.GetCellValue(sheet, addr)
		if strings.TrimSpace(v) == "" {
			t.Fatalf("회사 칸 %s 가 비었다", addr)
		}
	}
	tf, err := excelize.OpenFile(quoteFormPPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()
	ts := tf.GetSheetName(0)
	for _, addr := range []string{"F6", "I6", "F7", "I7", "H5"} {
		want, _ := tf.GetCellValue(ts, addr)
		got, _ := f.GetCellValue(sheet, addr)
		if strings.TrimSpace(got) != strings.TrimSpace(want) {
			t.Fatalf("%s 원본=%q 출력=%q", addr, want, got)
		}
	}
	pg, err := f.GetPageLayout(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if pg.FitToWidth == nil || *pg.FitToWidth != 1 {
		t.Fatalf("가로 1쪽 맞춤이 아니다 %+v", pg.FitToWidth)
	}
}

func TestFillQuoteFormD_LaborHalfRate(t *testing.T) {
	q := &model.SalesQuote{
		FormType: model.QuoteFormA2, QuoteDate: "2026-09-23",
		VATMode: model.QuoteVATExcluded, OverheadRate: 0, TechFeeRate: 0,
		Lines: []model.SalesQuoteLine{
			{GroupLabel: "인건비", Name: "응용SW개발자", Qty: 1, UnitPrice: 10_000_000, MMRate: 0.5, Unit: "M/M"},
		},
	}
	raw, err := FillQuoteFormD(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	srcN, _ := zipMediaCountFile(quoteFormDPath)
	if zipMediaCountBytes(raw) != srcN {
		t.Fatalf("D형 media 원본=%d 출력=%d", srcN, zipMediaCountBytes(raw))
	}
	hasEMF := false
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, zf := range zr.File {
		if strings.HasSuffix(strings.ToLower(zf.Name), ".emf") {
			hasEMF = true
		}
	}
	if !hasEMF {
		t.Fatal("D형 로고 .emf 가 빠졌다")
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	i17, _ := f.GetCellValue(sheet, "I17")
	j17, _ := f.GetCellValue(sheet, "J17")
	if atoiCell(i17) != 5_000_000 || atoiCell(j17) != 500_000 {
		t.Fatalf("I17/J17=%s/%s", i17, j17)
	}
	fm, _ := f.GetCellFormula(sheet, "I17")
	if fm != "" {
		t.Fatalf("I17 산식=%s", fm)
	}
}

func TestEnsureQuoteTemplatesAndOtherFormsNoFormula(t *testing.T) {
	if err := EnsureQuoteTemplates(); err != nil {
		t.Fatal(err)
	}
	q := &model.SalesQuote{
		QuoteNo: "VI-견적-20260816-001", QuoteDate: "2026-08-16",
		FormType: model.QuoteFormA2, VATMode: model.QuoteVATExcluded,
		OwnerName: "최혜영", OwnerPhone: "010", OverheadRate: 0, TechFeeRate: 0,
		Lines: []model.SalesQuoteLine{{Name: "응용SW개발자", Qty: 1, UnitPrice: 7_754_124, MMRate: 0.7, Unit: "M/M"}},
	}
	raw, err := FillQuoteWorkbook(q, model.QuoteCompany{Name: "㈜비젼아이티"})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "I17")

	q.FormType = model.QuoteFormB1
	q.VATMode = model.QuoteVATIncluded
	q.Lines = []model.SalesQuoteLine{{Name: "장서점검기", Qty: 1, UnitPrice: 5_700_000, GovPrice: 5_700_000}}
	raw, err = FillQuoteWorkbook(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "B13")

	q.FormType = model.QuoteFormB2
	q.Lines = []model.SalesQuoteLine{
		{Name: "A", GroupLabel: "서버", Qty: 1, UnitPrice: 1_000_000},
		{Name: "B", GroupLabel: "단말", Qty: 1, UnitPrice: 2_000_000},
	}
	raw, err = FillQuoteWorkbook(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "I18")
}

func assertXlsxNoFormula(t *testing.T, raw []byte, addr string) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	fm, _ := f.GetCellFormula(sheet, addr)
	if strings.TrimSpace(fm) != "" {
		t.Fatalf("%s 산식=%s", addr, fm)
	}
}
