package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type StatsHandler struct {
	repo      *repository.StatsRepo
	userRepo  *repository.UserRepo
	wbRepo    *repository.WBRepo
	workBoard *repository.WorkBoardRepo
	mntRepo   *repository.MaintenanceRepo
}

func NewStatsHandler(repo *repository.StatsRepo, userRepo *repository.UserRepo, wbRepo *repository.WBRepo, workBoard *repository.WorkBoardRepo, mntRepo *repository.MaintenanceRepo) *StatsHandler {
	return &StatsHandler{repo: repo, userRepo: userRepo, wbRepo: wbRepo, workBoard: workBoard, mntRepo: mntRepo}
}

func metricsViewData(repo *repository.StatsRepo) (base string) {
	if repo == nil {
		return ""
	}
	return repo.MetricsPolicy().BaseDate
}

func applyPlanningToKPI(workBoard *repository.WorkBoardRepo, kpi *model.StatsKPICard) {
	if workBoard == nil || kpi == nil {
		return
	}
	pc, err := workBoard.CountPlanning()
	if err != nil {
		return
	}
	rate := pc.Rate()
	kpi.PlanningRate = math.Round(rate*10) / 10
	kpi.PlanningOpen = pc.Open
	kpi.PlanningPlanned = pc.Planned
	kpi.HasPlanning = pc.HasPlanningRate()
	kpi.PlanDisplay = model.StatsReliability(pc.Open, !pc.HasPlanningRate(), kpi.PlanningRate)
}

func normalizeStatsMetric(m string) string {
	switch m {
	case model.StatsMetricCompleted, model.StatsMetricReceived, model.StatsMetricOverdue:
		return m
	default:
		return model.StatsMetricProgress
	}
}

func normalizeStatsScope(s string) string {
	if s == model.StatsScopeAssignee {
		return model.StatsScopeAssignee
	}
	return model.StatsScopeTeam
}

func normalizeStatsPeriod(p, metric string) string {
	switch p {
	case model.StatsPeriodWeek, model.StatsPeriodMonth, model.StatsPeriodRange:
		return p
	case model.StatsPeriodQuarter:
		if metric == model.StatsMetricProgress {
			return model.StatsPeriodDay
		}
		return p
	case model.StatsPeriodDay:
		return p
	default:
		return model.StatsPeriodDay
	}
}

func statsMetricLabel(m string) string {
	switch m {
	case model.StatsMetricCompleted:
		return "완료업무"
	case model.StatsMetricReceived:
		return "접수업무"
	case model.StatsMetricOverdue:
		return "지연"
	default:
		return "업무진행"
	}
}

func statsScopeLabel(s string) string {
	if s == model.StatsScopeAssignee {
		return "담당자별"
	}
	return "도서관사업팀 전체"
}

func statsPeriodLabel(p string) string {
	switch p {
	case model.StatsPeriodWeek:
		return "주간별"
	case model.StatsPeriodMonth:
		return "월별"
	case model.StatsPeriodQuarter:
		return "분기별"
	case model.StatsPeriodRange:
		return "원하는 기간"
	default:
		return "일별"
	}
}

func parseStatsQuery(c echo.Context) model.StatsQuery {
	metric := normalizeStatsMetric(c.QueryParam("metric"))
	scope := normalizeStatsScope(c.QueryParam("scope"))
	period := normalizeStatsPeriod(c.QueryParam("period"), metric)
	now := time.Now()
	q := model.StatsQuery{
		Metric:  metric,
		Scope:   scope,
		Period:  period,
		Date:    strings.TrimSpace(c.QueryParam("date")),
		Month:   strings.TrimSpace(c.QueryParam("month")),
		Quarter: strings.TrimSpace(c.QueryParam("quarter")),
		From:    strings.TrimSpace(c.QueryParam("from")),
		To:      strings.TrimSpace(c.QueryParam("to")),
	}
	// 기본값 채우기
	if q.Date == "" {
		q.Date = now.Format("2006-01-02")
	}
	if q.Period == model.StatsPeriodWeek {
		if t, err := time.ParseInLocation("2006-01-02", q.Date, now.Location()); err == nil {
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			q.Date = t.AddDate(0, 0, -(wd - 1)).Format("2006-01-02")
		}
	}
	if q.Month == "" {
		q.Month = now.Format("2006-01")
	}
	if q.Quarter == "" {
		qn := (int(now.Month())-1)/3 + 1
		q.Quarter = fmt.Sprintf("%d-Q%d", now.Year(), qn)
	}
	if q.From == "" {
		q.From = now.Format("2006-01-02")
	}
	if q.To == "" {
		q.To = now.Format("2006-01-02")
	}
	return q
}

func statsQueryString(q model.StatsQuery) string {
	v := url.Values{}
	v.Set("metric", q.Metric)
	v.Set("scope", q.Scope)
	v.Set("period", q.Period)
	switch q.Period {
	case model.StatsPeriodDay, model.StatsPeriodWeek:
		v.Set("date", q.Date)
	case model.StatsPeriodMonth:
		v.Set("month", q.Month)
	case model.StatsPeriodQuarter:
		v.Set("quarter", q.Quarter)
	case model.StatsPeriodRange:
		v.Set("from", q.From)
		v.Set("to", q.To)
	}
	return v.Encode()
}

func quarterOptions(now time.Time) []struct{ Value, Label string } {
	out := make([]struct{ Value, Label string }, 0, 8)
	y, m, _ := now.Date()
	curQ := (int(m)-1)/3 + 1
	// 최근 2년 분기
	for yy := y; yy >= y-1; yy-- {
		maxQ := 4
		if yy == y {
			maxQ = curQ
		}
		for qn := maxQ; qn >= 1; qn-- {
			out = append(out, struct{ Value, Label string }{
				Value: fmt.Sprintf("%d-Q%d", yy, qn),
				Label: fmt.Sprintf("%d년 %d분기", yy, qn),
			})
		}
	}
	return out
}

func monthOptions(now time.Time) []struct{ Value, Label string } {
	out := make([]struct{ Value, Label string }, 0, 24)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for i := 0; i < 24; i++ {
		t := start.AddDate(0, -i, 0)
		out = append(out, struct{ Value, Label string }{
			Value: t.Format("2006-01"),
			Label: t.Format("2006년 01월"),
		})
	}
	return out
}

func weekOptions(now time.Time) []struct{ Value, Label string } {
	out := make([]struct{ Value, Label string }, 0, 16)
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	wd := int(today.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon := today.AddDate(0, 0, -(wd - 1))
	for i := 0; i < 16; i++ {
		w := mon.AddDate(0, 0, -7*i)
		sun := w.AddDate(0, 0, 6)
		_, wn := w.ISOWeek()
		out = append(out, struct{ Value, Label string }{
			Value: w.Format("2006-01-02"),
			Label: fmt.Sprintf("%d년 %d주 (%s~%s)", w.Year(), wn, w.Format("01/02"), sun.Format("01/02")),
		})
	}
	return out
}

// Overview 일일/주간/월간 KPI·차트·예정·접수·처리 요약
func (h *StatsHandler) Overview(c echo.Context) error {
	now := time.Now()
	metricsBase := metricsViewData(h.repo)
	lb := parseLookback(c, metricsBase, now)
	filter := repository.ParseMeetingFilter(c.QueryParam("scope"), c.QueryParam("key"), c.QueryParam("project"))
	filter.IncludeImport = c.QueryParam("import") == "1"
	filter.ExcludeSalesActivity = c.QueryParam("sales") == "0"
	fromIncl, toIncl := lookbackTimes(lb, now)

	cols := repository.BuildStatsRangeColumns(fromIncl, toIncl)
	if err := h.repo.FillPeriodOverview(cols, filter); err != nil {
		return err
	}
	kpi, err := h.repo.LoadStatsKPI(model.StatsViewRange, cols, filter)
	if err != nil {
		return err
	}
	applyPlanningToKPI(h.workBoard, &kpi)
	var missingComplete repository.MissingCompleteDates
	if h.wbRepo != nil {
		missingComplete, _ = h.wbRepo.CountMissingCompleteDates()
	}
	series, err := h.repo.LoadStatsChartSeriesWindow(lb.View, fromIncl, toIncl, filter)
	if err != nil {
		return err
	}
	seriesJSON, _ := json.Marshal(series)

	var completedRows []model.StatsRow
	var completedRange string
	var analysis model.StatsWorkAnalysis
	var spotlight model.StatsSpotlight
	if len(cols) >= 2 {
		cur := cols[1]
		completedRows, err = h.repo.ListCompletedDetail(cur.From, cur.ToExclusive, filter)
		if err != nil {
			return err
		}
		completedRange = lb.Headline
		analysis, err = h.repo.LoadStatsWorkAnalysis(cur.From, cur.ToExclusive, filter)
		if err != nil {
			return err
		}
		spotlight, err = h.repo.LoadStatsSpotlight(cur.From, cur.ToExclusive, filter, analysis.Receipt)
		if err != nil {
			return err
		}
	}

	viewLabel := "일별"
	switch lb.View {
	case model.StatsViewWeek:
		viewLabel = "주별"
	case model.StatsViewMonth:
		viewLabel = "월별"
	}

	var assignees []model.User
	if h.userRepo != nil {
		assignees, _ = h.userRepo.ListAssignable()
	}
	var projects []model.WorkProject
	if h.wbRepo != nil {
		projects, _ = h.wbRepo.ListProjects(true)
	}

	anchor := now
	return c.Render(http.StatusOK, "stats/overview.html", map[string]interface{}{
		"Title":             "통계",
		"Active":            NavStats,
		"View":              lb.View,
		"ViewLabel":         viewLabel,
		"Lookback":          lb,
		"LookbackQS":        template.URL(lb.QueryValues()),
		"LookbackViewID":    "statsLookbackView",
		"Columns":           cols,
		"KPI":               kpi,
		"SeriesJSON":        template.JS(string(seriesJSON)),
		"CompletedRows":     completedRows,
		"CompletedRange":    completedRange,
		"Analysis":          analysis,
		"Spotlight":         spotlight,
		"VisitTarget":       model.StatsVisitTargetDays,
		"VisitLeadHint":     model.StatsVisitLeadHint,
		"MetricScopeAS":     model.StatsMetricScopeASOnly,
		"MetricScopeCounts": model.StatsMetricScopeCounts,
		"CompleteTarget":    model.StatsCompleteTargetDays,
		"PlanTarget":        model.StatsPlanTargetPct,
		"ChartBucketNote":   model.ChartBucketNote(lb.View),
		"Filter":            filter,
		"Assignees":         assignees,
		"Projects":          projects,
		"ProductOptions":    repository.StatsProductOptions(),
		"FilterQ":           statsMeetingFilterQuery(filter),
		"MetricsBaseDate":   metricsBase,
		"AnchorDate":        anchor.Format("2006-01-02"),
		"AnchorMonth":       anchor.Format("2006-01"),
		"AnchorMonthLabel":  anchor.Format("2006년 01월"),
		"Today":             now.Format("2006-01-02"),
		"CurrentMonth":      now.Format("2006-01"),
		"RangeFrom":         lb.From,
		"RangeTo":           lb.To,
		"MissingComplete":   missingComplete,
	})
}

func statsMeetingFilterQuery(f model.StatsMeetingFilter) string {
	q := url.Values{}
	if f.Scope != "" && f.Scope != model.StatsScopeTeam {
		q.Set("scope", f.Scope)
		if f.Key != "" {
			q.Set("key", f.Key)
		}
	}
	if f.ProjectID != "" {
		q.Set("project", f.ProjectID)
	}
	if f.IncludeImport {
		q.Set("import", "1")
	}
	if f.ExcludeSalesActivity {
		q.Set("sales", "0")
	}
	s := q.Encode()
	if s == "" {
		return ""
	}
	return "&" + s
}

func statsOverviewShift(view string, anchor time.Time, dir int) time.Time {
	switch view {
	case model.StatsViewWeek:
		return anchor.AddDate(0, 0, 7*dir)
	case model.StatsViewMonth:
		return time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location()).AddDate(0, dir, 0)
	default:
		return anchor.AddDate(0, 0, dir)
	}
}

func (h *StatsHandler) List(c echo.Context) error {
	q := parseStatsQuery(c)
	now := time.Now()
	_, _, _, rangeLabel := repository.ResolveStatsRange(q, now)

	rows, err := h.repo.ListDetail(q)
	if err != nil {
		return err
	}
	assignees, err := h.repo.ListByAssignee(q)
	if err != nil {
		return err
	}
	total := 0
	for _, a := range assignees {
		total += a.Count
	}

	// 담당자별: 사람 단위로 건 리스트 그룹핑
	groups := groupStatsByAssignee(rows)

	metricsBase := metricsViewData(h.repo)

	periodOptions := []struct{ Value, Label string }{
		{model.StatsPeriodDay, "일별"},
		{model.StatsPeriodWeek, "주간별"},
		{model.StatsPeriodMonth, "월별"},
	}
	if q.Metric != model.StatsMetricProgress {
		periodOptions = append(periodOptions, struct{ Value, Label string }{model.StatsPeriodQuarter, "분기별"})
	}
	periodOptions = append(periodOptions, struct{ Value, Label string }{model.StatsPeriodRange, "원하는 기간"})

	metricOptions := []struct{ Value, Label string }{
		{model.StatsMetricProgress, "업무진행"},
		{model.StatsMetricCompleted, "완료업무"},
		{model.StatsMetricReceived, "접수업무"},
		{model.StatsMetricOverdue, "지연"},
	}
	scopeOptions := []struct{ Value, Label string }{
		{model.StatsScopeTeam, "도서관사업팀 전체"},
		{model.StatsScopeAssignee, "담당자별"},
	}

	return c.Render(http.StatusOK, "stats/list.html", map[string]interface{}{
		"Title":           "통계 상세",
		"Active":          NavStatsDetail,
		"Query":           q,
		"QueryString":     statsQueryString(q),
		"Metric":          q.Metric,
		"MetricLabel":     statsMetricLabel(q.Metric),
		"MetricOptions":   metricOptions,
		"Scope":           q.Scope,
		"ScopeLabel":      statsScopeLabel(q.Scope),
		"ScopeOptions":    scopeOptions,
		"Period":          q.Period,
		"PeriodLabel":     statsPeriodLabel(q.Period),
		"PeriodOptions":   periodOptions,
		"RangeLabel":      rangeLabel,
		"MonthOptions":    monthOptions(now),
		"WeekOptions":     weekOptions(now),
		"QuarterOptions":  quarterOptions(now),
		"Rows":            rows,
		"Assignees":       assignees,
		"AssigneeGroups":  groups,
		"AssigneeTotal":   total,
		"RowCount":        len(rows),
		"ShowTeam":        q.Scope == model.StatsScopeTeam,
		"ShowAssignee":    q.Scope == model.StatsScopeAssignee,
		"OverdueNote":     q.Metric == model.StatsMetricOverdue,
		"MetricsBaseDate": metricsBase,
	})
}

func groupStatsByAssignee(rows []model.StatsRow) []model.StatsAssigneeGroup {
	order := []string{}
	m := map[string][]model.StatsRow{}
	for _, r := range rows {
		name := r.Assignee
		if name == "" {
			name = "(미배정)"
		}
		if _, ok := m[name]; !ok {
			order = append(order, name)
		}
		m[name] = append(m[name], r)
	}
	out := make([]model.StatsAssigneeGroup, 0, len(order))
	for _, name := range order {
		rs := m[name]
		out = append(out, model.StatsAssigneeGroup{
			Assignee: name,
			Count:    len(rs),
			Rows:     rs,
		})
	}
	return out
}

func (h *StatsHandler) ExportExcel(c echo.Context) error {
	q := parseStatsQuery(c)
	rows, err := h.repo.ListDetail(q)
	if err != nil {
		return err
	}
	assignees, err := h.repo.ListByAssignee(q)
	if err != nil {
		return err
	}

	f := excelize.NewFile()
	defer f.Close()
	sheet := statsMetricLabel(q.Metric)
	f.SetSheetName("Sheet1", sheet)
	if err := writeStatsSheet(f, sheet, rows, assignees); err != nil {
		return err
	}
	// 요약 시트
	sumSheet := "담당자별"
	f.NewSheet(sumSheet)
	_ = f.SetCellValue(sumSheet, "A1", "수행담당자")
	_ = f.SetCellValue(sumSheet, "B1", "건수")
	total := 0
	for i, a := range assignees {
		_ = f.SetCellValue(sumSheet, fmt.Sprintf("A%d", i+2), a.Assignee)
		_ = f.SetCellValue(sumSheet, fmt.Sprintf("B%d", i+2), a.Count)
		total += a.Count
	}
	_ = f.SetCellValue(sumSheet, fmt.Sprintf("A%d", len(assignees)+2), "종합")
	_ = f.SetCellValue(sumSheet, fmt.Sprintf("B%d", len(assignees)+2), total)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	name := fmt.Sprintf("stats_%s_%s", q.Metric, q.Period)
	return writeExcelDownload(c, buf.Bytes(), name)
}

func writeStatsSheet(f *excelize.File, sheet string, rows []model.StatsRow, assignees []model.StatsAssigneeRow) error {
	headers := []string{
		"제품분류", "제품모델명", "업무형태", "접수형태", "거래처명",
		"수행담당자", "접수내용", "조치내용", "처리상태",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, row := range rows {
		vals := []interface{}{
			row.ProductCategory, row.ProductModel, row.WorkForm, row.ReceiptForm, row.CustomerName,
			row.Assignee, row.Symptom, row.ActionTaken, row.StatusLabel,
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	start := len(rows) + 4
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start), "담당자별")
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start+1), "수행담당자")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", start+1), "건수")
	total := 0
	for i, a := range assignees {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start+2+i), a.Assignee)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", start+2+i), a.Count)
		total += a.Count
	}
	sumRow := start + 2 + len(assignees)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", sumRow), "종합")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", sumRow), total)
	return nil
}
