package handler

import (
	"fmt"
	"sort"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func excelEntryFontColor(category string) string {
	switch category {
	case "fixed":
		return "FFFF0000"
	case "office":
		return "FF0070C0"
	default:
		return "FF00B050"
	}
}

// BuildMaintenanceExcelWorkbook 기획서 §17.11 월별 달력 시트(1월~12월) xlsx 생성.
func BuildMaintenanceExcelWorkbook(year int, visits []model.MaintenanceVisit) (*excelize.File, error) {
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
	for m := 1; m <= 12; m++ {
		sheet := fmt.Sprintf("%d월", m)
		if err := fillMaintenanceMonthSheet(f, sheet, year, m, byDate, loc); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func fillMaintenanceMonthSheet(f *excelize.File, sheet string, year, month int, byDate map[string][]model.MaintenanceVisit, loc *time.Location) error {
	lastDay := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, loc).Day()

	titleStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 18, Color: "FF00B050", Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return err
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFE2EFDA"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	outStyle, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFF2F2F2"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	weekendHdr, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFDDDDDD"}, Pattern: 1},
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
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFEEEEEE"}, Pattern: 1},
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

			if len(list) == 0 {
				runs := []excelize.RichTextRun{
					{Text: fmt.Sprintf("%d", dayNum), Font: &excelize.Font{Bold: true, Size: 11, Color: "FF333333", Family: "맑은 고딕"}},
				}
				_ = f.SetCellRichText(sheet, cell, runs)
				if isWeekendCol {
					_ = f.SetCellStyle(sheet, cell, cell, weekendWrap)
				} else {
					_ = f.SetCellStyle(sheet, cell, cell, wrapTop)
				}
				continue
			}

			runs := []excelize.RichTextRun{
				{Text: fmt.Sprintf("%d\n", dayNum), Font: &excelize.Font{Bold: true, Size: 11, Color: "FF000000", Family: "맑은 고딕"}},
			}
			for _, v := range list {
				line := v.ShortName
				if line == "" {
					line = v.OrgName
				}
				runs = append(runs, excelize.RichTextRun{
					Text: line + "\n",
					Font: &excelize.Font{Size: 10, Color: excelEntryFontColor(v.EntryCategory), Family: "맑은 고딕"},
				})
			}
			_ = f.SetCellRichText(sheet, cell, runs)
			if isWeekendCol {
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
