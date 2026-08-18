package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	weeklyStatsSheet = "통계"
	weeklyListSheet  = "전체 리스트"
)

// ExportWeeklyReport 주간업무보고서 엑셀 (통계 · 전체 리스트). §16.1
func (h *StatsHandler) ExportWeeklyReport(c echo.Context) error {
	now := time.Now()
	p := parseReportPeriod(c, now)
	var (
		rep model.WeeklyReport
		err error
	)
	if p.Period == model.StatsPeriodWeek {
		rep, err = h.repo.BuildWeeklyReport(p.From)
	} else {
		rep, err = h.repo.BuildWeeklyReportRange(p.FromStr, p.ToExStr)
	}
	if err != nil {
		return err
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := writeWeeklyStatsSheet(f, rep); err != nil {
		return err
	}
	if err := writeWeeklyAllListSheet(f, rep); err != nil {
		return err
	}
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	day := p.FileDay
	if day == "" {
		day = strings.ReplaceAll(rep.Friday, "-", "")
	}
	if day == "" {
		day = now.Format("20060102")
	}
	ascii := day + "_weekly_report.xlsx"
	utf8 := day + "_주간업무보고.xlsx"
	return writeExcelFile(c, buf.Bytes(), ascii, utf8)
}

type reportPeriod struct {
	Period  string
	From    time.Time
	ToEx    time.Time
	FromStr string
	ToStr   string
	ToExStr string
	FileDay string
}

func parseReportPeriod(c echo.Context, now time.Time) reportPeriod {
	period := strings.TrimSpace(c.QueryParam("period"))
	switch period {
	case model.StatsPeriodMonth, model.StatsPeriodRange:
	default:
		period = model.StatsPeriodWeek
	}
	q := model.StatsQuery{
		Period: period,
		Date:   c.QueryParam("date"),
		Month:  c.QueryParam("month"),
		From:   c.QueryParam("from"),
		To:     c.QueryParam("to"),
	}
	from, toEx, ok, _ := repository.ResolveStatsRange(q, now)
	if !ok {
		period = model.StatsPeriodWeek
		from, toEx, _, _ = repository.ResolveStatsRange(model.StatsQuery{
			Period: model.StatsPeriodWeek, Date: now.Format("2006-01-02"),
		}, now)
	}
	toInc := toEx.AddDate(0, 0, -1)
	fileDay := toInc.Format("20060102")
	if period == model.StatsPeriodWeek {
		fileDay = from.AddDate(0, 0, 4).Format("20060102")
	}
	return reportPeriod{
		Period:  period,
		From:    from,
		ToEx:    toEx,
		FromStr: from.Format("2006-01-02"),
		ToStr:   toInc.Format("2006-01-02"),
		ToExStr: toEx.Format("2006-01-02"),
		FileDay: fileDay,
	}
}

func writeWeeklyStatsSheet(f *excelize.File, rep model.WeeklyReport) error {
	idx, err := f.NewSheet(weeklyStatsSheet)
	if err != nil {
		return err
	}
	f.SetActiveSheet(idx)

	title := fmt.Sprintf("[통계]   %s ~ %s (워킹데이 %d일) · 과거 이관 데이터 제외",
		rep.WeekFrom, rep.WeekTo, rep.WorkingDays)
	if rep.HolidayYearMissing {
		title += " · 공휴일 미등록"
	}
	_ = f.SetCellValue(weeklyStatsSheet, "A1", title)
	_ = f.MergeCell(weeklyStatsSheet, "A1", "J1")

	headers := []string{"구분", "계획대비 실행률", "신뢰도", "접수", "완료", "일일 평균", "일일 최대", "이월", "진행중", "미계획"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(weeklyStatsSheet, cell, h)
	}

	if st, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: "FFFFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FF1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    weeklyThinBorder(),
	}); err == nil {
		_ = f.SetCellStyle(weeklyStatsSheet, "A3", "J3", st)
	}
	teamSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Fill:   excelize.Fill{Type: "pattern", Color: []string{"FFDBEAFE"}, Pattern: 1},
		Border: weeklyThinBorder(),
	})
	okSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FF166534"},
		Border: weeklyThinBorder(),
	})
	badSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FFB91C1C"},
		Border: weeklyThinBorder(),
	})
	naSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FF6B7280"},
		Border: weeklyThinBorder(),
	})
	plainSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕"},
		Border: weeklyThinBorder(),
	})

	for i, row := range rep.PersonRows {
		r := 4 + i
		vals := []interface{}{
			row.Label,
			weeklyRateText(row.ExecDisplay),
			weeklyGradeText(row.ExecDisplay),
			row.Receipt,
			row.Completed,
			weeklyDayAvgText(row),
			weeklyDayMaxText(row),
			row.CarryOut,
			row.InProgress,
			row.Unplanned,
		}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, r)
			_ = f.SetCellValue(weeklyStatsSheet, cell, v)
			st := plainSt
			if row.IsTeam {
				st = teamSt
			}
			if j == 1 {
				st = weeklyTargetStyle(row.ExecDisplay, true, model.StatsExecTargetPct, okSt, badSt, naSt, st)
			}
			_ = f.SetCellStyle(weeklyStatsSheet, cell, cell, st)
		}
	}

	leadStart := 4 + len(rep.PersonRows) + 2
	_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", leadStart),
		fmt.Sprintf("접수→방문 평균일수(목표 %.0f일) · 접수→완료 평균일수(목표 %.0f일)",
			model.StatsVisitTargetDays, model.StatsCompleteTargetDays))
	_ = f.MergeCell(weeklyStatsSheet, fmt.Sprintf("A%d", leadStart), fmt.Sprintf("E%d", leadStart))

	leadHead := leadStart + 1
	leadHeaders := []string{"구분", "접수→방문", "신뢰도", "접수→완료", "신뢰도"}
	for i, h := range leadHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, leadHead)
		_ = f.SetCellValue(weeklyStatsSheet, cell, h)
	}
	for i, row := range rep.PersonRows {
		r := leadHead + 1 + i
		vals := []interface{}{
			row.Label,
			weeklyDaysText(row.VisitDisplay),
			weeklyGradeText(row.VisitDisplay),
			weeklyDaysText(row.CompleteDisplay),
			weeklyGradeText(row.CompleteDisplay),
		}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, r)
			_ = f.SetCellValue(weeklyStatsSheet, cell, v)
			st := plainSt
			if row.IsTeam {
				st = teamSt
			}
			if j == 1 {
				st = weeklyTargetStyle(row.VisitDisplay, false, model.StatsVisitTargetDays, okSt, badSt, naSt, st)
			}
			if j == 3 {
				st = weeklyTargetStyle(row.CompleteDisplay, false, model.StatsCompleteTargetDays, okSt, badSt, naSt, st)
			}
			_ = f.SetCellStyle(weeklyStatsSheet, cell, cell, st)
		}
	}

	planRow := leadHead + 1 + len(rep.PersonRows) + 2
	if rep.HasPlanning {
		_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", planRow),
			fmt.Sprintf("계획 수립률 %s %s (예정+미정 %d / 미완료 %d) · 목표 %.0f%%",
				weeklyRateText(rep.PlanDisplay), weeklyGradeText(rep.PlanDisplay),
				rep.PlanningPlanned, rep.PlanningOpen, model.StatsPlanTargetPct))
	} else {
		_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", planRow), "계획 수립률 — (미완료 업무 없음)")
	}
	_ = f.MergeCell(weeklyStatsSheet, fmt.Sprintf("A%d", planRow), fmt.Sprintf("J%d", planRow))

	adminLeadRow := planRow + 2
	waitShareRow := adminLeadRow + 1
	avgText, waitText := weeklyAdminLeadSheetTexts(rep.AdminLead)
	_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", adminLeadRow), avgText)
	_ = f.MergeCell(weeklyStatsSheet, fmt.Sprintf("A%d", adminLeadRow), fmt.Sprintf("J%d", adminLeadRow))
	_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", waitShareRow), waitText)
	_ = f.MergeCell(weeklyStatsSheet, fmt.Sprintf("A%d", waitShareRow), fmt.Sprintf("J%d", waitShareRow))

	mntRow := waitShareRow + 2
	_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("A%d", mntRow), "정기점검 월 진행")
	if rep.MntMonthLabel != "" {
		_ = f.SetCellValue(weeklyStatsSheet, fmt.Sprintf("B%d", mntRow), rep.MntMonthLabel)
	}
	mntHead := mntRow + 1
	mntHeaders := []string{"월목표", "접수(예약)", "방문(완료)", "누적", "남은"}
	for i, h := range mntHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, mntHead)
		_ = f.SetCellValue(weeklyStatsSheet, cell, h)
	}
	mntData := mntHead + 1
	mntVals := []interface{}{rep.MntQuota, rep.MntReceipt, rep.MntDone, rep.MntCumulative, rep.MntRemaining}
	for i, v := range mntVals {
		cell, _ := excelize.CoordinatesToCellName(i+1, mntData)
		_ = f.SetCellValue(weeklyStatsSheet, cell, v)
	}

	_ = f.SetColWidth(weeklyStatsSheet, "A", "A", 16)
	_ = f.SetColWidth(weeklyStatsSheet, "B", "B", 16)
	_ = f.SetColWidth(weeklyStatsSheet, "C", "C", 14)
	_ = f.SetColWidth(weeklyStatsSheet, "D", "J", 12)
	_ = f.SetRowHeight(weeklyStatsSheet, 1, 22)
	_ = f.SetPanes(weeklyStatsSheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 1, YSplit: 3,
		TopLeftCell: "B4", ActivePane: "bottomRight",
	})
	return nil
}

func writeWeeklyAllListSheet(f *excelize.File, rep model.WeeklyReport) error {
	if _, err := f.NewSheet(weeklyListSheet); err != nil {
		return err
	}
	_ = f.SetCellValue(weeklyListSheet, "A1", "[전체 리스트] · 과거 이관 데이터 제외")
	_ = f.MergeCell(weeklyListSheet, "A1", "J1")

	headers := []string{"구분", "업무번호", "발생일", "유형", "거래처", "제품·모델", "담당자", "내용", "결과·상태", "소요(분)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(weeklyListSheet, cell, h)
	}
	if st, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: "FFFFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FF1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    weeklyThinBorder(),
	}); err == nil {
		_ = f.SetCellStyle(weeklyListSheet, "A2", "J2", st)
	}

	for i, row := range rep.Events {
		r := i + 3
		vals := []interface{}{
			row.Kind, row.WorkNo, row.OccurDate, row.WorkType, row.Customer,
			row.Product, row.Assignee, row.Content, row.Result, row.Minutes,
		}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, r)
			_ = f.SetCellValue(weeklyListSheet, cell, v)
		}
	}
	last := len(rep.Events) + 2
	if last < 2 {
		last = 2
	}
	_ = f.AutoFilter(weeklyListSheet, fmt.Sprintf("A2:J%d", last), nil)
	_ = f.SetColWidth(weeklyListSheet, "A", "A", 10)
	_ = f.SetColWidth(weeklyListSheet, "B", "B", 14)
	_ = f.SetColWidth(weeklyListSheet, "C", "C", 12)
	_ = f.SetColWidth(weeklyListSheet, "D", "D", 12)
	_ = f.SetColWidth(weeklyListSheet, "E", "E", 18)
	_ = f.SetColWidth(weeklyListSheet, "F", "F", 18)
	_ = f.SetColWidth(weeklyListSheet, "G", "G", 12)
	_ = f.SetColWidth(weeklyListSheet, "H", "H", 32)
	_ = f.SetColWidth(weeklyListSheet, "I", "I", 14)
	_ = f.SetColWidth(weeklyListSheet, "J", "J", 10)
	_ = f.SetPanes(weeklyListSheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 2,
		TopLeftCell: "A3", ActivePane: "bottomLeft",
	})
	return nil
}

func weeklyThinBorder() []excelize.Border {
	c := "FFCBD5E1"
	return []excelize.Border{
		{Type: "left", Color: c, Style: 1},
		{Type: "right", Color: c, Style: 1},
		{Type: "top", Color: c, Style: 1},
		{Type: "bottom", Color: c, Style: 1},
	}
}

func weeklyAdminLeadSheetTexts(lead model.AdminLeadBreakdown) (avg string, wait string) {
	lead.FillDisplays()
	avg = "평균 리드타임 (행정/지원) —"
	if lead.LeadDisplay.ShowValue {
		avg = fmt.Sprintf("평균 리드타임 (행정/지원) %.1f일 %s",
			lead.LeadDisplay.Value, weeklyGradeText(lead.LeadDisplay))
	}
	wait = "외부 대기 비중 —"
	if lead.WaitShareDisplay.ShowValue {
		wait = fmt.Sprintf("외부 대기 비중 %s %s · 실작업 %d분 · 내부유휴 %d분",
			weeklyRateText(lead.WaitShareDisplay), weeklyGradeText(lead.WaitShareDisplay),
			lead.WorkMinutes, lead.IdleMinutes)
	}
	return avg, wait
}

func weeklyRateText(v model.StatsValue) string {
	if !v.ShowValue {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", v.Value)
}

func weeklyDaysText(v model.StatsValue) string {
	if !v.ShowValue {
		return "—"
	}
	return fmt.Sprintf("%.1f", v.Value)
}

func weeklyGradeText(v model.StatsValue) string {
	if v.GradeMark == "" {
		return ""
	}
	if v.Sample > 0 {
		return fmt.Sprintf("%s 표본 %d건", v.GradeMark, v.Sample)
	}
	return v.GradeMark
}

func weeklyDayAvgText(row model.WeeklyPersonRow) string {
	if !row.HasDayAvg {
		return "—"
	}
	return fmt.Sprintf("%.1f", row.DayAvg)
}

func weeklyDayMaxText(row model.WeeklyPersonRow) string {
	if row.DayMax <= 0 || row.DayMaxDate == "" {
		return fmt.Sprintf("%d", row.DayMax)
	}
	d := row.DayMaxDate
	if len(d) >= 10 {
		d = d[5:10]
	}
	return fmt.Sprintf("%d (%s)", row.DayMax, d)
}

func weeklyTargetStyle(v model.StatsValue, higherBetter bool, target float64, ok, bad, na, fallback int) int {
	if !v.ShowValue {
		return na
	}
	if !v.ShowTarget {
		return fallback
	}
	if higherBetter {
		if v.Value+1e-9 >= target {
			return ok
		}
		return bad
	}
	if v.Value <= target+1e-9 {
		return ok
	}
	return bad
}
