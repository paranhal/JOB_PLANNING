package model

import (
	"sort"
	"strings"
	"time"
)

func SalesSupplyAmount(amount int, vatIncluded bool) int {
	if amount <= 0 {
		return 0
	}
	if !vatIncluded {
		return amount
	}
	return int(float64(amount) / 1.1)
}

func SalesYMOf(p SalesProject, basis string) string {
	switch strings.TrimSpace(basis) {
	case "contract":
		if len(p.ContractedAt) >= 7 {
			return p.ContractedAt[:7]
		}
	case "won":
		if len(p.WonAt) >= 7 {
			return p.WonAt[:7]
		}
	case "bid":
		return NormalizeSalesYM(p.BidYM)
	case "plan":
		return NormalizeSalesYM(p.ExpectedYM)
	default:
		return NormalizeSalesYM(p.RevenueYM)
	}
	return ""
}

type SalesForecastOpts struct {
	Basis  string
	From   string
	To     string
	Group  string
	Cell   string
	Spread bool
	Now    time.Time
}

type SalesForecastRow struct {
	Project  SalesProject
	BasisYM  string
	Unsched  bool
	Amount   int
	Supply   int
	Weighted int
	VATLabel string
	Source   string
}

type SalesForecastSummary struct {
	OpenN, OpenAmt, Weighted, ContractAmt int
	UnschedN, UnschedAmt                  int
}

type SalesMonthMatrix struct {
	Months  []string
	Rows    []SalesMonthMatrixRow
	Totals  []int
	WTotals []int
}

type SalesMonthMatrixRow struct {
	Key      string
	Label    string
	Cells    []int
	WCells   []int
	Unsched  int
	WUnsched int
}

type SalesForecast struct {
	Opts    SalesForecastOpts
	Rows    []SalesForecastRow
	Summary SalesForecastSummary
	Matrix  SalesMonthMatrix
}

func DefaultForecastOpts(now time.Time) SalesForecastOpts {
	if now.IsZero() {
		now = time.Now()
	}
	from := now.Format("2006-01")
	toT := now.AddDate(0, 11, 0)
	return SalesForecastOpts{Basis: "revenue", From: from, To: toT.Format("2006-01"), Group: "stage", Cell: "amount", Now: now}
}

type forecastAcc struct {
	label string
	cells []int
	w     []int
	u, wu int
}

func BuildSalesForecast(projects []SalesProject, opts SalesForecastOpts) SalesForecast {
	if opts.Basis == "" {
		opts.Basis = "revenue"
	}
	if opts.Group == "" {
		opts.Group = "stage"
	}
	months := ymRange(opts.From, opts.To)
	out := SalesForecast{Opts: opts}
	monthIdx := map[string]int{}
	for i, m := range months {
		monthIdx[m] = i
	}
	type acc = forecastAcc
	groups := map[string]*acc{}
	groupOrder := []string{}
	seenUnique := map[string]bool{}
	tot := make([]int, len(months))
	wtot := make([]int, len(months))
	var totU, totWU int

	for i := range projects {
		p := projects[i]
		if p.Status == SalesStatusDormant {
			continue
		}
		if p.CloseReason == SalesCloseLost || p.CloseReason == SalesCloseDropped || p.Status == SalesStatusLost {
			continue
		}
		if p.PipeAmount == 0 && p.PipeSource == "" {
			ApplySalesPipelineAmount(&p, nil)
		}
		amt := p.PipeAmount
		sup := SalesSupplyAmount(amt, p.AmountVATIncluded)
		w64 := weightedAmount(sup, SalesProbability(&p))
		w := int(w64)
		if p.IsSupply() && amt > 0 {
			w = sup
		}
		ym := SalesYMOf(p, opts.Basis)
		un := ym == ""
		row := SalesForecastRow{
			Project: p, BasisYM: ym, Unsched: un, Amount: amt, Supply: sup, Weighted: w,
			VATLabel: map[bool]string{true: "VAT 포함", false: "VAT 별도"}[p.AmountVATIncluded],
			Source:   SalesAmountSourceLabel(p.PipeSource, p.PipeQuoteN),
		}
		out.Rows = append(out.Rows, row)
		out.Summary.OpenN++
		out.Summary.OpenAmt += sup
		out.Summary.Weighted += w
		if p.CloseReason == SalesCloseContracted {
			out.Summary.ContractAmt += sup
		}
		if un {
			out.Summary.UnschedN++
			out.Summary.UnschedAmt += sup
		}

		keys := forecastGroupKeys(p, opts.Group)
		dup := opts.Group == "group" && len(keys) > 1
		for _, k := range keys {
			g := groups[k.key]
			if g == nil {
				g = &acc{label: k.label, cells: make([]int, len(months)), w: make([]int, len(months))}
				groups[k.key] = g
				groupOrder = append(groupOrder, k.key)
			}
			putForecastCells(g, monthIdx, months, p, opts, sup, w, un)
		}
		if !seenUnique[p.SalesID] {
			seenUnique[p.SalesID] = true
			if !dup {
				putForecastTotals(tot, wtot, &totU, &totWU, monthIdx, months, p, opts, sup, w, un)
			} else {
				putForecastTotals(tot, wtot, &totU, &totWU, monthIdx, months, p, opts, sup, w, un)
			}
		}
		_ = dup
	}

	sort.Strings(groupOrder)
	mat := SalesMonthMatrix{Months: months, Totals: tot, WTotals: wtot}
	for _, k := range groupOrder {
		g := groups[k]
		mat.Rows = append(mat.Rows, SalesMonthMatrixRow{
			Key: k, Label: g.label, Cells: g.cells, WCells: g.w, Unsched: g.u, WUnsched: g.wu,
		})
	}
	out.Matrix = mat
	return out
}

type gkey struct{ key, label string }

func forecastGroupKeys(p SalesProject, group string) []gkey {
	switch group {
	case "owner":
		n := strings.TrimSpace(p.SalesOwner)
		if n == "" {
			n = "(미지정)"
		}
		return []gkey{{n, n}}
	case "target":
		n := strings.TrimSpace(p.ContractTarget)
		if n == "" {
			n = "(미정)"
		}
		return []gkey{{n, n}}
	case "group":
		if len(p.ForecastGroups) == 0 {
			return []gkey{{"(대분류 없음)", "(대분류 없음)"}}
		}
		var out []gkey
		for _, n := range p.ForecastGroups {
			out = append(out, gkey{n, n})
		}
		return out
	default:
		lab := p.StageLabel
		if lab == "" {
			lab = p.Stage
		}
		return []gkey{{p.Stage, lab}}
	}
}

func putForecastCells(g *forecastAcc, idx map[string]int, months []string, p SalesProject, opts SalesForecastOpts, sup, w int, un bool) {
	parts := spreadAmounts(p, opts, sup, w, un)
	for _, pt := range parts {
		if pt.un {
			g.u += pt.amt
			g.wu += pt.w
			continue
		}
		i, ok := idx[pt.ym]
		if !ok {
			continue
		}
		g.cells[i] += pt.amt
		g.w[i] += pt.w
	}
}

func putForecastTotals(tot, wtot []int, totU, totWU *int, idx map[string]int, months []string, p SalesProject, opts SalesForecastOpts, sup, w int, un bool) {
	parts := spreadAmounts(p, opts, sup, w, un)
	for _, pt := range parts {
		if pt.un {
			*totU += pt.amt
			*totWU += pt.w
			continue
		}
		i, ok := idx[pt.ym]
		if !ok {
			continue
		}
		tot[i] += pt.amt
		wtot[i] += pt.w
	}
}

type spreadPart struct {
	ym  string
	amt int
	w   int
	un  bool
}

func spreadAmounts(p SalesProject, opts SalesForecastOpts, sup, w int, un bool) []spreadPart {
	if un {
		return []spreadPart{{un: true, amt: sup, w: w}}
	}
	if !opts.Spread {
		return []spreadPart{{ym: SalesYMOf(p, opts.Basis), amt: sup, w: w}}
	}
	from := NormalizeSalesYM(p.RevenueFrom)
	to := NormalizeSalesYM(p.RevenueTo)
	cyc := strings.TrimSpace(p.BillingCycle)
	if from == "" || to == "" || cyc == "" {
		ym := SalesYMOf(p, opts.Basis)
		if ym == "" {
			ym = from
		}
		if ym == "" {
			return []spreadPart{{un: true, amt: sup, w: w}}
		}
		return []spreadPart{{ym: ym, amt: sup, w: w}}
	}
	step := 1
	switch cyc {
	case "once":
		return []spreadPart{{ym: from, amt: sup, w: w}}
	case "quarter":
		step = 3
	case "half":
		step = 6
	case "year":
		step = 12
	}
	ys := ymRange(from, to)
	var slots []string
	for i := 0; i < len(ys); i += step {
		slots = append(slots, ys[i])
	}
	if len(slots) == 0 {
		return []spreadPart{{ym: from, amt: sup, w: w}}
	}
	n := len(slots)
	base, rem := sup/n, sup%n
	wb, wr := w/n, w%n
	var out []spreadPart
	for i, ym := range slots {
		a, ww := base, wb
		if i == n-1 {
			a += rem
			ww += wr
		}
		out = append(out, spreadPart{ym: ym, amt: a, w: ww})
	}
	return out
}

func ymRange(from, to string) []string {
	from, to = NormalizeSalesYM(from), NormalizeSalesYM(to)
	if from == "" || to == "" {
		return nil
	}
	a, err1 := time.Parse("2006-01", from)
	b, err2 := time.Parse("2006-01", to)
	if err1 != nil || err2 != nil {
		return nil
	}
	if b.Before(a) {
		a, b = b, a
	}
	var out []string
	for !a.After(b) {
		out = append(out, a.Format("2006-01"))
		a = a.AddDate(0, 1, 0)
	}
	return out
}

func BillingCycleLabel(s string) string {
	switch strings.TrimSpace(s) {
	case "once":
		return "일시"
	case "month":
		return "월"
	case "quarter":
		return "분기"
	case "half":
		return "반기"
	case "year":
		return "연"
	default:
		return s
	}
}

func ForecastBasisLabel(s string) string {
	switch s {
	case "contract":
		return "계약 시기"
	case "won":
		return "수주 시기"
	case "bid":
		return "입찰 시기"
	case "plan":
		return "사업 시기"
	default:
		return "매출 시기"
	}
}

func (s SalesForecastSummary) OpenAmtLabel() string { return FormatSalesMoney(int64(s.OpenAmt)) }
