package handler

import (
	"bytes"
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
	for _, addr := range []string{"B13", "I19", "I20", "I21"} {
		fm, _ := f.GetCellFormula(sheet, addr)
		if fm != "" {
			t.Fatalf("%s 산식=%s", addr, fm)
		}
	}
	i19, _ := f.GetCellValue(sheet, "I19")
	i20, _ := f.GetCellValue(sheet, "I20")
	i21, _ := f.GetCellValue(sheet, "I21")
	if atoiCell(i19) != tot.Supply || atoiCell(i20) != tot.VAT || atoiCell(i21) != tot.Total {
		t.Fatalf("I19-21=%s/%s/%s", i19, i20, i21)
	}
	a31, _ := f.GetCellValue(sheet, "A31")
	if a31 != model.QuoteDocNoA1 {
		t.Fatalf("A31=%q", a31)
	}
	a7, _ := f.GetCellValue(sheet, "A7")
	if a7 != "▷ 견 적 일 : 2026년 08월 16일" {
		t.Fatalf("A7=%q", a7)
	}
}

func TestEnsureQuoteTemplatesAndOtherFormsNoFormula(t *testing.T) {
	if err := EnsureQuoteTemplates(); err != nil {
		t.Fatal(err)
	}
	q := &model.SalesQuote{
		QuoteNo: "VI-견적-20260816-001", QuoteDate: "2026-08-16",
		FormType: model.QuoteFormA2, VATMode: model.QuoteVATExcluded,
		OwnerName: "최혜영", OwnerPhone: "010", OverheadRate: 110, TechFeeRate: 20,
		Lines: []model.SalesQuoteLine{{Name: "응용SW개발자", Qty: 1, UnitPrice: 7_754_124, MMRate: 0.7, Unit: "M/M"}},
	}
	raw, err := FillQuoteWorkbook(q, model.QuoteCompany{Name: "㈜비젼아이티"})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "I19")

	q.FormType = model.QuoteFormB1
	q.VATMode = model.QuoteVATIncluded
	q.Lines = []model.SalesQuoteLine{{Name: "장서점검기", Qty: 1, UnitPrice: 5_700_000, GovPrice: 5_700_000}}
	raw, err = FillQuoteWorkbook(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "K25")

	q.FormType = model.QuoteFormB2
	q.Lines = []model.SalesQuoteLine{
		{Name: "A", GroupLabel: "서버", Qty: 1, UnitPrice: 1_000_000},
		{Name: "B", GroupLabel: "단말", Qty: 1, UnitPrice: 2_000_000},
	}
	raw, err = FillQuoteWorkbook(q, model.QuoteCompany{})
	if err != nil {
		t.Fatal(err)
	}
	assertXlsxNoFormula(t, raw, "K8")
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
