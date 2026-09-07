package handler

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func excelEntryFontColor(category string) string {
	switch category {
	case "fixed":
		return "FF0000"
	case "office":
		return "0070C0"
	default:
		return "00B050"
	}
}

// BuildMaintenanceExcelWorkbook 기획서 §23.7 월별 달력 시트(1월~12월) xlsx 생성.
// offDates·holidayNames가 있으면 휴일 칸을 회색으로 칠하고 공휴일명을 적는다. §23.13.6
func BuildMaintenanceExcelWorkbook(year int, visits []model.MaintenanceVisit, offDates map[string]bool, holidayNames ...map[string]string) (*excelize.File, error) {
	byDate := make(map[string][]model.MaintenanceVisit)
	for _, v := range visits {
		byDate[v.VisitDate] = append(byDate[v.VisitDate], v)
	}
	for k := range byDate {
		sort.Slice(byDate[k], func(i, j int) bool {
			return byDate[k][i].ShortName < byDate[k][j].ShortName
		})
	}

	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "1월"); err != nil {
		return nil, err
	}
	for m := 2; m <= 12; m++ {
		if _, err := f.NewSheet(fmt.Sprintf("%d월", m)); err != nil {
			return nil, err
		}
	}

	loc := time.Local
	var off map[string]bool
	if offDates != nil {
		off = offDates
	}
	var names map[string]string
	if len(holidayNames) > 0 {
		names = holidayNames[0]
	}
	for m := 1; m <= 12; m++ {
		sheet := fmt.Sprintf("%d월", m)
		if err := fillMaintenanceMonthSheet(f, sheet, year, m, byDate, loc, off, names); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func fillMaintenanceMonthSheet(f *excelize.File, sheet string, year, month int, byDate map[string][]model.MaintenanceVisit, loc *time.Location, off map[string]bool, names map[string]string) error {
	lastDay := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, loc).Day()

	titleStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 18, Color: "00B050", Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return err
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E2EFDA"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	outStyle, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F2F2F2"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	weekendHdr, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"DDDDDD"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	wrapTop, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "left"},
	})
	if err != nil {
		return err
	}
	weekendWrap, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEEEEE"}, Pattern: 1},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Horizontal: "left"},
	})
	if err != nil {
		return err
	}

	_ = f.MergeCell(sheet, "A1", "G1")
	_ = f.SetCellStr(sheet, "A1", fmt.Sprintf("%d년 %d월", year, month))
	_ = f.SetCellStyle(sheet, "A1", "G1", titleStyle)
	_ = f.SetRowHeight(sheet, 1, 36)

	headers := []string{"일요일", "월요일", "화요일", "수요일", "목요일", "금요일", "토요일"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellStr(sheet, cell, h)
		if i == 0 || i == 6 {
			_ = f.SetCellStyle(sheet, cell, cell, weekendHdr)
		} else {
			_ = f.SetCellStyle(sheet, cell, cell, hdrStyle)
		}
	}
	_ = f.SetRowHeight(sheet, 2, 22)

	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	offset := int(first.Weekday())

	for week := 0; week < 6; week++ {
		row := 3 + week
		_ = f.SetRowHeight(sheet, row, 72)
		for col := 0; col < 7; col++ {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			dayNum := week*7 + col - offset + 1
			isWeekendCol := col == 0 || col == 6

			if dayNum < 1 || dayNum > lastDay {
				_ = f.SetCellStyle(sheet, cell, cell, outStyle)
				continue
			}

			ds := fmt.Sprintf("%04d-%02d-%02d", year, month, dayNum)
			list := byDate[ds]
			isOff := isWeekendCol || off[ds]
			hName := ""
			if names != nil {
				hName = strings.TrimSpace(names[ds])
			}

			if len(list) == 0 {
				dayText := fmt.Sprintf("%d", dayNum)
				if hName != "" {
					dayText = fmt.Sprintf("%d\n%s", dayNum, hName)
				}
				runs := []excelize.RichTextRun{
					{Text: dayText, Font: &excelize.Font{Bold: true, Size: 11, Color: "333333", Family: "맑은 고딕"}},
				}
				_ = f.SetCellRichText(sheet, cell, runs)
				if isOff {
					_ = f.SetCellStyle(sheet, cell, cell, weekendWrap)
				} else {
					_ = f.SetCellStyle(sheet, cell, cell, wrapTop)
				}
				continue
			}

			head := fmt.Sprintf("%d\n", dayNum)
			if hName != "" {
				head = fmt.Sprintf("%d %s\n", dayNum, hName)
			}
			runs := []excelize.RichTextRun{
				{Text: head, Font: &excelize.Font{Bold: true, Size: 11, Color: "000000", Family: "맑은 고딕"}},
			}
			for _, v := range list {
				line := visitLabel(v)
				color := excelProductFontColor(v.ProductType)
				if color == "333333" {
					// 점검 대상이 없으면 예전 엑셀 유형(고정/사무소) 색을 쓴다.
					color = excelEntryFontColor(v.EntryCategory)
				}
				runs = append(runs, excelize.RichTextRun{
					Text: line + "\n",
					Font: &excelize.Font{Size: 10, Color: color, Family: "맑은 고딕"},
				})
			}
			_ = f.SetCellRichText(sheet, cell, runs)
			if isOff {
				_ = f.SetCellStyle(sheet, cell, cell, weekendWrap)
			} else {
				_ = f.SetCellStyle(sheet, cell, cell, wrapTop)
			}
		}
	}

	cols := []string{"A", "B", "C", "D", "E", "F", "G"}
	for _, colName := range cols {
		_ = f.SetColWidth(sheet, colName, colName, 16)
	}
	return nil
}

// BuildSiteConfigExcelWorkbook 점검 사이트(고객 사이트) 설정 목록 xlsx
func BuildSiteConfigExcelWorkbook(items []model.MaintenanceSiteConfig) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "Sites"); err != nil {
		return nil, err
	}
	sheet = "Sites"

	headers := []interface{}{
		"고객ID", "기관명", "표시명", "지역", "점검주기",
		"KLAS", "RFID", "엑셀유형", "고정규칙",
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true, Family: "맑은 고딕", Size: 11,
			Color: "FFFFFF",
		},
		Fill: excelize.Fill{
			Type: "pattern", Color: []string{"2F5496"}, Pattern: 1,
		},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	if err := f.SetSheetRow(sheet, "A1", &headers); err != nil {
		return nil, err
	}
	if err := f.SetCellStyle(sheet, "A1", "I1", hdrStyle); err != nil {
		return nil, err
	}
	if err := f.SetRowHeight(sheet, 1, 30); err != nil {
		return nil, err
	}
	for r, it := range items {
		row := []interface{}{
			it.CustomerID,
			it.OrgName,
			it.ShortName,
			it.Region,
			siteInspectionCycleLabel(it.InspectionCycle),
			boolMark(it.HasKlas),
			boolMark(it.HasRfid),
			siteEntryCategoryLabel(it.EntryCategory),
			siteFixedRuleLabel(it.FixedRule),
		}
		cell := fmt.Sprintf("A%d", r+2)
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return nil, err
		}
	}
	widths := []float64{22, 28, 16, 12, 12, 8, 8, 14, 22}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
	return f, nil
}

func boolMark(v bool) string {
	if v {
		return "○"
	}
	return ""
}

func siteInspectionCycleLabel(s string) string {
	m := map[string]string{
		"monthly": "월", "odd_bimonthly": "홀수격월", "even_bimonthly": "짝수격월",
		"quarterly": "분기", "semi": "반기", "yearly": "년1회",
	}
	if l, ok := m[s]; ok {
		return l
	}
	if s == "" {
		return "월"
	}
	return s
}

func siteEntryCategoryLabel(s string) string {
	switch s {
	case "fixed":
		return "고정·지정일"
	case "office":
		return "사무소 등"
	default:
		return "일반"
	}
}

func siteFixedRuleLabel(s string) string {
	days, last := model.ParseFixedRule(s)
	if last && len(days) == 0 {
		return "매월 마지막 월요일"
	}
	if len(days) == 0 && !last {
		return ""
	}
	return s
}
