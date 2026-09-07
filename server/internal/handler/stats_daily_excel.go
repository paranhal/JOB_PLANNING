package handler

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const (
	dailyMatrixSheet = "일자별"
	dailyDetailSheet = "담당자별 상세"
)

// ExportDailyAssigneeReport 담당자별 일일업무 엑셀. §16.4
func (h *StatsHandler) ExportDailyAssigneeReport(c echo.Context) error {
	now := time.Now()
	base, _, _ := metricsViewData(h.repo)
	p := parseLookbackReportPeriod(c, now, base)
	rep, err := h.repo.BuildDailyAssigneeReport(p.FromStr, p.ToExStr, p.FileDay)
	if err != nil {
		return err
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := writeDailyMatrixSheet(f, rep); err != nil {
		return err
	}
	if err := writeDailyDetailSheet(f, rep); err != nil {
		return err
	}
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	day := p.FileDay
	if day == "" {
		day = rep.FileDay
	}
	ascii := day + "_daily_assignee.xlsx"
	utf8 := day + "_담당자별일일업무.xlsx"
	return writeExcelFile(c, buf.Bytes(), ascii, utf8)
}

func writeDailyMatrixSheet(f *excelize.File, rep model.DailyAssigneeReport) error {
	idx, err := f.NewSheet(dailyMatrixSheet)
	if err != nil {
		return err
	}
	f.SetActiveSheet(idx)

	headSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: "FFFFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FF1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    weeklyThinBorder(),
	})
	offHeadSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: "FF374151"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFE5E7EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    weeklyThinBorder(),
	})
	plainSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    weeklyThinBorder(),
	})
	offSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FF6B7280"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFE5E7EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    weeklyThinBorder(),
	})
	zeroSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FFB91C1C"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFFECACA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    weeklyThinBorder(),
	})
	leaveSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Family: "맑은 고딕", Color: "FFF97316"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    weeklyThinBorder(),
	})
	totalSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFDBEAFE"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    weeklyThinBorder(),
	})
	nameSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕"},
		Border: weeklyThinBorder(),
	})

	title := fmt.Sprintf("[일자별]  %s ~ %s · 완료 건수 기준", rep.From, shortMD(rep.To))
	if rep.HolidayYearMissing {
		title += " · 공휴일 미등록"
	}
	title += " · 과거 이관 데이터 제외"
	_ = f.SetCellValue(dailyMatrixSheet, "A1", title)

	countEnd := writeDailyMatrixBlock(f, dailyMatrixSheet, 2, "담당자", rep, true,
		headSt, offHeadSt, plainSt, offSt, zeroSt, leaveSt, totalSt, nameSt)

	minTitleRow := countEnd + 2
	minTitle := fmt.Sprintf("[일자별]  %s ~ %s · 소요시간(분) 기준", rep.From, shortMD(rep.To))
	cell, _ := excelize.CoordinatesToCellName(1, minTitleRow)
	_ = f.SetCellValue(dailyMatrixSheet, cell, minTitle)
	writeDailyMatrixBlock(f, dailyMatrixSheet, minTitleRow+1, "담당자", rep, false,
		headSt, offHeadSt, plainSt, offSt, zeroSt, leaveSt, totalSt, nameSt)

	lastCol, _ := excelize.CoordinatesToCellName(1+len(rep.Days)+3, 1)
	_ = f.MergeCell(dailyMatrixSheet, "A1", lastCol)
	minTitleCell, _ := excelize.CoordinatesToCellName(1+len(rep.Days)+3, minTitleRow)
	_ = f.MergeCell(dailyMatrixSheet, cell, minTitleCell)

	_ = f.SetColWidth(dailyMatrixSheet, "A", "A", 14)
	if len(rep.Days) > 0 {
		fromCol, _ := excelize.ColumnNumberToName(2)
		toCol, _ := excelize.ColumnNumberToName(1 + len(rep.Days) + 3)
		_ = f.SetColWidth(dailyMatrixSheet, fromCol, toCol, 11)
	}
	_ = f.SetPanes(dailyMatrixSheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 1, YSplit: 2,
		TopLeftCell: "B3", ActivePane: "bottomRight",
	})
	return nil
}

func writeDailyMatrixBlock(f *excelize.File, sheet string, headerRow int, nameHeader string, rep model.DailyAssigneeReport, counts bool,
	headSt, offHeadSt, plainSt, offSt, zeroSt, leaveSt, totalSt, nameSt int) int {
	headers := []string{nameHeader}
	for _, d := range rep.Days {
		headers = append(headers, d.Label)
	}
	headers = append(headers, "합계", "일평균", "최대")
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		_ = f.SetCellValue(sheet, cell, h)
		st := headSt
		if i >= 1 && i <= len(rep.Days) && rep.Days[i-1].Off {
			st = offHeadSt
		}
		_ = f.SetCellStyle(sheet, cell, cell, st)
	}

	writePerson := func(row int, name string, vals []int, total int, avg float64, hasAvg bool, maxN int, isTeam bool, leaveMarks []string) {
		nameCell, _ := excelize.CoordinatesToCellName(1, row)
		_ = f.SetCellValue(sheet, nameCell, name)
		nst := nameSt
		if isTeam {
			nst = totalSt
		}
		_ = f.SetCellStyle(sheet, nameCell, nameCell, nst)
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+2, row)
			if !isTeam && i < len(leaveMarks) && leaveMarks[i] != "" {
				_ = f.SetCellValue(sheet, cell, leaveMarks[i])
				_ = f.SetCellStyle(sheet, cell, cell, leaveSt)
				continue
			}
			_ = f.SetCellValue(sheet, cell, v)
			st := plainSt
			if isTeam {
				st = totalSt
			}
			if i < len(rep.Days) && rep.Days[i].Off {
				st = offSt
			} else if i < len(rep.Days) && !rep.Days[i].Off && v == 0 {
				st = zeroSt
			}
			_ = f.SetCellStyle(sheet, cell, cell, st)
		}
		sumCol := 2 + len(rep.Days)
		avgCol := sumCol + 1
		maxCol := avgCol + 1
		sc, _ := excelize.CoordinatesToCellName(sumCol, row)
		ac, _ := excelize.CoordinatesToCellName(avgCol, row)
		mc, _ := excelize.CoordinatesToCellName(maxCol, row)
		_ = f.SetCellValue(sheet, sc, total)
		if hasAvg {
			_ = f.SetCellValue(sheet, ac, avg)
		} else {
			_ = f.SetCellValue(sheet, ac, "—")
		}
		_ = f.SetCellValue(sheet, mc, maxN)
		sumSt := plainSt
		if isTeam {
			sumSt = totalSt
		}
		_ = f.SetCellStyle(sheet, sc, mc, sumSt)
	}

	r := headerRow + 1
	for _, p := range rep.People {
		if counts {
			writePerson(r, p.Label, p.Counts, p.CountTotal, p.CountAvg, p.HasCountAvg, p.CountMax, false, p.LeaveMarks)
		} else {
			writePerson(r, p.Label, p.Minutes, p.MinuteTotal, p.MinuteAvg, p.HasMinuteAvg, p.MinuteMax, false, p.LeaveMarks)
		}
		r++
	}
	if counts {
		writePerson(r, "합계", rep.TeamCounts, rep.TeamCountTotal, rep.TeamCountAvg, rep.HasTeamCountAvg, rep.TeamCountMax, true, nil)
	} else {
		writePerson(r, "합계", rep.TeamMinutes, rep.TeamMinuteTotal, rep.TeamMinuteAvg, rep.HasTeamMinuteAvg, rep.TeamMinuteMax, true, nil)
	}
	return r
}

func writeDailyDetailSheet(f *excelize.File, rep model.DailyAssigneeReport) error {
	if _, err := f.NewSheet(dailyDetailSheet); err != nil {
		return err
	}
	_ = f.SetCellValue(dailyDetailSheet, "A1", "[담당자별 상세] · 과거 이관 데이터 제외")
	_ = f.MergeCell(dailyDetailSheet, "A1", "J1")

	headers := []string{"담당자", "날짜", "요일", "구분", "업무번호", "거래처", "내용", "상태", "소요(분)", "누적(분)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(dailyDetailSheet, cell, h)
	}
	headSt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: "FFFFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FF1E3A5F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    weeklyThinBorder(),
	})
	_ = f.SetCellStyle(dailyDetailSheet, "A2", "J2", headSt)

	plainSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Size: 10, Family: "맑은 고딕"},
		Border: weeklyThinBorder(),
	})
	totalSt, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Fill:   excelize.Fill{Type: "pattern", Color: []string{"FFF1F5F9"}, Pattern: 1},
		Border: weeklyThinBorder(),
	})
	breakSt, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Family: "맑은 고딕"},
		Border: []excelize.Border{
			{Type: "left", Color: "FFCBD5E1", Style: 1},
			{Type: "right", Color: "FFCBD5E1", Style: 1},
			{Type: "top", Color: "FF1E3A5F", Style: 2},
			{Type: "bottom", Color: "FFCBD5E1", Style: 1},
		},
	})

	for i, row := range rep.Details {
		r := i + 3
		vals := []interface{}{
			row.Assignee, row.Date, row.Weekday, row.Kind, row.WorkNo,
			row.Customer, row.Content, row.Status, row.Minutes, row.Cumulative,
		}
		st := plainSt
		if row.IsDayTotal {
			st = totalSt
			vals[3] = "합계"
			vals[7] = ""
		} else if row.NewPerson {
			st = breakSt
		}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, r)
			_ = f.SetCellValue(dailyDetailSheet, cell, v)
			_ = f.SetCellStyle(dailyDetailSheet, cell, cell, st)
		}
	}
	last := len(rep.Details) + 2
	if last < 2 {
		last = 2
	}
	_ = f.AutoFilter(dailyDetailSheet, fmt.Sprintf("A2:J%d", last), nil)
	_ = f.SetColWidth(dailyDetailSheet, "A", "A", 12)
	_ = f.SetColWidth(dailyDetailSheet, "B", "B", 12)
	_ = f.SetColWidth(dailyDetailSheet, "C", "C", 8)
	_ = f.SetColWidth(dailyDetailSheet, "D", "D", 12)
	_ = f.SetColWidth(dailyDetailSheet, "E", "E", 14)
	_ = f.SetColWidth(dailyDetailSheet, "F", "F", 18)
	_ = f.SetColWidth(dailyDetailSheet, "G", "G", 32)
	_ = f.SetColWidth(dailyDetailSheet, "H", "H", 10)
	_ = f.SetColWidth(dailyDetailSheet, "I", "J", 10)
	_ = f.SetPanes(dailyDetailSheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 2,
		TopLeftCell: "A3", ActivePane: "bottomLeft",
	})
	return nil
}

func shortMD(date string) string {
	if len(date) >= 10 {
		return date[5:10]
	}
	return date
}
