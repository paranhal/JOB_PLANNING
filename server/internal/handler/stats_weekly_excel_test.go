package handler

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func TestWriteWeeklyReportTwoSheetsNoDropdown(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rep := model.WeeklyReport{
		WeekFrom: "2026-08-10", WeekTo: "2026-08-16", Friday: "2026-08-14",
		WorkingDays: 5, HolidayYearMissing: true,
		PersonRows: []model.WeeklyPersonRow{
			{
				Label: model.StatsTeamLabel, IsTeam: true,
				Receipt: 23, Completed: 18, HasDayAvg: true, DayAvg: 3.6,
				DayMax: 7, DayMaxDate: "2026-08-12", CarryOut: 5, InProgress: 9, Unplanned: 3,
			},
			{
				Label: "최혜경", Receipt: 9, Completed: 8, HasDayAvg: true, DayAvg: 1.6,
				DayMax: 3, DayMaxDate: "2026-08-12", CarryOut: 1, InProgress: 3, Unplanned: 1,
			},
			{
				Label: model.StatsUnassignedLabel, Unassigned: true,
				Receipt: 1, Completed: 0, CarryOut: 1,
			},
		},
		HasPlanning: true, PlanningRate: 80, PlanningOpen: 10, PlanningPlanned: 8,
		PlanDisplay: model.StatsReliability(10, false, 80),
		MntQuota:    75, MntReceipt: 11, MntDone: 11, MntCumulative: 21, MntRemaining: 54,
		MntMonthLabel: "2026-08",
		AdminLead: func() model.AdminLeadBreakdown {
			b := model.AdminLeadBreakdown{Sample: 5, LeadDaysAvg: 1, LeadDaysSum: 5, WaitDaysSum: 4.5, WorkMinutes: 45}
			b.FillDisplays()
			return b
		}(),
		Events: []model.WeeklyEventRow{
			{Kind: model.WeeklyEventReceipt, WorkNo: "R2608-051", OccurDate: "2026-08-11", WorkType: "AS", Customer: "충남교육청", Assignee: "최혜경", Content: "대출반납기 오류", Result: "진행중"},
			{Kind: model.WeeklyEventAction, WorkNo: "R2608-051", OccurDate: "2026-08-12", WorkType: "AS", Customer: "충남교육청", Assignee: "최혜경", Content: "부품 교체", Result: "진행중", Minutes: 40},
			{Kind: model.WeeklyEventAdmin, WorkNo: "WT-001", OccurDate: "2026-08-15", WorkType: "행정", Content: "전자도서관 대기중 · 대기 대상 □□정보 박OO · 회신예정 2026-08-15", Result: "회신 대기"},
		},
	}
	if err := writeWeeklyStatsSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	if err := writeWeeklyAllListSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	_ = f.DeleteSheet("Sheet1")

	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("시트=%v want 통계·전체 리스트", sheets)
	}
	hasStats, hasList := false, false
	for _, s := range sheets {
		if s == "통계" {
			hasStats = true
		}
		if s == "전체 리스트" {
			hasList = true
		}
		if s == "업무리스트" || s == "진행중건" || s == "미계획건" || s == "_stats_src" {
			t.Fatalf("옛 시트 남음: %s", s)
		}
	}
	if !hasStats || !hasList {
		t.Fatalf("시트=%v", sheets)
	}

	title, err := f.GetCellValue("통계", "A1")
	if err != nil || !strings.Contains(title, "과거 이관 데이터 제외") {
		t.Fatalf("A1=%q err=%v", title, err)
	}
	if !strings.Contains(title, "워킹데이 5일") || !strings.Contains(title, "공휴일 미등록") {
		t.Fatalf("워킹데이/공휴일 주석 A1=%q", title)
	}
	a3, _ := f.GetCellValue("통계", "A3")
	if a3 != "구분" {
		t.Fatalf("헤더 A3=%q", a3)
	}
	team, _ := f.GetCellValue("통계", "A4")
	if team != model.StatsTeamLabel {
		t.Fatalf("첫 데이터 행=%q", team)
	}
	if dvs, _ := f.GetDataValidations("통계"); len(dvs) > 0 {
		t.Fatalf("담당자 드롭다운이 남아 있음: %+v", dvs)
	}

	listTitle, _ := f.GetCellValue("전체 리스트", "A1")
	if !strings.Contains(listTitle, "과거 이관 데이터 제외") {
		t.Fatalf("리스트 제목=%q", listTitle)
	}
	kind, _ := f.GetCellValue("전체 리스트", "A3")
	if kind != "접수" {
		t.Fatalf("리스트 A3=%q want 접수", kind)
	}
	no, _ := f.GetCellValue("전체 리스트", "B3")
	if no != "R2608-051" {
		t.Fatalf("업무번호=%q", no)
	}

	statsText := weeklySheetJoined(t, f, "통계")
	if !strings.Contains(statsText, "평균 리드타임") || !strings.Contains(statsText, "외부 대기") {
		t.Fatalf("통계 시트에 리드타임/외부대기 행 없음:\n%s", statsText)
	}
	if !strings.Contains(statsText, "1.0일") {
		t.Fatalf("평균 리드타임 값 없음:\n%s", statsText)
	}
	listText := weeklySheetJoined(t, f, "전체 리스트")
	if !strings.Contains(listText, "□□정보 박OO") || !strings.Contains(listText, "2026-08-15") {
		t.Fatalf("회신 대기 대기대상/예정일 없음:\n%s", listText)
	}
	if strings.Contains(listText, "미계획건") {
		t.Fatal("전체 리스트에 미계획건 시트 흔적")
	}
}

func weeklySheetJoined(t *testing.T, f *excelize.File, sheet string) string {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(strings.Join(row, " "))
		b.WriteByte('\n')
	}
	return b.String()
}
