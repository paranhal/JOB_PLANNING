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
		amt := p.PipelineAmountLabel()
		if amt == "" {
			amt = "—"
		}
		if src := SalesAmountSourceLabel(p.PipeSource, p.PipeQuoteN); src != "" {
			amt += " · " + src
		}
		out = append(out, SalesDashRow{
			SalesID: p.SalesID, DisplayNo: p.DisplayNo(), Name: p.Name, Customer: p.CustomerValue(),
			Stage: p.Stage, StageLabel: label, Amount: amt, Prob: prob,
			Next: nextTxt, Href: "/sales/" + p.SalesID,
		})
	}
	return out
}

type SalesDropBar struct {
	Code  string
	Label string
	Count int
}

type SalesDropDash struct {
	Count   int
	ByCode  []SalesDropBar
	ByStage []SalesDropBar
}

func BuildSalesDropDash(projects []SalesProject, from, to string) SalesDropDash {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	codeN := map[string]int{}
	stageN := map[string]int{}
	var n int
	for i := range projects {
		p := projects[i]
		if p.CloseReason != SalesCloseDropped {
			continue
		}
		day := strings.TrimSpace(p.DroppedAt)
		if len(day) >= 10 {
			day = day[:10]
		}
		if from != "" && day < from {
			continue
		}
		if to != "" && day > to {
			continue
		}
		n++
		code := strings.TrimSpace(p.DropReasonCode)
		if code == "" {
			code = "etc"
		}
		codeN[code]++
		st := strings.TrimSpace(p.DroppedFromStage)
		if st == "" {
			st = p.Stage
		}
		stageN[st]++
	}
	out := SalesDropDash{Count: n}
	for _, c := range []struct{ code, label string }{
		{"budget", "고객 예산 미확보·사업 취소"},
		{"postponed", "고객 사업 무기한 연기"},
		{"low_margin", "사업성 부족"},
		{"not_qualified", "참가 자격·실적 미달"},
		{"competitor", "경쟁 열세로 참여 포기"},
		{"resource", "당사 인력·일정 사정"},
		{"duplicate", "중복 등록"},
		{"etc", "기타"},
	} {
		if codeN[c.code] > 0 {
			out.ByCode = append(out.ByCode, SalesDropBar{Code: c.code, Label: c.label, Count: codeN[c.code]})
		}
	}
	for _, c := range []struct{ code, label string }{
		{SalesStage4Discover, "발굴"},
		{SalesStage4Propose, "제안"},
		{SalesStage4Bid, "입찰"},
		{SalesStage4Closed, "사업 종료"},
	} {
		if stageN[c.code] > 0 {
			out.ByStage = append(out.ByStage, SalesDropBar{Code: c.code, Label: c.label, Count: stageN[c.code]})
		}
	}
	return out
}

type SalesJudgeRow struct {
	Owner string
	Low   string
	Mid   string
	High  string
}

func frozenWinProb(p *SalesProject) int {
	if p == nil {
		return 0
	}
	if p.ProbabilityFinal != nil {
		return *p.ProbabilityFinal
	}
	if p.WinProb != nil {
		return *p.WinProb
	}
	return 0
}

func judgeBand(p int) string {
	if p >= 10 && p <= 30 {
		return "low"
	}
	if p >= 40 && p <= 60 {
		return "mid"
	}
	if p >= 70 && p <= 90 {
		return "high"
	}
	return ""
}

func judgeRateLabel(won, n int) string {
	if n < 5 {
		return "—"
	}
	return FormatSalesRate(won, n)
}

// BuildSalesJudgeAccuracy 동결 확도 구간별 실제 수주 비율. 표본 5건 미만이면 —. §47.4.3
func BuildSalesJudgeAccuracy(projects []SalesProject) []SalesJudgeRow {
	type acc struct{ won, n int }
	type bags struct{ low, mid, high acc }
	by := map[string]*bags{}
	var owners []string
	for i := range projects {
		p := projects[i]
		if p.CloseReason == SalesCloseDropped {
			continue
		}
		won := p.CloseReason == SalesCloseContracted || strings.TrimSpace(p.WonAt) != ""
		lost := p.CloseReason == SalesCloseLost && strings.TrimSpace(p.WonAt) == ""
		if !won && !lost {
			continue
		}
		band := judgeBand(frozenWinProb(&p))
		if band == "" {
			continue
		}
		name := strings.TrimSpace(p.SalesOwner)
		if name == "" {
			name = "(담당 없음)"
		}
		b := by[name]
		if b == nil {
			b = &bags{}
			by[name] = b
			owners = append(owners, name)
		}
		var cell *acc
		switch band {
		case "low":
			cell = &b.low
		case "mid":
			cell = &b.mid
		default:
			cell = &b.high
		}
		cell.n++
		if won {
			cell.won++
		}
	}
	sort.Strings(owners)
	out := make([]SalesJudgeRow, 0, len(owners))
	for _, name := range owners {
		b := by[name]
		out = append(out, SalesJudgeRow{
			Owner: name,
			Low:   judgeRateLabel(b.low.won, b.low.n),
			Mid:   judgeRateLabel(b.mid.won, b.mid.n),
			High:  judgeRateLabel(b.high.won, b.high.n),
		})
	}
	return out
}
