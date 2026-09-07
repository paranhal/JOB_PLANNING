package model

import "testing"

func TestQuoteVATIncluded5700000(t *testing.T) {
	lines := []SalesQuoteLine{{Name: "장서점검기", Qty: 1, UnitPrice: 5_700_000}}
	tot := ComputeQuoteTotals(lines, QuoteVATIncluded, QuoteRoundNone)
	if tot.Supply != 5_181_818 || tot.VAT != 518_182 || tot.Total != 5_700_000 {
		t.Fatalf("공급가=%d 부가세=%d 합계=%d", tot.Supply, tot.VAT, tot.Total)
	}
	if tot.Supply+tot.VAT != tot.Total {
		t.Fatal("공급가+부가세가 합계와 다르다")
	}
	if tot.LineSum != 5_700_000 {
		t.Fatalf("소계(라인합)=%d", tot.LineSum)
	}
}

func TestQuoteElevenLinesAllInSum(t *testing.T) {
	var lines []SalesQuoteLine
	want := 0
	for i := 1; i <= 11; i++ {
		price := i * 100_000
		lines = append(lines, SalesQuoteLine{Name: "품목", Qty: 1, UnitPrice: price})
		want += price
	}
	tot := ComputeQuoteTotals(lines, QuoteVATExcluded, QuoteRoundNone)
	if tot.LineSum != want {
		t.Fatalf("11줄 소계=%d want=%d", tot.LineSum, want)
	}
	q := &SalesQuote{VATMode: QuoteVATExcluded, RoundRule: QuoteRoundNone}
	if err := ApplyQuoteTotals(q, lines); err != nil {
		t.Fatal(err)
	}
	if len(q.Lines) != 11 || q.Subtotal != want {
		t.Fatalf("저장 소계=%d lines=%d", q.Subtotal, len(q.Lines))
	}
}

func TestQuoteLineDeleteAndReorder(t *testing.T) {
	lines := []SalesQuoteLine{
		{Name: "A", Qty: 1, UnitPrice: 29_100_000},
		{Name: "B", Qty: 1, UnitPrice: 8_850_000},
		{Name: "C", Qty: 1, UnitPrice: 32_010_000},
	}
	tot := ComputeQuoteTotals(lines, QuoteVATExcluded, QuoteRoundNone)
	if tot.LineSum != 69_960_000 {
		t.Fatalf("세 줄 합=%d", tot.LineSum)
	}
	lines = []SalesQuoteLine{lines[2], lines[0]}
	tot = ComputeQuoteTotals(lines, QuoteVATExcluded, QuoteRoundNone)
	if tot.LineSum != 61_110_000 {
		t.Fatalf("지우고 순서 바꾼 합=%d", tot.LineSum)
	}
}

func TestQuoteRoundHundredMatchesLabel(t *testing.T) {
	if RoundRuleLabel(QuoteRoundHundred) != "100원 단위 절사" {
		t.Fatal(RoundRuleLabel(QuoteRoundHundred))
	}
	if ApplyRoundRule(25_222_587, QuoteRoundHundred) != 25_222_500 {
		t.Fatalf("100원 절사=%d", ApplyRoundRule(25_222_587, QuoteRoundHundred))
	}
	if ApplyRoundRule(25_222_587, QuoteRoundThousand) != 25_222_000 {
		t.Fatalf("천원 절사=%d", ApplyRoundRule(25_222_587, QuoteRoundThousand))
	}
	if RoundRuleUnit(QuoteRoundHundred) != 100 || RoundRuleUnit(QuoteRoundThousand) != 1000 {
		t.Fatal("절사 자릿수가 라벨과 다르다")
	}
}

func TestQuoteNoTiedToDate(t *testing.T) {
	no := FormatQuoteNo("2026-08-16", 1)
	if no != "VI-견적-20260816-001" {
		t.Fatal(no)
	}
	if err := AssertQuoteNoMatchesDate(no, "2026-08-16", false); err != nil {
		t.Fatal(err)
	}
	if err := AssertQuoteNoMatchesDate(no, "2026-06-29", false); err == nil {
		t.Fatal("날짜가 다른데 통과했다")
	}
	if err := AssertQuoteNoMatchesDate("VIT-260330-01", "2026-03-30", true); err != nil {
		t.Fatal("옛 번호 반입이 막혔다")
	}
}

func TestQuoteOwnerRequired(t *testing.T) {
	q := &SalesQuote{OwnerName: "", OwnerPhone: "010"}
	if err := q.ValidateOwner(); err != ErrQuoteOwnerRequired {
		t.Fatal(err)
	}
	q.OwnerName = "최혜영"
	q.OwnerPhone = ""
	if err := q.ValidateOwner(); err != ErrQuoteOwnerRequired {
		t.Fatal(err)
	}
	q.OwnerPhone = "010-0000-0000"
	if err := q.ValidateOwner(); err != nil {
		t.Fatal(err)
	}
}

func TestQuoteDefaultVATByForm(t *testing.T) {
	if DefaultVATMode(QuoteFormA) != QuoteVATExcluded || DefaultVATMode(QuoteFormB) != QuoteVATIncluded {
		t.Fatal("양식별 VAT 기본값이 다르다")
	}
}

func TestQuoteLaborLineAmount(t *testing.T) {
	ln := SalesQuoteLine{Name: "개발", Qty: 1, UnitPrice: 7_754_124, MMRate: 0.7}
	if LineAmount(ln) != 5_427_886 {
		t.Fatalf("용역 금액=%d", LineAmount(ln))
	}
}

func TestQuoteKanbanFiveStatus(t *testing.T) {
	v := FillQuoteKanban([]SalesQuote{{QuoteID: "Q-1", Status: QuoteStatusDraft, Title: "a"}})
	if len(v.Columns) != 5 {
		t.Fatalf("열=%d", len(v.Columns))
	}
}

func TestParseQuoteDateRejectsDay74(t *testing.T) {
	if _, err := ParseQuoteDateISO("2026-07-74"); err == nil {
		t.Fatal("74일이 통과했다")
	}
}
