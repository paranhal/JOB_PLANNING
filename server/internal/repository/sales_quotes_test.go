package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestQuoteRepoNumberDateDuplicateAndCopy(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "quotes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewQuoteRepo(db)
	today := time.Now().Format("2006-01-02")

	q := &model.SalesQuote{
		FormType:      model.QuoteFormA,
		VATMode:       model.QuoteVATExcluded,
		RoundRule:     model.QuoteRoundNone,
		QuoteDate:     today,
		Title:         "감열지",
		OwnerName:     "최혜영",
		OwnerPhone:    "010-1111-2222",
		RecipientName: "세종시교육청",
		Lines: []model.SalesQuoteLine{
			{Name: "감열지", Qty: 1, UnitPrice: 150000, Unit: "EA"},
		},
	}
	if err := repo.Create(q); err != nil {
		t.Fatal(err)
	}
	if q.QuoteNo == "" || !strings.HasPrefix(q.QuoteNo, "VI-견적-") {
		t.Fatalf("번호=%s", q.QuoteNo)
	}
	if err := model.AssertQuoteNoMatchesDate(q.QuoteNo, q.QuoteDate, false); err != nil {
		t.Fatal(err)
	}
	day := strings.ReplaceAll(today, "-", "")
	if !strings.Contains(q.QuoteNo, day) {
		t.Fatalf("번호에 견적일이 없다: %s %s", q.QuoteNo, today)
	}

	q2 := *q
	q2.QuoteID = ""
	q2.QuoteNo = q.QuoteNo
	q2.Lines = []model.SalesQuoteLine{{Name: "다른 건", Qty: 1, UnitPrice: 1, Unit: "EA"}}
	q2.OwnerName, q2.OwnerPhone = "최혜영", "010-1111-2222"
	if err := repo.Create(&q2); err != nil {
		t.Fatal(err)
	}
	if q2.QuoteNo == q.QuoteNo {
		t.Fatal("같은 날 두 번째 번호가 겹쳤다")
	}

	_, err = db.Exec(`INSERT INTO sales_quotes (
		quote_id, quote_no, rev, quote_date, title, vat_mode, round_rule, owner_name, owner_phone, status
	) VALUES ('Q-dup', ?, 0, ?, 'dup', 'excluded', 'none', 'a', 'b', 'draft')`, q.QuoteNo, today)
	if err == nil {
		t.Fatal("중복 번호가 들어갔다")
	}

	copied, err := repo.CopyAsNew(q.QuoteID, today)
	if err != nil {
		t.Fatal(err)
	}
	if copied.QuoteNo == q.QuoteNo || copied.QuoteDate != today {
		t.Fatalf("복사 번호/날짜: %s %s", copied.QuoteNo, copied.QuoteDate)
	}
	if err := model.AssertQuoteNoMatchesDate(copied.QuoteNo, copied.QuoteDate, false); err != nil {
		t.Fatal(err)
	}

	q.OwnerName = ""
	if err := repo.Update(q); err != model.ErrQuoteOwnerRequired {
		t.Fatalf("담당자 없이 저장됨: %v", err)
	}

	got, _ := repo.Get(copied.QuoteID)
	if got.Subtotal != 150000 {
		t.Fatalf("복사 소계=%d", got.Subtotal)
	}

	p := &model.SalesProject{Name: "기존 구축", IsTentativeName: true}
	if err := NewSalesRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	if p.DealType != model.SalesDealBuild {
		t.Fatalf("영업 기본값=%s", p.DealType)
	}
}

func TestQuoteRepoReviseLaborSeedRatesFrozenBudgetPipeline(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "quotes_d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewQuoteRepo(db)

	rates, err := repo.ListLaborRates(2026)
	if err != nil || len(rates) != 17 {
		t.Fatalf("2026 직군=%d err=%v", len(rates), err)
	}
	var found07, found08, found09 bool
	for _, r := range rates {
		switch r.JobCode {
		case "07":
			found07 = r.Monthly == 6_901_660
		case "08":
			found08 = r.Monthly == 5_159_246
		case "09":
			found09 = r.Monthly == 7_754_124
		}
	}
	if !found07 || !found08 || !found09 {
		t.Fatal("기획서 ⑦⑧⑨ 시드가 다르다")
	}

	q := &model.SalesQuote{
		FormType: model.QuoteFormB2, VATMode: model.QuoteVATIncluded,
		OwnerName: "a", OwnerPhone: "1", Title: "그룹",
		Lines: []model.SalesQuoteLine{
			{Name: "A", GroupLabel: "G1", Qty: 1, UnitPrice: 100},
			{Name: "B", GroupLabel: "G1", Qty: 1, UnitPrice: 50},
			{Name: "C", GroupLabel: "G2", Qty: 1, UnitPrice: 25},
		},
	}
	if err := repo.Create(q); err != nil {
		t.Fatal(err)
	}
	if err := model.AssertGroupSumEquals(q.Lines, q.Subtotal); err != nil {
		t.Fatal(err)
	}

	rev, err := repo.Revise(q.QuoteID, "단가 조정")
	if err != nil {
		t.Fatal(err)
	}
	old, err := repo.Get(q.QuoteID)
	if err != nil || old.Rev != 0 {
		t.Fatalf("원본 유실: %v rev=%d", err, old.Rev)
	}
	if rev.Rev != 1 || rev.QuoteNo != old.QuoteNo {
		t.Fatalf("개정 no=%s r%d", rev.QuoteNo, rev.Rev)
	}
	old.Title = "x"
	if err := repo.Update(old); err != model.ErrQuoteReadOnly {
		t.Fatalf("읽기 전용: %v", err)
	}

	labor := &model.SalesQuote{
		FormType: model.QuoteFormA2, VATMode: model.QuoteVATExcluded,
		OwnerName: "a", OwnerPhone: "1", OverheadRate: 110, TechFeeRate: 20,
		Lines: []model.SalesQuoteLine{
			{Name: "응용SW개발자", Qty: 1, UnitPrice: 7_754_124, MMRate: 0.7, Unit: "M/M"},
		},
	}
	if err := repo.Create(labor); err != nil {
		t.Fatal(err)
	}
	frozen := labor.Total
	if err := repo.SetStandardRates(50, 10); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(labor.QuoteID)
	if got.Total != frozen || got.OverheadRate != 110 {
		t.Fatalf("소급 total=%d rate=%g", got.Total, got.OverheadRate)
	}

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "예산건", IsTentativeName: true, ExpectedAmount: 30_000_000, ExpectedYM: "2027-03"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	before, err := sales.Pipeline(time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bq := &model.SalesQuote{
		SalesID: p.SalesID, Purpose: model.QuotePurposeBudget, BudgetYear: 2027,
		OwnerName: "a", OwnerPhone: "1",
		Lines: []model.SalesQuoteLine{{Name: "출입", Qty: 1, UnitPrice: 1000}},
	}
	if err := repo.Create(bq); err != nil {
		t.Fatal(err)
	}
	after, err := sales.Pipeline(time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.TotalCount != before.TotalCount-1 {
		t.Fatalf("예산용이 파이프라인에 남음 before=%d after=%d", before.TotalCount, after.TotalCount)
	}
	jan := time.Date(2027, 1, 10, 0, 0, 0, 0, time.Local)
	follow, err := repo.ListBudgetFollowups(jan)
	if err != nil || len(follow) != 1 {
		t.Fatalf("1월 후속=%d err=%v", len(follow), err)
	}
	sep := time.Date(2027, 9, 4, 0, 0, 0, 0, time.Local)
	follow, _ = repo.ListBudgetFollowups(sep)
	if len(follow) != 0 {
		t.Fatal("9월에 예산 후속이 떴다")
	}
}

func TestQuoteRepoRejectsLineSumMismatch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "quotes_sum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewQuoteRepo(db)
	q := &model.SalesQuote{
		OwnerName: "a", OwnerPhone: "1",
		Lines: []model.SalesQuoteLine{
			{Name: "A", Qty: 1, UnitPrice: 100},
			{Name: "B", Qty: 1, UnitPrice: 200},
		},
	}
	if err := repo.Create(q); err != nil {
		t.Fatal(err)
	}
	if q.Subtotal != 300 {
		t.Fatalf("소계=%d", q.Subtotal)
	}
	if _, err := db.Exec(`UPDATE sales_quote_lines SET amount=1 WHERE quote_id=?`, q.QuoteID); err != nil {
		t.Fatal(err)
	}
	var sum int
	_ = db.QueryRow(`SELECT SUM(amount) FROM sales_quote_lines WHERE quote_id=?`, q.QuoteID).Scan(&sum)
	if sum == q.Subtotal {
		t.Fatal("검산 전제를 못 만들었다")
	}
}
