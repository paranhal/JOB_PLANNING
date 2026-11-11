package model

import (
	"strings"
	"testing"
	"time"
)

func Test95_175C_Josa(t *testing.T) {
	cases := []struct{ word, kind, want string }{
		{"지역", "은는", "은"},
		{"지역", "이가", "이"},
		{"지역", "을를", "을"},
		{"지역", "으로로", "으로"},
		{"지구", "은는", "는"},
		{"지구", "이가", "가"},
		{"지구", "을를", "를"},
		{"지구", "으로로", "로"},
		{"학교", "은는", "는"},
		{"학교", "을를", "를"},
		{"A", "은는", "는"},
		{"A", "이가", "가"},
		{"A", "을를", "를"},
		{"A", "으로로", "로"},
		{"물", "으로로", "로"},
	}
	for _, c := range cases {
		if got := Josa(c.word, c.kind); got != c.want {
			t.Fatalf("%s %s → %s 기대 %s", c.word, c.kind, got, c.want)
		}
	}
}

func allBiz(string) bool { return true }

func Test95_175C_PaceBounds(t *testing.T) {
	biz := func(d string) bool { return WeekendBiz(d) }
	if PlanPace("", "2026-11-12", 1, 10, 5, 2, biz) != PaceBefore {
		t.Fatal("종료일 없으면 판정 전")
	}
	if PlanPace("2026-11-01", "2026-11-12", 1, 10, 5, 2, biz) != PaceOver {
		t.Fatal("기한 지남")
	}
	base := PaceFacts{PlanEnd: "2026-11-12", Today: "2026-11-12", Total: 10, Done: 2, ActualDays: 3, WindowDays: 1, Amount: 10, Biz: allBiz}
	check := func(remain int, want string) {
		t.Helper()
		f := base
		f.Done = f.Total - remain
		if got := AssessPace(f).Label; got != want {
			t.Fatalf("남은 %d 여유 판정 %s 기대 %s", remain, got, want)
		}
	}
	check(8, PaceOK)
	check(9, PaceWarn)
	check(10, PaceWarn)
	check(11, PaceRisk)
	zero := base
	zero.Amount = 0
	zero.PlanEnd = "2026-12-01"
	zero.WindowDays = 7
	if got := AssessPace(zero); got.Label != PaceRisk || got.Expect != "—" {
		t.Fatalf("평균 0 %+v", got)
	}
	short := AssessPace(PaceFacts{PlanEnd: "2026-12-01", Today: "2026-11-12", Total: 10, Done: 1, ActualDays: 2, WindowDays: 2, Amount: 4, Biz: allBiz})
	if short.Label != PaceBefore {
		t.Fatalf("3일 미만 %s", short.Label)
	}
	if AssessPace(PaceFacts{PlanEnd: "2026-12-01", Today: "2026-11-12", Total: 0, ActualDays: 5, Biz: allBiz}).Label != PaceBefore {
		t.Fatal("대상 0")
	}
	done := AssessPace(PaceFacts{PlanEnd: "2026-12-01", Today: "2026-11-12", Total: 4, Done: 4, ActualDays: 1, Biz: allBiz, Confirmed: true})
	if done.Label != PaceDone {
		t.Fatalf("완료 %s", done.Label)
	}
	sun := "2026-11-15"
	if time.Weekday(0) != time.Sunday {
		t.Fatal("일요일 상수")
	}
	left0 := AssessPace(PaceFacts{PlanEnd: sun, Today: sun, Total: 5, Done: 1, ActualDays: 4, WindowDays: 4, Amount: 4, Biz: WeekendBiz})
	if left0.Label != PaceRisk {
		t.Fatalf("남은 영업일 0 %s", left0.Label)
	}
	hol := map[string]bool{"2026-11-11": true}
	skip := func(d string) bool {
		if hol[d] {
			return false
		}
		return WeekendBiz(d)
	}
	if !skip("2026-11-12") || skip("2026-11-11") || skip("2026-11-14") {
		t.Fatal("휴일·주말")
	}
	days := []DayAmt{{Date: "2026-11-09", Sites: 1}, {Date: "2026-11-10", Sites: 1}, {Date: "2026-11-12", Sites: 1}}
	wf := WindowFacts("2026-11-12", "2026-12-31", BasisSite, 3, 10, false, days, skip)
	if wf.WindowDays != 3 || wf.WindowDays >= PaceLookbackBiz {
		t.Fatalf("7영업일보다 짧은 창 %d", wf.WindowDays)
	}
	if wf.ActualDays != 3 {
		t.Fatalf("실적일 %d", wf.ActualDays)
	}
	long := []DayAmt{}
	for _, d := range []string{"2026-10-26", "2026-10-27", "2026-10-28", "2026-10-29", "2026-10-30", "2026-11-02", "2026-11-03", "2026-11-04", "2026-11-05", "2026-11-06", "2026-11-09", "2026-11-10", "2026-11-12"} {
		long = append(long, DayAmt{Date: d, Sites: 1})
	}
	wide := WindowFacts("2026-11-12", "2026-12-31", BasisSite, 13, 40, false, long, WeekendBiz)
	if wide.WindowDays != PaceLookbackBiz {
		t.Fatalf("최근 7영업일 %d", wide.WindowDays)
	}
}

func cloneReport(in ProgressReport) ProgressReport {
	out := in
	out.Plans = append([]ReportPlan(nil), in.Plans...)
	return out
}

func Test95_175C_ReportSentences(t *testing.T) {
	paceFast := PaceResult{Label: PaceOK, Avg: 2, Need: 1, Expect: "2026-11-20", Ahead: 6, Window: 7, Unit: "곳"}
	base := ProgressReport{
		Has: true, PeriodLabel: "오늘", Phase: "전처리", Package: "환경조사", NextPackage: "물품",
		Plans: []ReportPlan{{
			Name: "홍천 지역", Package: "환경조사",
			SitesDone: 4, SitesTotal: 50, Doing: 1, PeriodSites: 4,
			QtyDone: 18, QtyTotal: 120, Unit: "식",
			PlanEnd: "2026-11-30", Pace: paceFast,
			NextNote: "홍천 지역 환경조사가 끝나면 신청할 예정입니다.",
		}},
	}
	got := strings.Join(BuildProjectProgressReport(base), "\n")
	want := []string{
		"현재 전처리를 진행 중이며, 환경조사 단계에서 홍천 지역을 수행하고 있습니다.",
		"홍천 지역 50곳 중 4곳 완료(8%)했고, 오늘 4곳을 처리했습니다(수량 기준 18/120식, 15%).",
		"2026-11-30까지 50곳 완료 예정입니다. (최근 7영업일 평균 2곳/일, 예상 완료 2026-11-20, 목표보다 영업일 6일 빠름)",
		"홍천 지역 환경조사가 끝나면 신청할 예정입니다.",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("없음\n%s\n---\n%s", w, got)
		}
	}
	noPlan := cloneReport(base)
	noPlan.Plans = nil
	noPlan.NextPackage = ""
	lines := BuildProjectProgressReport(noPlan)
	if len(lines) != 1 || lines[0] != "현재 전처리를 진행 중이며, 환경조사 단계를 수행하고 있습니다." {
		t.Fatalf("계획 없음 %v", lines)
	}
	if BuildProjectProgressReport(ProgressReport{}) != nil {
		t.Fatal("구조 없음")
	}
	start := cloneReport(base)
	start.Plans = []ReportPlan{{Name: "홍천 지역", SitesTotal: 50}}
	start.NextPackage = ""
	s := strings.Join(BuildProjectProgressReport(start), "\n")
	if !strings.Contains(s, "홍천 지역은 아직 착수 전입니다. 대상은 50곳입니다.") {
		t.Fatalf("착수 전 %s", s)
	}
	zero := cloneReport(base)
	zero.Plans[0].PeriodSites = 0
	zero.Plans[0].QtyTotal = 0
	z := strings.Join(BuildProjectProgressReport(zero), "\n")
	if !strings.Contains(z, "오늘은 처리 실적이 없습니다.") {
		t.Fatalf("실적 0 %s", z)
	}
	week := cloneReport(base)
	week.PeriodLabel = "이번 주"
	week.Plans[0].PeriodSites = 0
	week.Plans[0].QtyTotal = 0
	if !strings.Contains(strings.Join(BuildProjectProgressReport(week), "\n"), "이번 주는 처리 실적이 없습니다.") {
		t.Fatal("이번 주 조사")
	}
	month := cloneReport(base)
	month.PeriodLabel = "이번 달"
	month.Plans[0].PeriodSites = 0
	month.Plans[0].QtyTotal = 0
	if !strings.Contains(strings.Join(BuildProjectProgressReport(month), "\n"), "이번 달은 처리 실적이 없습니다.") {
		t.Fatal("이번 달 조사")
	}
	qty := cloneReport(base)
	qty.Plans[0].Basis = BasisQty
	qty.Plans[0].PeriodQty = 12
	qty.Plans[0].Pace.Unit = "식"
	q := strings.Join(BuildProjectProgressReport(qty), "\n")
	if !strings.Contains(q, "홍천 지역 120식 중 18식 완료(15%)했고(4/50곳), 오늘 12식을 처리했습니다.") {
		t.Fatalf("수량 문장 %s", q)
	}
	late := cloneReport(base)
	late.Plans[0].QtyTotal = 0
	late.Plans[0].Pace = PaceResult{Label: PaceRisk, Avg: 1, Need: 4, Expect: "2026-12-10", Ahead: -6, Window: 7, Unit: "곳"}
	l := strings.Join(BuildProjectProgressReport(late), "\n")
	if !strings.Contains(l, "목표보다 영업일 6일 늦음. 목표일까지 하루 평균 4곳 필요") || !strings.Contains(l, "예정일까지 하루 평균 4곳이 필요합니다(최근 평균 1곳)") {
		t.Fatalf("지연 %s", l)
	}
	warn := cloneReport(base)
	warn.Plans[0].QtyTotal = 0
	warn.Plans[0].Pace = PaceResult{Label: PaceWarn, Avg: 2, Need: 2, Expect: "2026-11-30", Ahead: 0, Window: 7, Unit: "곳"}
	if !strings.Contains(strings.Join(BuildProjectProgressReport(warn), "\n"), "목표일과 일치) 여유가 적습니다.") {
		t.Fatal("주의")
	}
	over := cloneReport(base)
	over.Plans[0].QtyTotal = 0
	over.Plans[0].Pace = PaceResult{Label: PaceOver}
	if !strings.Contains(strings.Join(BuildProjectProgressReport(over), "\n"), "목표일이 지났습니다.") {
		t.Fatal("기한 문장")
	}
	noEnd := cloneReport(base)
	noEnd.Plans[0].PlanEnd = ""
	noEnd.Plans[0].QtyTotal = 0
	if strings.Contains(strings.Join(BuildProjectProgressReport(noEnd), "\n"), "완료 예정입니다") {
		t.Fatal("종료일 없는 일정 문장")
	}
	fin := cloneReport(base)
	fin.Plans = []ReportPlan{{Name: "홍천 지역", Package: "환경조사", Confirmed: true, SitesDone: 50, SitesTotal: 50, NextNote: "다음 메모"}}
	fin.NextPackage = "물품"
	f := strings.Join(BuildProjectProgressReport(fin), "\n")
	if !strings.Contains(f, "홍천 지역 환경조사를 완료했습니다(50곳).") || !strings.Contains(f, "다음 메모") {
		t.Fatalf("완료 %s", f)
	}
	if strings.Contains(f, "수행하고 있습니다.") && strings.Contains(f, "홍천 지역을 수행") {
		t.Fatal("완료 계획을 현재 수행으로 적음")
	}
	pkgNote := cloneReport(base)
	pkgNote.Plans[0].NextNote = ""
	pkgNote.Plans[0].PackageNote = "하부 메모"
	pkgNote.Plans[0].QtyTotal = 0
	if !strings.Contains(strings.Join(BuildProjectProgressReport(pkgNote), "\n"), "하부 메모") {
		t.Fatal("하부 다음")
	}
	onlyNext := cloneReport(base)
	onlyNext.Plans[0].NextNote = ""
	onlyNext.Plans[0].PackageNote = ""
	onlyNext.Plans[0].QtyTotal = 0
	onlyNext.NextPackage = "물품"
	if !strings.Contains(strings.Join(BuildProjectProgressReport(onlyNext), "\n"), "이후 ‘물품’ 단계를 진행할 예정입니다.") {
		t.Fatal("다음 하부")
	}
	none := onlyNext
	none.NextPackage = ""
	joined := strings.Join(BuildProjectProgressReport(none), "\n")
	if strings.Contains(joined, "진행할 예정입니다") {
		t.Fatal("다음이 없는데 문장이 있다")
	}
	many := ProgressReport{Has: true, PeriodLabel: "오늘", Phase: "전처리", Package: "환경조사"}
	for _, name := range []string{"가", "나", "다", "라"} {
		many.Plans = append(many.Plans, ReportPlan{Name: name, SitesDone: 1, SitesTotal: 2, Doing: 1, PeriodSites: 1})
	}
	m := strings.Join(BuildProjectProgressReport(many), "\n")
	if !strings.Contains(m, "외 1개 계획.") || strings.Contains(m, "라 2곳") {
		t.Fatalf("여러 계획 %s", m)
	}
	latin := ProgressReport{Has: true, PeriodLabel: "오늘", Phase: "A단계", Package: "학교",
		Plans: []ReportPlan{{Name: "A", SitesDone: 1, SitesTotal: 2, Doing: 1, PeriodSites: 1}}}
	lat := strings.Join(BuildProjectProgressReport(latin), "\n")
	if !strings.Contains(lat, "A단계를 진행") || !strings.Contains(lat, "A를 수행") {
		t.Fatalf("영문 조사 %s", lat)
	}
	big := cloneReport(base)
	big.Plans[0].SitesTotal = 1200
	big.Plans[0].SitesDone = 100
	big.Plans[0].QtyTotal = 0
	big.Plans[0].PeriodSites = 1000
	if !strings.Contains(strings.Join(BuildProjectProgressReport(big), "\n"), "1,200곳 중 100곳") || !strings.Contains(strings.Join(BuildProjectProgressReport(big), "\n"), "1,000곳을") {
		t.Fatal("쉼표")
	}
	if ProgressChartSVG(nil) != "" {
		t.Fatal("빈 차트")
	}
	svg := ProgressChartSVG([]ChartDay{{Date: "2026-11-10", Done: 1, Day: 1}, {Date: "2026-11-11", Done: 1, Day: 0}})
	if strings.Count(svg, "<polyline") != 1 || strings.Count(svg, "<rect") != 2 {
		t.Fatalf("차트 %s", svg)
	}
	from, to, lab := ReportSpan("2026-11-12", "week")
	if from != "2026-11-09" || to != "2026-11-12" || lab != "이번 주" {
		t.Fatalf("주간 %s %s %s", from, to, lab)
	}
	if PeriodWord("2026-11-02", "2026-11-08", "2026-11-12") != "2026-11-02~2026-11-08" {
		t.Fatal("지난 주 범위")
	}
}
