package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	QuoteVATIncluded = "included"
	QuoteVATExcluded = "excluded"

	QuoteRoundNone     = "none"
	QuoteRoundHundred  = "hundred"
	QuoteRoundThousand = "thousand"

	QuoteFormA  = "A"
	QuoteFormA2 = "A2"
	QuoteFormB  = "B"
	QuoteFormB1 = "B1"
	QuoteFormB2 = "B2"

	QuoteStatusDraft   = "draft"
	QuoteStatusSent    = "sent"
	QuoteStatusWon     = "won"
	QuoteStatusLost    = "lost"
	QuoteStatusExpired = "expired"

	QuotePurposeDeal      = "deal"
	QuotePurposeBudget    = "budget"
	QuotePurposeReference = "reference"

	QuoteRecipientCustomer = "customer"
	QuoteRecipientPartner  = "partner"

	QuoteKindMaint        = "maint_service"
	QuoteKindDev          = "dev_service"
	QuoteKindConstruction = "construction"
	QuoteKindSolution     = "solution"

	QuoteNoPrefix = "VI-견적-"
	QuoteDocNoA1  = "QEP-710-01"
)

var (
	ErrQuoteOwnerRequired = fmt.Errorf("담당자와 연락처가 필요합니다")
	ErrQuoteNoDuplicate   = fmt.Errorf("같은 견적번호가 이미 있습니다")
	ErrQuoteReadOnly      = fmt.Errorf("이전 개정은 수정할 수 없습니다")
	ErrQuoteRevReason     = fmt.Errorf("개정 사유가 필요합니다")
	ErrQuoteSalesRequired = fmt.Errorf("사업을 먼저 고르세요")
	ErrQuoteLineSum       = fmt.Errorf("라인 합계와 소계가 다릅니다")
)

type SalesQuote struct {
	QuoteID        string
	SalesID        string
	SalesNo        string
	QuoteKind      string
	QuoteNo        string
	Rev            int
	FormType       string
	RecipientKind  string
	CustomerID     string
	RecipientName  string
	AttnName       string
	AttnTitle      string
	QuoteDate      string
	Title          string
	ValidUntilText string
	DueText        string
	PlaceText      string
	PaymentText    string
	VATMode        string
	RoundRule      string
	Subtotal       int
	VAT            int
	Total          int
	OwnerUserID    string
	OwnerName      string
	OwnerPhone     string
	Remarks        string
	Purpose        string
	BudgetYear     int
	OverheadRate   float64
	TechFeeRate    float64
	Status         string
	IsLegacy       bool
	RevReason      string
	IsReverseCalc  bool
	TargetTotal    int
	MaintBlock     bool
	CreatedAt      string
	UpdatedAt      string
	Lines          []SalesQuoteLine
	Prev           []SalesQuote
	MissingSales   bool
}

type SalesQuoteLine struct {
	LineID          string
	QuoteID         string
	Seq             int
	ItemID          string
	GroupLabel      string
	Name            string
	Spec            string
	Qty             float64
	Unit            string
	UnitPrice       int
	Amount          int
	GovPrice        int
	MMRate          float64
	DiscountRate    float64
	Note            string
	RateID          string
	LaborYear       int
	PriceOverridden bool
}

type QuoteTotals struct {
	LineSum          int
	Supply           int
	VAT              int
	Total            int
	TotalBeforeRound int
	RoundRule        string
	Direct           int
	Overhead         int
	TechFee          int
}

type QuoteGroupSubtotal struct {
	Label string
	Sum   int
}

type QuoteCompany struct {
	Name      string
	BizNo     string
	CEO       string
	BizType   string
	Item      string
	Address   string
	Phone     string
	Bank      string
	Account   string
	StampPath string
}

type LaborRate struct {
	RateID        string
	Year          int
	JobCode       string
	JobName       string
	Monthly       int
	Daily         int
	Hourly        int
	EffectiveFrom string
	EffectiveTo   string
	SourceNote    string
}

type LaborAddon struct {
	Direct   int
	Overhead int
	TechFee  int
	Subtotal int
}

func LineAmount(ln SalesQuoteLine) int {
	qty := ln.Qty
	if qty <= 0 {
		return 0
	}
	rate := ln.MMRate
	if rate <= 0 {
		rate = 1
	}
	disc := ln.DiscountRate
	if disc < 0 {
		disc = 0
	}
	if disc > 1 {
		disc = 1
	}
	v := qty * float64(ln.UnitPrice) * rate * (1 - disc)
	if v <= 0 {
		return 0
	}
	return int(v)
}

func PrepareQuoteLines(lines []SalesQuoteLine) []SalesQuoteLine {
	out := make([]SalesQuoteLine, 0, len(lines))
	for _, ln := range lines {
		if strings.TrimSpace(ln.Name) == "" && ln.Qty == 0 && ln.UnitPrice == 0 {
			continue
		}
		ln.Amount = LineAmount(ln)
		out = append(out, ln)
	}
	return out
}

func (ln *SalesQuoteLine) MarkPriceOverride(monthly int) {
	if ln == nil || monthly <= 0 {
		return
	}
	if ln.UnitPrice != monthly {
		ln.PriceOverridden = true
	}
}

func (ln SalesQuoteLine) LaborYearBadge(quoteYear int) string {
	if ln.LaborYear <= 0 || quoteYear <= 0 || ln.LaborYear == quoteYear {
		return ""
	}
	return fmt.Sprintf("%d년 단가", ln.LaborYear)
}

func LaborAddons(direct int, overheadPct, techPct float64) LaborAddon {
	oh := int(float64(direct) * overheadPct / 100)
	tech := int(float64(direct+oh) * techPct / 100)
	return LaborAddon{Direct: direct, Overhead: oh, TechFee: tech, Subtotal: direct + oh + tech}
}

func ComputeQuoteTotals(lines []SalesQuoteLine, vatMode, roundRule string) QuoteTotals {
	return ComputeQuoteTotalsFromBase(sumLineAmount(lines), vatMode, roundRule)
}

func ComputeQuoteTotalsFromBase(sum int, vatMode, roundRule string) QuoteTotals {
	vatMode = NormalizeVATMode(vatMode)
	roundRule = NormalizeRoundRule(roundRule)
	tot := QuoteTotals{LineSum: sum, Direct: sum, RoundRule: roundRule}
	if vatMode == QuoteVATIncluded {
		tot.TotalBeforeRound = sum
		tot.Total = ApplyRoundRule(sum, roundRule)
		if tot.Total > 0 {
			tot.Supply = int(float64(tot.Total) / 1.1)
		}
		tot.VAT = tot.Total - tot.Supply
		return tot
	}
	tot.Supply = sum
	tot.VAT = sum / 10
	tot.TotalBeforeRound = tot.Supply + tot.VAT
	tot.Total = ApplyRoundRule(tot.TotalBeforeRound, roundRule)
	return tot
}

func (q *SalesQuote) Totals() QuoteTotals {
	if q == nil {
		return QuoteTotals{}
	}
	lines := PrepareQuoteLines(q.Lines)
	direct := sumLineAmount(lines)
	if QuoteFormIsLabor(q.FormType) {
		add := LaborAddons(direct, q.OverheadRate, q.TechFeeRate)
		tot := ComputeQuoteTotalsFromBase(add.Subtotal, q.VATMode, q.RoundRule)
		tot.LineSum = add.Subtotal
		tot.Direct = add.Direct
		tot.Overhead = add.Overhead
		tot.TechFee = add.TechFee
		return tot
	}
	return ComputeQuoteTotals(lines, q.VATMode, q.RoundRule)
}

func ApplyQuoteTotals(q *SalesQuote, lines []SalesQuoteLine) error {
	if q == nil {
		return fmt.Errorf("견적이 필요합니다")
	}
	q.Lines = PrepareQuoteLines(lines)
	tot := q.Totals()
	q.Subtotal = tot.LineSum
	q.VAT = tot.VAT
	q.Total = tot.Total
	return nil
}

func sumLineAmount(lines []SalesQuoteLine) int {
	sum := 0
	for _, ln := range lines {
		sum += LineAmount(ln)
	}
	return sum
}

func ApplyRoundRule(n int, rule string) int {
	unit := RoundRuleUnit(rule)
	if unit <= 1 || n == 0 {
		return n
	}
	neg := n < 0
	if neg {
		n = -n
	}
	n = n / unit * unit
	if neg {
		return -n
	}
	return n
}

func RoundRuleUnit(rule string) int {
	switch NormalizeRoundRule(rule) {
	case QuoteRoundHundred:
		return 100
	case QuoteRoundThousand:
		return 1000
	default:
		return 1
	}
}

func RoundRuleLabel(rule string) string {
	switch NormalizeRoundRule(rule) {
	case QuoteRoundHundred:
		return "100원 단위 절사"
	case QuoteRoundThousand:
		return "1,000원 단위 절사"
	default:
		return "절사 없음"
	}
}

func NormalizeRoundRule(s string) string {
	switch strings.TrimSpace(s) {
	case QuoteRoundHundred, "100":
		return QuoteRoundHundred
	case QuoteRoundThousand, "1000", "1,000":
		return QuoteRoundThousand
	default:
		return QuoteRoundNone
	}
}

func NormalizeVATMode(s string) string {
	if strings.TrimSpace(s) == QuoteVATIncluded {
		return QuoteVATIncluded
	}
	return QuoteVATExcluded
}

func DefaultVATMode(form string) string {
	switch NormalizeQuoteForm(form) {
	case QuoteFormB, QuoteFormB1, QuoteFormB2:
		return QuoteVATIncluded
	default:
		return QuoteVATExcluded
	}
}

func VATModeLabel(s string) string {
	if NormalizeVATMode(s) == QuoteVATIncluded {
		return "VAT 포함"
	}
	return "VAT 미포함"
}

func NormalizeQuoteForm(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case QuoteFormA2:
		return QuoteFormA2
	case QuoteFormB2:
		return QuoteFormB2
	case QuoteFormB1, "B-1":
		return QuoteFormB1
	case QuoteFormB, "B-ICT":
		return QuoteFormB
	default:
		return QuoteFormA
	}
}

func QuoteFormIsLabor(form string) bool {
	return NormalizeQuoteForm(form) == QuoteFormA2
}

func QuoteFormIsGrouped(form string) bool {
	return NormalizeQuoteForm(form) == QuoteFormB2
}

func QuoteFormLabel(form string) string {
	switch NormalizeQuoteForm(form) {
	case QuoteFormA2:
		return "용역(A-2)"
	case QuoteFormB1:
		return "ICT솔루션(B-1)"
	case QuoteFormB2:
		return "ICT솔루션(B-2)"
	case QuoteFormB:
		return "ICT솔루션(B)"
	default:
		return "QEP-710-01(A)"
	}
}

func NormalizeQuoteStatus(s string) string {
	switch strings.TrimSpace(s) {
	case QuoteStatusSent, QuoteStatusWon, QuoteStatusLost, QuoteStatusExpired:
		return strings.TrimSpace(s)
	default:
		return QuoteStatusDraft
	}
}

func QuoteStatusLabel(s string) string {
	switch NormalizeQuoteStatus(s) {
	case QuoteStatusSent:
		return "제출"
	case QuoteStatusWon:
		return "수주"
	case QuoteStatusLost:
		return "실주"
	case QuoteStatusExpired:
		return "만료"
	default:
		return "작성중"
	}
}

func QuoteStatusDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: QuoteStatusDraft, Title: "작성중", Border: "border-slate-200"},
		{Key: QuoteStatusSent, Title: "제출", Border: "border-sky-200"},
		{Key: QuoteStatusWon, Title: "수주", Border: "border-emerald-200"},
		{Key: QuoteStatusLost, Title: "실주", Border: "border-rose-200"},
		{Key: QuoteStatusExpired, Title: "만료", Border: "border-amber-200"},
	}
}

func FillQuoteKanban(items []SalesQuote) KanbanView {
	cols := emptyKanbanColumns(QuoteStatusDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		q := items[i]
		id := strings.TrimSpace(q.QuoteID)
		if id == "" || seen[id] {
			continue
		}
		b := NormalizeQuoteStatus(q.Status)
		j, ok := idx[b]
		if !ok {
			continue
		}
		seen[id] = true
		ref := q.DisplayNo()
		if s := q.SalesDisplayNo(); s != "" {
			ref = s + " / " + ref
		}
		cols[j].Items = append(cols[j].Items, KanbanCard{
			ID: id, RefID: id, RefNumber: ref, Title: q.Title,
			Href: "/quotes/" + id, EditHref: "/quotes/" + id + "/edit",
			OrgName: q.RecipientName, Extra: formatSalesAmount(q.Total) + "원",
			Bucket: b, AmountDesc: q.Total,
		})
	}
	v := finishKanbanView(cols)
	for i := range v.Columns {
		v.Columns[i].CountUnit = "건"
	}
	return v
}

func NormalizeQuoteKind(s string) string {
	switch strings.TrimSpace(s) {
	case QuoteKindDev, QuoteKindConstruction, QuoteKindSolution, QuoteKindMaint:
		return strings.TrimSpace(s)
	default:
		return ""
	}
}

func QuoteKindForm(kind string) string {
	if NormalizeQuoteKind(kind) == QuoteKindDev {
		return QuoteFormA2
	}
	if kind == "" {
		return ""
	}
	return QuoteFormA
}

func QuoteKindFromBizType(biz string) string {
	switch strings.TrimSpace(biz) {
	case "maintenance":
		return QuoteKindMaint
	case "develop":
		return QuoteKindDev
	case "construction":
		return QuoteKindConstruction
	case "build", "goods":
		return QuoteKindSolution
	default:
		return ""
	}
}

func QuoteKindFromContractTarget(target string) string {
	switch strings.TrimSpace(target) {
	case "construction":
		return QuoteKindConstruction
	case "goods_make", "goods_buy":
		return QuoteKindSolution
	case "service":
		return QuoteKindMaint
	default:
		return QuoteKindMaint
	}
}

func GroupLatestQuotes(items []SalesQuote) []SalesQuote {
	best := map[string]int{}
	for i := range items {
		no := strings.TrimSpace(items[i].QuoteNo)
		if no == "" {
			no = items[i].QuoteID
		}
		j, ok := best[no]
		if !ok || items[i].Rev > items[j].Rev {
			best[no] = i
		}
	}
	var out []SalesQuote
	seen := map[string]bool{}
	for i := range items {
		no := strings.TrimSpace(items[i].QuoteNo)
		if no == "" {
			no = items[i].QuoteID
		}
		if best[no] != i || seen[no] {
			continue
		}
		seen[no] = true
		q := items[i]
		q.MissingSales = strings.TrimSpace(q.SalesID) == ""
		for j := range items {
			jn := strings.TrimSpace(items[j].QuoteNo)
			if jn == "" {
				jn = items[j].QuoteID
			}
			if jn == no && items[j].Rev < q.Rev {
				q.Prev = append(q.Prev, items[j])
			}
		}
		out = append(out, q)
	}
	return out
}

func NormalizeQuotePurpose(s string) string {
	switch strings.TrimSpace(s) {
	case QuotePurposeBudget, QuotePurposeReference:
		return strings.TrimSpace(s)
	default:
		return QuotePurposeDeal
	}
}

func QuotePurposeLabel(s string) string {
	switch NormalizeQuotePurpose(s) {
	case QuotePurposeBudget:
		return "예산용"
	case QuotePurposeReference:
		return "참고"
	default:
		return "실거래"
	}
}

func QuotePurposeBadgeClass(s string) string {
	switch NormalizeQuotePurpose(s) {
	case QuotePurposeBudget:
		return "bg-violet-50 text-violet-800"
	case QuotePurposeReference:
		return "bg-slate-100 text-slate-600"
	default:
		return "bg-emerald-50 text-emerald-800"
	}
}

func (q *SalesQuote) ValidateOwner() error {
	if q == nil || strings.TrimSpace(q.OwnerName) == "" || strings.TrimSpace(q.OwnerPhone) == "" {
		return ErrQuoteOwnerRequired
	}
	return nil
}

func (q *SalesQuote) ValidatePurpose() error {
	if q == nil {
		return nil
	}
	q.Purpose = NormalizeQuotePurpose(q.Purpose)
	if q.Purpose == QuotePurposeBudget && q.BudgetYear <= 0 {
		return fmt.Errorf("예산용은 대상 연도가 필요합니다")
	}
	return nil
}

func (q *SalesQuote) DisplayNo() string {
	if q == nil {
		return ""
	}
	no := strings.TrimSpace(q.QuoteNo)
	if q.Rev > 0 && no != "" {
		return fmt.Sprintf("%s-%d", no, q.Rev)
	}
	return no
}

func (q *SalesQuote) SalesDisplayNo() string {
	if q == nil {
		return ""
	}
	if s := strings.TrimSpace(q.SalesNo); s != "" {
		return s
	}
	return strings.TrimSpace(q.SalesID)
}

func (q *SalesQuote) QuoteYear() int {
	if q == nil {
		return 0
	}
	t, err := ParseQuoteDateISO(q.QuoteDate)
	if err != nil {
		return 0
	}
	return t.Year()
}

func (q *SalesQuote) InPipeline() bool {
	return q != nil && NormalizeQuotePurpose(q.Purpose) == QuotePurposeDeal
}

func ParseQuoteDateISO(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("견적일이 필요합니다")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("견적일이 올바르지 않습니다")
	}
	if t.Format("2006-01-02") != s {
		return time.Time{}, fmt.Errorf("견적일이 올바르지 않습니다")
	}
	return t, nil
}

func FormatQuoteDateKorean(iso string) string {
	t, err := ParseQuoteDateISO(iso)
	if err != nil {
		return strings.TrimSpace(iso)
	}
	return t.Format("2006년 01월 02일")
}

func FormatQuoteNo(iso string, seq int) string {
	t, err := ParseQuoteDateISO(iso)
	if err != nil || seq < 1 {
		return ""
	}
	return fmt.Sprintf("%s%s-%03d", QuoteNoPrefix, t.Format("20060102"), seq)
}

func AssertQuoteNoMatchesDate(no, iso string, legacy bool) error {
	no = strings.TrimSpace(no)
	t, err := ParseQuoteDateISO(iso)
	if err != nil {
		return err
	}
	if legacy {
		compact := strings.ReplaceAll(iso, "-", "")
		yy := compact[2:]
		if strings.Contains(no, compact) || strings.Contains(no, yy) || strings.Contains(no, t.Format("060102")) {
			return nil
		}
		return fmt.Errorf("견적번호와 견적일이 다릅니다")
	}
	day := t.Format("20060102")
	if !strings.HasPrefix(no, QuoteNoPrefix) || !strings.Contains(no, day) {
		return fmt.Errorf("견적번호와 견적일이 다릅니다")
	}
	return nil
}

func GroupSubtotals(lines []SalesQuoteLine) []QuoteGroupSubtotal {
	order := []string{}
	sum := map[string]int{}
	for _, ln := range PrepareQuoteLines(lines) {
		g := strings.TrimSpace(ln.GroupLabel)
		if g == "" {
			g = "(구분 없음)"
		}
		if _, ok := sum[g]; !ok {
			order = append(order, g)
		}
		sum[g] += LineAmount(ln)
	}
	out := make([]QuoteGroupSubtotal, 0, len(order))
	for _, g := range order {
		out = append(out, QuoteGroupSubtotal{Label: g, Sum: sum[g]})
	}
	return out
}

func AssertGroupSumEquals(lines []SalesQuoteLine, want int) error {
	got := 0
	for _, g := range GroupSubtotals(lines) {
		got += g.Sum
	}
	if got != want {
		return fmt.Errorf("그룹 소계 합(%d)이 소계(%d)와 다릅니다", got, want)
	}
	return nil
}

func ReverseQuoteLines(lines []SalesQuoteLine, target int, vatMode, roundRule string) []SalesQuoteLine {
	out := PrepareQuoteLines(lines)
	if target <= 0 || len(out) == 0 {
		return out
	}
	base := sumLineAmount(out)
	if base <= 0 {
		if len(out) == 1 && out[0].Qty > 0 {
			out[0].UnitPrice = target
			out[0].Amount = LineAmount(out[0])
		}
		return out
	}
	for i := range out {
		out[i].UnitPrice = int(float64(out[i].UnitPrice) * float64(target) / float64(base))
		out[i].Amount = LineAmount(out[i])
	}
	tot := ComputeQuoteTotals(out, vatMode, roundRule)
	diff := target - tot.Total
	if diff != 0 && len(out) > 0 && out[len(out)-1].Qty > 0 {
		out[len(out)-1].UnitPrice += int(float64(diff) / out[len(out)-1].Qty)
		out[len(out)-1].Amount = LineAmount(out[len(out)-1])
	}
	return out
}

func RateDiffLabel(used, standard float64) string {
	if used <= 0 || standard <= 0 || used == standard {
		return ""
	}
	return fmt.Sprintf("표준 %g%% → %g%%", standard, used)
}

func BudgetFollowupActive(year int, today time.Time) bool {
	if year <= 0 || today.IsZero() {
		return false
	}
	return today.Year() == year && today.Month() <= 3
}

func (q SalesQuote) IsExpiredSent(today string) bool {
	if NormalizeQuoteStatus(q.Status) != QuoteStatusSent {
		return false
	}
	exp := q.expiryDate()
	return exp != "" && today > exp
}

func (q SalesQuote) IsExpiringSoon(today string) bool {
	if NormalizeQuoteStatus(q.Status) != QuoteStatusSent {
		return false
	}
	exp := q.expiryDate()
	if exp == "" || today > exp {
		return false
	}
	t, err := ParseQuoteDateISO(today)
	if err != nil {
		return false
	}
	e, err := ParseQuoteDateISO(exp)
	if err != nil {
		return false
	}
	return e.Sub(t).Hours() <= 7*24
}

func (q SalesQuote) ExpiryLabel(today string) string {
	if q.IsExpiredSent(today) {
		return "만료"
	}
	if q.IsExpiringSoon(today) {
		return "만료 임박"
	}
	return ""
}

func (q SalesQuote) expiryDate() string {
	t, err := ParseQuoteDateISO(q.QuoteDate)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 14).Format("2006-01-02")
}

type QuoteListKPI struct {
	WaitSend    int
	MonthQuotes int
	AvgDiscount string
	WinRate     string
}

func BuildQuoteListKPI(items []SalesQuote, ym string) QuoteListKPI {
	ym = strings.TrimSpace(ym)
	if ym == "" {
		ym = time.Now().Format("2006-01")
	}
	k := QuoteListKPI{AvgDiscount: "—", WinRate: "—"}
	won, lost := 0, 0
	var discSum float64
	discN := 0
	for i := range items {
		q := items[i]
		st := NormalizeQuoteStatus(q.Status)
		if st == QuoteStatusDraft {
			k.WaitSend++
		}
		if len(q.QuoteDate) >= 7 && q.QuoteDate[:7] == ym {
			k.MonthQuotes++
		}
		if st == QuoteStatusWon {
			won++
		}
		if st == QuoteStatusLost {
			lost++
		}
		for _, ln := range q.Lines {
			if ln.DiscountRate > 0 {
				discSum += ln.DiscountRate
				discN++
			}
		}
	}
	k.WinRate = FormatSalesRate(won, won+lost)
	if discN > 0 {
		k.AvgDiscount = fmt.Sprintf("%.1f%%", discSum/float64(discN))
	}
	return k
}
