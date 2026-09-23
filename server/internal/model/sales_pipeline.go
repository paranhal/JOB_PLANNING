package model

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// SalesPipeline 영업 전용 지표 (§32.11). §4 실행률·3일·7일과 섞지 않는다.
type SalesPipeline struct {
	Stages        []SalesPipelineStage
	TotalCount    int
	TotalAmount   int64
	WeightedTotal int64
	PreWonAmount  int64
	WonAmount     int64
	Months        []SalesMonthBucket
	WinContracted int
	WinLost       int
	WinRateLabel  string
	People        []SalesPersonCount
}

type SalesPipelineStage struct {
	Code          string
	Label         string
	Count         int
	Amount        int64
	Weighted      int64
	DwellDays     float64
	DwellSamples  int
	DwellLabel    string
	AmountLabel   string
	WeightedLabel string
}

type SalesMonthBucket struct {
	YM          string
	Label       string
	Count       int
	Amount      int64
	AmountLabel string
}

type SalesPersonCount struct {
	Name  string
	Count int
}

type SalesTimelineAxis struct {
	Months []string
	Rows   []SalesTimelineRow
}

type SalesTimelineRow struct {
	SalesID     string
	DisplayNo   string
	Name        string
	YM          string
	Period      string
	StageLabel  string
	Unscheduled bool
}

const salesUnscheduledYM = "unscheduled"

// FormatSalesRate 분모 0이면 — (§4.3).
func FormatSalesRate(num, den int) string {
	if den <= 0 {
		return "—"
	}
	pct := float64(num) * 100 / float64(den)
	return strconv.FormatFloat(pct, 'f', 0, 64) + "%"
}

func FormatSalesMoney(n int64) string {
	if n < 0 {
		return "-" + FormatSalesMoney(-n)
	}
	if n > int64(^uint(0)>>1) {
		n = int64(^uint(0) >> 1)
	}
	return formatSalesAmount(int(n)) + "원"
}

// FormatSalesMoneyShort 열 머리글·헤더용. 억/천만으로 줄인다 (§32.11.1).
func FormatSalesMoneyShort(n int64) string {
	if n < 0 {
		return "-" + FormatSalesMoneyShort(-n)
	}
	if n == 0 {
		return "0"
	}
	const eok = int64(100_000_000)
	const cheonman = int64(10_000_000)
	if n >= eok/10 {
		v := float64(n) / float64(eok)
		s := strconv.FormatFloat(v, 'f', 1, 64)
		s = strings.TrimSuffix(s, ".0")
		return s + "억"
	}
	if n >= cheonman {
		return strconv.FormatInt(n/cheonman, 10) + "천만"
	}
	return FormatSalesMoney(n)
}

func FormatSalesDwell(days float64, samples int) string {
	if samples <= 0 {
		return "—"
	}
	if days < 0 {
		days = 0
	}
	return strconv.FormatFloat(days, 'f', 1, 64) + "일"
}

func weightedAmount(amount int, prob int) int64 {
	if amount <= 0 || prob <= 0 {
		return 0
	}
	return int64(amount) * int64(prob) / 100
}

// BuildSalesPipeline 단계별 건수·금액·가중 파이프라인·예정월·수주율·체류일·담당자 활동.
func BuildSalesPipeline(projects []SalesProject, stages []SalesStageDef, hist []SalesStageHistory, acts []SalesActivity, now time.Time, extraPeople []string) SalesPipeline {
	if now.IsZero() {
		now = time.Now()
	}
	if len(stages) == 0 {
		stages = fallbackSalesStages()
	}
	idx := map[string]int{}
	out := SalesPipeline{WinRateLabel: "—"}
	for i, st := range stages {
		idx[st.Code] = i
		out.Stages = append(out.Stages, SalesPipelineStage{
			Code: st.Code, Label: st.Label, DwellLabel: "—",
			AmountLabel: "0원", WeightedLabel: "0원",
		})
	}

	monthMap := map[string]*SalesMonthBucket{}
	for i := range projects {
		p := projects[i]
		si, ok := idx[p.Stage]
		if !ok {
			continue
		}
		st := &out.Stages[si]
		st.Count++
		st.Amount += int64(p.ExpectedAmount)
		w := weightedAmount(p.ExpectedAmount, SalesProbability(&p))
		if p.IsSupply() && p.ExpectedAmount > 0 {
			w = int64(p.ExpectedAmount)
		}
		st.Weighted += w
		out.TotalCount++
		out.TotalAmount += int64(p.ExpectedAmount)
		out.WeightedTotal += w
		switch {
		default:
			if strings.TrimSpace(p.Stage) != SalesStage4Closed {
				out.PreWonAmount += int64(p.ExpectedAmount)
			}
		}
		ym := NormalizeSalesYM(p.ExpectedYM)
		key := ym
		label := ym
		if key == "" {
			key = salesUnscheduledYM
			label = "미정"
		}
		b := monthMap[key]
		if b == nil {
			b = &SalesMonthBucket{YM: key, Label: label}
			monthMap[key] = b
		}
		b.Count++
		b.Amount += int64(p.ExpectedAmount)
	}
	out.WinContracted, out.WinLost = SalesWinSample(projects)
	out.WinRateLabel = FormatSalesRate(out.WinContracted, out.WinContracted+out.WinLost)

	for i := range out.Stages {
		out.Stages[i].AmountLabel = FormatSalesMoney(out.Stages[i].Amount)
		out.Stages[i].WeightedLabel = FormatSalesMoney(out.Stages[i].Weighted)
	}

	var months []SalesMonthBucket
	for _, b := range monthMap {
		b.AmountLabel = FormatSalesMoney(b.Amount)
		months = append(months, *b)
	}
	sort.Slice(months, func(i, j int) bool {
		if months[i].YM == salesUnscheduledYM {
			return false
		}
		if months[j].YM == salesUnscheduledYM {
			return true
		}
		return months[i].YM < months[j].YM
	})
	out.Months = months

	dwellDays := make([]float64, len(out.Stages))
	dwellN := make([]int, len(out.Stages))
	addDwell := func(stage string, from, to time.Time) {
		si, ok := idx[stage]
		if !ok || to.Before(from) {
			return
		}
		days := to.Sub(from).Hours() / 24
		if days < 0 {
			days = 0
		}
		dwellDays[si] += days
		dwellN[si]++
	}
	bySales := map[string][]SalesStageHistory{}
	for _, h := range hist {
		bySales[h.SalesID] = append(bySales[h.SalesID], h)
	}
	for i := range projects {
		p := &projects[i]
		events := append([]SalesStageHistory(nil), bySales[p.SalesID]...)
		sort.Slice(events, func(a, b int) bool {
			if events[a].ChangedAt == events[b].ChangedAt {
				return events[a].HistoryID < events[b].HistoryID
			}
			return events[a].ChangedAt < events[b].ChangedAt
		})
		if len(events) == 0 {
			start := parseSalesAt(p.CreatedAt)
			if start.IsZero() {
				start = now
			}
			addDwell(p.Stage, start, now)
			continue
		}
		for i, ev := range events {
			from := parseSalesAt(ev.ChangedAt)
			if from.IsZero() {
				continue
			}
			to := now
			if i+1 < len(events) {
				if next := parseSalesAt(events[i+1].ChangedAt); !next.IsZero() {
					to = next
				}
			}
			stage := CurrentSalesStage(strings.TrimSpace(ev.ToStage))
			if stage == "" {
				stage = p.Stage
			}
			addDwell(stage, from, to)
		}
	}
	for i := range out.Stages {
		out.Stages[i].DwellDays = 0
		out.Stages[i].DwellSamples = dwellN[i]
		if dwellN[i] > 0 {
			out.Stages[i].DwellDays = dwellDays[i] / float64(dwellN[i])
		}
		out.Stages[i].DwellLabel = FormatSalesDwell(out.Stages[i].DwellDays, out.Stages[i].DwellSamples)
	}

	people := map[string]int{}
	for _, name := range extraPeople {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := people[name]; !ok {
			people[name] = 0
		}
	}
	for _, a := range acts {
		for _, name := range a.OurMemberNames() {
			people[name]++
		}
	}
	for name, n := range people {
		out.People = append(out.People, SalesPersonCount{Name: name, Count: n})
	}
	sort.Slice(out.People, func(i, j int) bool {
		if out.People[i].Count == out.People[j].Count {
			return out.People[i].Name < out.People[j].Name
		}
		return out.People[i].Count > out.People[j].Count
	})
	return out
}

func parseSalesAt(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// BuildSalesTimelineAxis 예정월 가로축 · 사업 세로축 (§32.8 목록 보기).
func BuildSalesTimelineAxis(projects []SalesProject, stages []SalesStageDef, now time.Time) SalesTimelineAxis {
	if now.IsZero() {
		now = time.Now()
	}
	seen := map[string]bool{}
	var yms []string
	add := func(ym string) {
		ym = NormalizeSalesYM(ym)
		if ym == "" || seen[ym] {
			return
		}
		seen[ym] = true
		yms = append(yms, ym)
	}
	cur := now.Format("2006-01")
	add(cur)
	for i := range projects {
		add(projects[i].ExpectedYM)
	}
	if len(yms) == 0 {
		add(cur)
	}
	sort.Strings(yms)
	start, _ := time.Parse("2006-01", yms[0])
	end, _ := time.Parse("2006-01", yms[len(yms)-1])
	if start.IsZero() {
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	if end.IsZero() || end.Before(start) {
		end = start
	}
	var months []string
	for t := start; !t.After(end); t = t.AddDate(0, 1, 0) {
		months = append(months, t.Format("2006-01"))
		if len(months) > 36 {
			break
		}
	}
	axis := SalesTimelineAxis{Months: months}
	for i := range projects {
		p := projects[i]
		def := FindSalesStage(stages, p.Stage)
		row := SalesTimelineRow{
			SalesID:    p.SalesID,
			DisplayNo:  p.DisplayNo(),
			Name:       p.Name,
			YM:         NormalizeSalesYM(p.ExpectedYM),
			Period:     p.PeriodLabel(),
			StageLabel: p.DisplayStage(def),
		}
		if row.YM == "" {
			row.Unscheduled = true
			row.Period = "미정"
		}
		axis.Rows = append(axis.Rows, row)
	}
	return axis
}

func SalesKanbanStages(stages []SalesStageDef) (process []SalesStageDef, lost *SalesStageDef) {
	if len(stages) == 0 {
		stages = fallbackSalesStages()
	}
	process = append(process, stages...)
	return process, nil
}

func SalesActivityColumnKey(a SalesActivity, group string) string {
	if group == "stage" {
		if s := strings.TrimSpace(a.StageAtTime); s != "" {
			return s
		}
		return "_"
	}
	if s := strings.TrimSpace(a.ActivityType); s != "" {
		return s
	}
	return "other"
}

func (p SalesPipeline) WeightedLabel() string {
	return FormatSalesMoneyShort(p.WeightedTotal)
}

func (p SalesPipeline) TotalAmountLabel() string {
	return FormatSalesMoneyShort(p.PreWonAmount)
}

func (p SalesPipeline) WonAmountLabel() string {
	return FormatSalesMoneyShort(p.WonAmount)
}

func (p SalesPipeline) WinSampleLabel() string {
	return SalesWinSampleLabel()
}
