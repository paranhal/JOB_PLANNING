package handler

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/repository"
)

var companyWeeklySheetHeads = []string{
	"월", "주차", "시작일", "종료일", "팀", "부문/실", "담당자 (필요시)", "No",
	"프로젝트 이름 or 업무활동 (선택만 가능)", "고객(사용자)", "추진내역", "추진계획",
	"지원 필요사항", "주요 의사결정 (주간업무보고 시간 의사결정 내역 기록)",
}

type companyWeeklySheetRow struct {
	Month    int
	WeekN    int
	Start    string
	End      string
	Team     string
	Division string
	Assignee string
	No       string
	Name     string
	Customer string
	Done     string
	Plan     string
	Need     string
	Decision string
}

func weeklyReportToSheetRows(v weeklyReportView) []companyWeeklySheetRow {
	wi := v.WeekInfo
	start := companyWeekDayLabel(wi.Monday)
	end := companyWeekDayLabel(wi.Sunday)
	var out []companyWeeklySheetRow
	for _, c := range v.Cards {
		if !c.HasContent {
			continue
		}
		name := c.Title
		if c.Kind == "activity" {
			name = "## " + name
		}
		out = append(out, companyWeeklySheetRow{
			Month: wi.Month, WeekN: wi.WeekN, Start: start, End: end,
			Team: v.OrgShort, Assignee: c.Assignee, Name: excelPlain(name),
			Customer: excelPlain(c.Customer),
			Done:     excelPlain(bulletLines(c.Done)),
			Plan:     excelPlain(bulletLines(c.Plan)),
			Need:     excelPlain(c.Need),
			Decision: excelPlain(c.Decision),
		})
	}
	return out
}

func bulletLines(lines []repository.CompanyWeeklyLine) string {
	var parts []string
	for _, l := range lines {
		t := strings.TrimSpace(l.Text)
		if t == "" {
			continue
		}
		parts = append(parts, "* "+t)
	}
	return strings.Join(parts, "\n")
}

func excelPlain(s string) string {
	return s
}

func companyWeekDayLabel(t interface{ Format(string) string }) string {
	type tm interface {
		Weekday() interface{}
	}
	return formatWeekDay(t)
}

func formatWeekDay(t interface{ Format(string) string }) string {
	s := t.Format("06.01.02")
	wd := t.Format("Mon")
	kor := map[string]string{"Mon": "월", "Tue": "화", "Wed": "수", "Thu": "목", "Fri": "금", "Sat": "토", "Sun": "일"}
	return s + "(" + kor[wd] + ")"
}

func buildCompanyWeeklyTeamWorkbook(views []weeklyReportView) ([]byte, error) {
	f := excelize.NewFile()
	first := true
	for _, v := range views {
		name := v.OrgShort
		if name == "" {
			name = v.OrgID
		}
		name = excelSheetName(name)
		if first {
			f.SetSheetName(f.GetSheetName(0), name)
			first = false
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
		if err := writeCompanyWeeklySheet(f, name, weeklyReportToSheetRows(v)); err != nil {
			return nil, err
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func excelSheetName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(":", "", "\\", "", "/", "", "?", "", "*", "", "[", "", "]", "").Replace(s)
	if s == "" {
		s = "팀"
	}
	if utf8.RuneCountInString(s) > 31 {
		r := []rune(s)
		s = string(r[:31])
	}
	return s
}

func writeCompanyWeeklySheet(f *excelize.File, sheet string, rows []companyWeeklySheetRow) error {
	headStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E8F1EC"}, Pattern: 1},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"},
	})
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	for i, h := range companyWeeklySheetHeads {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(sheet, cell, h)
	}
	_ = f.SetCellStyle(sheet, "A1", "N1", headStyle)
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	widths := []float64{6, 6, 14, 14, 14, 10, 12, 6, 42, 18, 48, 48, 28, 28}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
	for r, row := range rows {
		ri := r + 2
		_ = f.SetCellInt(sheet, fmt.Sprintf("A%d", ri), row.Month)
		_ = f.SetCellInt(sheet, fmt.Sprintf("B%d", ri), row.WeekN)
		vals := []string{row.Start, row.End, row.Team, row.Division, row.Assignee, row.No, row.Name, row.Customer, row.Done, row.Plan, row.Need, row.Decision}
		for i, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+3, ri)
			_ = f.SetCellStr(sheet, cell, val)
		}
		_ = f.SetCellStyle(sheet, fmt.Sprintf("C%d", ri), fmt.Sprintf("N%d", ri), wrapStyle)
	}
	return nil
}

func weeklyReportExportNames(v weeklyReportView, all bool) (ascii, utf8 string) {
	wi := v.WeekInfo
	team := v.OrgShort
	if all {
		team = "전사"
	}
	utf8 = fmt.Sprintf("전사주간업무보고_%s_%02d%02d_%d주차.xlsx", team, wi.Year%100, wi.Month, wi.WeekN)
	ascii = fmt.Sprintf("weekly_%02d%02d_w%d.xlsx", wi.Year%100, wi.Month, wi.WeekN)
	return ascii, utf8
}
