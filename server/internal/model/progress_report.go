package model

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Josa 는 단어 끝 받침으로 조사를 고른다. 한글이 아니면 받침 없음으로 본다.
// kind: 은는, 이가, 을를, 으로로. ㄹ 받침의 으로/로는 로.
func Josa(word, kind string) string {
	batchim, _ := hangulBatchim(word)
	rieul := endsWithRieul(word)
	switch kind {
	case "은는":
		if batchim {
			return "은"
		}
		return "는"
	case "이가":
		if batchim {
			return "이"
		}
		return "가"
	case "을를":
		if batchim {
			return "을"
		}
		return "를"
	default:
		if batchim && !rieul {
			return "으로"
		}
		return "로"
	}
}

func hangulBatchim(word string) (batchim bool, hangul bool) {
	word = strings.TrimSpace(word)
	if word == "" {
		return false, false
	}
	r, _ := utf8.DecodeLastRuneInString(word)
	if r < 0xAC00 || r > 0xD7A3 {
		return false, false
	}
	return (r-0xAC00)%28 != 0, true
}

func endsWithRieul(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(word)
	if r < 0xAC00 || r > 0xD7A3 {
		return false
	}
	return (r-0xAC00)%28 == 8
}

type PaceFacts struct {
	PlanEnd, Today string
	Done, Total    int
	ActualDays     int
	WindowDays     int
	Amount         float64
	Confirmed      bool
	Biz            func(string) bool
	Unit           string
}

type PaceResult struct {
	Label  string
	Avg    float64
	Need   int
	Expect string
	Ahead  int
	Window int
	Unit   string
}

func (p PaceResult) Note() string {
	if p.Label == "" || p.Label == PaceBefore || p.Label == PaceDone || p.Avg <= 0 {
		return ""
	}
	unit := p.Unit
	if unit == "" {
		unit = "곳"
	}
	s := fmt.Sprintf("참고 · 평균 %s%s/일(최근 %d영업일)", formatAvg(p.Avg), unit, p.Window)
	if p.Need > 0 {
		s += fmt.Sprintf(" · 필요 %d%s/일", p.Need, unit)
	}
	return s
}

// AssessPace 설계서 §5-3. 여유율과 임계값은 이 함수와 상수만 본다.
func AssessPace(f PaceFacts) PaceResult {
	out := PaceResult{Label: PaceBefore, Unit: strings.TrimSpace(f.Unit), Window: f.WindowDays, Expect: "—"}
	if out.Unit == "" {
		out.Unit = "곳"
	}
	end := strings.TrimSpace(f.PlanEnd)
	today := strings.TrimSpace(f.Today)
	if end == "" || f.Biz == nil || f.Total <= 0 {
		return out
	}
	if f.Confirmed || f.Done >= f.Total {
		out.Label = PaceDone
		return out
	}
	if f.ActualDays < PaceMinActualDays {
		return out
	}
	if today > end {
		out.Label = PaceOver
		return out
	}
	window := f.WindowDays
	if window < 1 {
		window = 1
	}
	out.Window = window
	remain := f.Total - f.Done
	if remain < 0 {
		remain = 0
	}
	avg := f.Amount / float64(window)
	out.Avg = avg
	if avg <= 0 {
		out.Label = PaceRisk
		out.Expect = "—"
		return out
	}
	needDays := int(math.Ceil(float64(remain) / avg))
	out.Expect = AddBizDays(today, needDays, f.Biz)
	left := BizDaysInclusive(today, end, f.Biz)
	if left <= 0 {
		out.Label = PaceRisk
		out.Need = remain
		out.Ahead = BizBetween(out.Expect, end, f.Biz)
		return out
	}
	need := 0
	if remain > 0 {
		need = (remain + left - 1) / left
	}
	out.Need = need
	out.Ahead = BizBetween(out.Expect, end, f.Biz)
	slack := float64(need) / avg
	switch {
	case slack > PaceSlackLimit:
		out.Label = PaceRisk
	case slack > PaceSlackOK:
		out.Label = PaceWarn
	default:
		out.Label = PaceOK
	}
	return out
}

type DayAmt struct {
	Date       string
	Sites, Qty int
}

// WindowFacts 최근 7영업일(오늘 포함). 첫 실적이 그 창보다 뒤면 첫 실적일부터 나눈다.
func WindowFacts(today, planEnd, basis string, done, total int, confirmed bool, days []DayAmt, biz func(string) bool) PaceFacts {
	f := PaceFacts{
		PlanEnd: planEnd, Today: today, Done: done, Total: total,
		Confirmed: confirmed, Biz: biz,
	}
	if basis == BasisQty {
		f.Unit = ""
	}
	if biz == nil {
		return f
	}
	start, window := lastBizDays(today, PaceLookbackBiz, biz)
	qty := basis == BasisQty
	first := ""
	for _, d := range days {
		if amountOf(d, qty) <= 0 {
			continue
		}
		if first == "" || d.Date < first {
			first = d.Date
		}
	}
	if first != "" && first > start {
		start = first
		window = BizDaysInclusive(first, today, biz)
		if window < 1 {
			window = 1
		}
	}
	f.WindowDays = window
	for _, d := range days {
		if d.Date < start || d.Date > today || !biz(d.Date) {
			continue
		}
		n := amountOf(d, qty)
		if n > 0 {
			f.ActualDays++
			f.Amount += float64(n)
		}
	}
	return f
}

func amountOf(d DayAmt, qty bool) int {
	if qty {
		return d.Qty
	}
	return d.Sites
}

func lastBizDays(today string, n int, biz func(string) bool) (start string, count int) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(today))
	if err != nil || biz == nil || n < 1 {
		return today, 0
	}
	var dates []string
	for i := 0; i < 60 && len(dates) < n; i++ {
		d := t.AddDate(0, 0, -i).Format("2006-01-02")
		if biz(d) {
			dates = append(dates, d)
		}
	}
	if len(dates) == 0 {
		return today, 0
	}
	return dates[len(dates)-1], len(dates)
}

// BizDaysInclusive from~to 의 영업일. 양 끝 날짜가 영업일이면 포함한다.
func BizDaysInclusive(from, to string, biz func(string) bool) int {
	a, e1 := time.Parse("2006-01-02", strings.TrimSpace(from))
	b, e2 := time.Parse("2006-01-02", strings.TrimSpace(to))
	if e1 != nil || e2 != nil || biz == nil || b.Before(a) {
		return 0
	}
	n := 0
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if biz(d.Format("2006-01-02")) {
			n++
		}
	}
	return n
}

// AddBizDays today 다음 영업일부터 n일. n이 0이면 today.
func AddBizDays(today string, n int, biz func(string) bool) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(today))
	if err != nil || biz == nil {
		return "—"
	}
	if n <= 0 {
		return t.Format("2006-01-02")
	}
	got := 0
	for i := 0; i < 400 && got < n; i++ {
		t = t.AddDate(0, 0, 1)
		if biz(t.Format("2006-01-02")) {
			got++
		}
	}
	return t.Format("2006-01-02")
}

// BizBetween a에서 b까지 영업일 부호. a가 이르면 양수(목표보다 빠름).
func BizBetween(a, b string, biz func(string) bool) int {
	if a == "" || a == "—" || b == "" || biz == nil {
		return 0
	}
	if a == b {
		return 0
	}
	if a < b {
		return BizDaysInclusive(nextDay(a), b, biz)
	}
	return -BizDaysInclusive(nextDay(b), a, biz)
}

func nextDay(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func formatAvg(v float64) string {
	if v < 0 {
		v = 0
	}
	r := math.Round(v*10) / 10
	if r == math.Trunc(r) {
		return fmt.Sprintf("%.0f", r)
	}
	return fmt.Sprintf("%.1f", r)
}

// ReportSpan 오늘·이번 주(월~오늘)·이번 달.
func ReportSpan(today, which string) (from, to, label string) {
	today = strings.TrimSpace(today)
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return today, today, "오늘"
	}
	switch which {
	case "week":
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return t.AddDate(0, 0, -(wd - 1)).Format("2006-01-02"), today, "이번 주"
	case "month":
		return t.Format("2006-01-02")[:8] + "01", today, "이번 달"
	default:
		return today, today, "오늘"
	}
}

// PeriodWord 보고 기간이 오늘·이번 주·이번 달이면 그 말, 아니면 날짜 범위.
func PeriodWord(from, to, today string) string {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" || from == to {
		return "오늘"
	}
	wf, wt, wlab := ReportSpan(today, "week")
	mf, mt, mlab := ReportSpan(today, "month")
	if from == wf && to == wt {
		return wlab
	}
	if from == mf && to == mt {
		return mlab
	}
	if from == today && to == today {
		return "오늘"
	}
	return from + "~" + to
}

type ReportPlan struct {
	Name, Package, NextNote, PackageNote string
	Confirmed                            bool
	Basis, Unit                          string
	SitesDone, SitesTotal, Doing         int
	QtyDone, QtyTotal                    int
	PeriodSites, PeriodQty               int
	PlanEnd                              string
	Pace                                 PaceResult
}

// ReportPackageQty 현재 하부 업무가 수량만(사이트·계획 없음)일 때의 목표·누적. 95-176 질문 1.
type ReportPackageQty struct {
	Name        string
	Unit        string
	Done, Total int
}

type ProgressReport struct {
	Has            bool
	PeriodLabel    string
	Phase, Package string
	NextPackage    string
	Plans          []ReportPlan
	PackageQty     *ReportPackageQty
}

// BuildProjectProgressReport 설계서 §7·§7-1. 구조가 없으면 문장이 없다.
func BuildProjectProgressReport(in ProgressReport) []string {
	if !in.Has || strings.TrimSpace(in.Phase) == "" || strings.TrimSpace(in.Package) == "" {
		return nil
	}
	label := in.PeriodLabel
	if label == "" {
		label = "오늘"
	}
	plans := in.Plans
	extra := 0
	if len(plans) > 3 {
		extra = len(plans) - 3
		plans = plans[:3]
	}
	locName := ""
	for _, p := range in.Plans {
		if !p.Confirmed && strings.TrimSpace(p.Name) != "" {
			locName = p.Name
			break
		}
	}
	out := []string{locationSentence(in.Phase, in.Package, locName)}
	for _, p := range plans {
		if p.Confirmed {
			out = append(out, doneSentence(p))
			continue
		}
		if p.SitesDone == 0 && p.Doing == 0 && !(p.Basis == BasisQty && p.QtyDone > 0) {
			out = append(out, notStartedSentence(p))
		} else {
			out = append(out, progressSentence(p, label))
		}
		if strings.TrimSpace(p.PlanEnd) != "" {
			out = append(out, scheduleSentence(p))
		}
	}
	if q := in.PackageQty; q != nil && q.Total > 0 {
		out = append(out, qtyPackageSentence(*q))
	}
	if extra > 0 {
		out = append(out, fmt.Sprintf("외 %s개 계획.", FormatIntComma(extra)))
	}
	if next := nextSentence(in); next != "" {
		out = append(out, next)
	}
	return out
}

// qtyPackageSentence 수량만 하부 업무: 「환경조사는 120식 중 30식 완료(25%)했습니다.」 분모 0 이면 문장을 만들지 않는다.
func qtyPackageSentence(q ReportPackageQty) string {
	unit := strings.TrimSpace(q.Unit)
	if unit == "" {
		unit = "식"
	}
	name := strings.TrimSpace(q.Name)
	if q.Done == 0 {
		return fmt.Sprintf("%s%s 아직 착수 전입니다. 대상은 %s%s입니다.", name, Josa(name, "은는"), FormatIntComma(q.Total), unit)
	}
	if q.Done >= q.Total {
		return fmt.Sprintf("%s%s 목표 %s%s를 모두 완료했습니다.", name, Josa(name, "은는"), FormatIntComma(q.Total), unit)
	}
	return fmt.Sprintf("%s%s %s%s 중 %s%s 완료(%s)했습니다.", name, Josa(name, "은는"),
		FormatIntComma(q.Total), unit, FormatIntComma(q.Done), unit, SiteCheckRateLabel(q.Done, q.Total))
}

func locationSentence(phase, pkg, plan string) string {
	head := fmt.Sprintf("현재 %s%s 진행 중이며, %s 단계", strings.TrimSpace(phase), Josa(phase, "을를"), strings.TrimSpace(pkg))
	if strings.TrimSpace(plan) == "" {
		return head + "를 수행하고 있습니다."
	}
	return head + fmt.Sprintf("에서 %s%s 수행하고 있습니다.", strings.TrimSpace(plan), Josa(plan, "을를"))
}

func notStartedSentence(p ReportPlan) string {
	name := strings.TrimSpace(p.Name)
	if p.Basis == BasisQty {
		unit := qtyUnit(p.Unit)
		return fmt.Sprintf("%s%s 아직 착수 전입니다. 대상은 %s%s입니다.", name, Josa(name, "은는"), FormatIntComma(p.QtyTotal), unit)
	}
	return fmt.Sprintf("%s%s 아직 착수 전입니다. 대상은 %s곳입니다.", name, Josa(name, "은는"), FormatIntComma(p.SitesTotal))
}

func progressSentence(p ReportPlan, period string) string {
	name := strings.TrimSpace(p.Name)
	if p.Basis == BasisQty {
		unit := qtyUnit(p.Unit)
		pct := SiteCheckRateLabel(p.QtyDone, p.QtyTotal)
		s := fmt.Sprintf("%s %s%s 중 %s%s 완료(%s)했고(%s/%s곳)",
			name, FormatIntComma(p.QtyTotal), unit, FormatIntComma(p.QtyDone), unit, pct,
			FormatIntComma(p.SitesDone), FormatIntComma(p.SitesTotal))
		return s + periodTail(period, p.PeriodQty, unit)
	}
	pct := SiteCheckRateLabel(p.SitesDone, p.SitesTotal)
	s := fmt.Sprintf("%s %s곳 중 %s곳 완료(%s)했고", name, FormatIntComma(p.SitesTotal), FormatIntComma(p.SitesDone), pct)
	s += periodTail(period, p.PeriodSites, "곳")
	if p.QtyTotal > 0 {
		unit := qtyUnit(p.Unit)
		s = strings.TrimSuffix(s, ".")
		s += fmt.Sprintf("(수량 기준 %s/%s%s, %s).", FormatIntComma(p.QtyDone), FormatIntComma(p.QtyTotal), unit, SiteCheckRateLabel(p.QtyDone, p.QtyTotal))
	}
	return s
}

func periodTail(period string, n int, unit string) string {
	if n > 0 {
		return fmt.Sprintf(", %s %s%s%s 처리했습니다.", period, FormatIntComma(n), unit, Josa(unit, "을를"))
	}
	return fmt.Sprintf(", %s%s 처리 실적이 없습니다.", period, Josa(period, "은는"))
}

func scheduleSentence(p ReportPlan) string {
	end := strings.TrimSpace(p.PlanEnd)
	unit, total := "곳", p.SitesTotal
	if p.Basis == BasisQty {
		unit = qtyUnit(p.Unit)
		total = p.QtyTotal
	}
	s := fmt.Sprintf("%s까지 %s%s 완료 예정입니다.", end, FormatIntComma(total), unit)
	pace := p.Pace
	if pace.Label == PaceOver {
		return s + " 목표일이 지났습니다."
	}
	if pace.Label == PaceBefore || pace.Avg <= 0 {
		if pace.Label == PaceRisk && pace.Avg <= 0 && pace.Label != PaceBefore {
			return s + " 목표일까지 맞추기 어렵습니다."
		}
		return s
	}
	u := pace.Unit
	if u == "" {
		u = unit
	}
	s += fmt.Sprintf(" (최근 %d영업일 평균 %s%s/일, 예상 완료 %s", pace.Window, formatAvg(pace.Avg), u, pace.Expect)
	switch {
	case pace.Ahead > 0:
		s += fmt.Sprintf(", 목표보다 영업일 %s일 빠름)", FormatIntComma(pace.Ahead))
	case pace.Ahead < 0:
		n := -pace.Ahead
		s += fmt.Sprintf(", 목표보다 영업일 %s일 늦음. 목표일까지 하루 평균 %s%s 필요)", FormatIntComma(n), FormatIntComma(pace.Need), u)
	default:
		s += ", 목표일과 일치)"
	}
	if pace.Label == PaceRisk {
		s += fmt.Sprintf(" 예정일까지 하루 평균 %s%s%s 필요합니다(최근 평균 %s%s).", FormatIntComma(pace.Need), u, Josa(u, "이가"), formatAvg(pace.Avg), u)
	}
	if pace.Label == PaceWarn {
		s += " 여유가 적습니다."
	}
	return s
}

func doneSentence(p ReportPlan) string {
	pkg := strings.TrimSpace(p.Package)
	name := strings.TrimSpace(p.Name)
	if p.Basis == BasisQty {
		unit := qtyUnit(p.Unit)
		return fmt.Sprintf("%s %s%s 완료했습니다(%s%s).", name, pkg, Josa(pkg, "을를"), FormatIntComma(p.QtyDone), unit)
	}
	return fmt.Sprintf("%s %s%s 완료했습니다(%s곳).", name, pkg, Josa(pkg, "을를"), FormatIntComma(p.SitesDone))
}

func nextSentence(in ProgressReport) string {
	for _, p := range in.Plans {
		if note := strings.TrimSpace(p.NextNote); note != "" {
			return note
		}
	}
	for _, p := range in.Plans {
		if note := strings.TrimSpace(p.PackageNote); note != "" {
			return note
		}
	}
	if name := strings.TrimSpace(in.NextPackage); name != "" {
		return fmt.Sprintf("이후 ‘%s’ 단계를 진행할 예정입니다.", name)
	}
	return ""
}

func qtyUnit(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return "식"
	}
	return unit
}

type MilestoneView struct {
	ID, PackageID, PlanID, State, When, Note string
	Pct                                      int
}

type ChartDay struct {
	Date      string
	Done, Day int
}

// ProgressChartSVG 누적 완료 선과 하루 실적 막대. 값이 없으면 빈 문자열.
func ProgressChartSVG(days []ChartDay) string {
	if len(days) == 0 {
		return ""
	}
	max := 1
	for _, d := range days {
		if d.Done > max {
			max = d.Done
		}
		if d.Day > max {
			max = d.Day
		}
	}
	const w, h, pad = 640, 96, 8
	n := len(days)
	slot := float64(w-2*pad) / float64(n)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="100%%" height="%d" role="img" aria-label="일별 누적 실적">`, w, h, h)
	var pts []string
	for i, d := range days {
		x := pad + int(float64(i)*slot)
		bw := int(slot * 0.6)
		if bw < 1 {
			bw = 1
		}
		bh := (h - 2*pad) * d.Day / max
		y := h - pad - bh
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="currentColor" opacity="0.35"/>`, x, y, bw, bh)
		cy := h - pad - (h-2*pad)*d.Done/max
		cx := x + bw/2
		pts = append(pts, fmt.Sprintf("%d,%d", cx, cy))
	}
	fmt.Fprintf(&b, `<polyline fill="none" stroke="currentColor" stroke-width="2" points="%s"/>`, strings.Join(pts, " "))
	b.WriteString(`</svg>`)
	return b.String()
}
