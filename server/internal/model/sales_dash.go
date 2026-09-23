package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CountOpenSales 수주·실주가 아닌 진행 건. §32.11 진행 사업.
func CountOpenSales(projects []SalesProject) int {
	n := 0
	for i := range projects {
		st := CurrentSalesStage(projects[i].Stage)
		if st == SalesStage4Closed || st == SalesStageWon || st == SalesStageLost || st == SalesStageDropped || st == SalesStageDelivered {
			continue
		}
		n++
	}
	return n
}

// SumMonthWon 이번 달 수주 금액. WonAt(없으면 UpdatedAt)이 해당 월. §46.2
func SumMonthWon(projects []SalesProject, ym string) int64 {
	ym = NormalizeSalesYM(ym)
	if ym == "" {
		ym = time.Now().Format("2006-01")
	}
	var sum int64
	for i := range projects {
		p := projects[i]
		st := CurrentSalesStage(p.Stage)
		if st != SalesStageWon && !(p.IsSupply() && p.Stage == SalesStageDelivered) {
			continue
		}
		day := strings.TrimSpace(p.WonAt)
		if day == "" {
			day = strings.TrimSpace(p.UpdatedAt)
		}
		if len(day) >= 7 && day[:7] == ym {
			sum += int64(p.ExpectedAmount)
		}
	}
	return sum
}

// CountMonthClose 예정월이 이번 달인 건.
func CountMonthClose(projects []SalesProject, ym string) int {
	ym = NormalizeSalesYM(ym)
	n := 0
	for i := range projects {
		if NormalizeSalesYM(projects[i].ExpectedYM) == ym {
			n++
		}
	}
	return n
}

// CountSalesDelayed 다음 행동일이 오늘 이전이고 아직 남아 있는 사업. 월만 있는 날짜는 세지 않는다. §46.4
func CountSalesDelayed(nextBySales map[string]SalesActivity, today string) int {
	today = strings.TrimSpace(today)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	n := 0
	for _, a := range nextBySales {
		if SalesNextIsDelayed(a, today) {
			n++
		}
	}
	return n
}

func SalesNextIsDelayed(a SalesActivity, today string) bool {
	d := strings.TrimSpace(a.NextActionDate)
	if d == "" || IsSalesMonthOnly(d) {
		return false
	}
	if NormalizeAppDate(d) == "" {
		return false
	}
	if strings.TrimSpace(a.NextAction) == "" && d == "" {
		return false
	}
	return d < today
}

func SalesDnLabel(date, today string) string {
	date = strings.TrimSpace(date)
	today = strings.TrimSpace(today)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	if date == "" || IsSalesMonthOnly(date) {
		return ""
	}
	due, err1 := time.ParseInLocation("2006-01-02", date, time.Local)
	now, err2 := time.ParseInLocation("2006-01-02", today, time.Local)
	if err1 != nil || err2 != nil {
		return ""
	}
	days := int(due.Sub(now).Hours() / 24)
	if days == 0 {
		return "D-Day"
	}
	if days > 0 {
		return fmt.Sprintf("D-%d", days)
	}
	return fmt.Sprintf("D+%d", -days)
}

func SalesStageStripeClass(code string) string {
	switch CurrentSalesStage(code) {
	case SalesStageContact:
		return "bg-sky-500"
	case SalesStageLead:
		return "bg-indigo-500"
	case SalesStageProposal:
		return "bg-amber-500"
	case SalesStageNegotiation:
		return "bg-orange-500"
	case SalesStageWon, SalesStageDelivered:
		return "bg-emerald-500"
	case SalesStageLost, SalesStageDropped:
		return "bg-rose-500"
	default:
		return "bg-slate-400"
	}
}

func SalesActivityBadgeLabel(code string, types []Code) string {
	code = strings.TrimSpace(code)
	for _, t := range types {
		if t.CodeValue == code {
			return t.CodeName
		}
	}
	switch code {
	case SalesActTypeVisit:
		return "방문"
	case SalesActTypeCall:
		return "통화"
	case SalesActTypeOnline:
		return "회의"
	case SalesActTypeInternal:
		return "할일"
	default:
		if code == "" {
			return "활동"
		}
		return code
	}
}

type SalesDashRow struct {
	SalesID    string
	DisplayNo  string
	Name       string
	Customer   string
	Stage      string
	StageLabel string
	Amount     string
	Prob       string
	Next       string
	Href       string
}

func BuildSalesDashRows(projects []SalesProject, stages []SalesStageDef, next map[string]SalesActivity, limit int) []SalesDashRow {
	if limit <= 0 {
		limit = 8
	}
	var open []SalesProject
	for i := range projects {
		st := CurrentSalesStage(projects[i].Stage)
		if st == SalesStageWon || st == SalesStageLost || st == SalesStageDropped {
			continue
		}
		open = append(open, projects[i])
	}
	sort.SliceStable(open, func(i, j int) bool {
		if open[i].ExpectedAmount == open[j].ExpectedAmount {
			return open[i].Name < open[j].Name
		}
		return open[i].ExpectedAmount > open[j].ExpectedAmount
	})
	if len(open) > limit {
		open = open[:limit]
	}
	out := make([]SalesDashRow, 0, len(open))
	for i := range open {
		p := open[i]
		def := FindSalesStage(stages, p.Stage)
		label := p.DisplayStage(def)
		nextTxt := "—"
		if a, ok := next[p.SalesID]; ok {
			if t := strings.TrimSpace(a.NextAction); t != "" {
				nextTxt = t
			} else if d := strings.TrimSpace(a.NextActionDate); d != "" {
				nextTxt = d
			}
		}
		prob := ""
		if !p.IsSupply() {
			prob = fmt.Sprintf("%d%%", p.EffectiveProbability())
		}
		amt := p.AmountLabel()
		if amt == "" {
			amt = "—"
		}
		out = append(out, SalesDashRow{
			SalesID: p.SalesID, DisplayNo: p.DisplayNo(), Name: p.Name, Customer: p.CustomerValue(),
			Stage: p.Stage, StageLabel: label, Amount: amt, Prob: prob,
			Next: nextTxt, Href: "/sales/" + p.SalesID,
		})
	}
	return out
}
