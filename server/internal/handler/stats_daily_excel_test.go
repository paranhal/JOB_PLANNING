package handler

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func TestWriteDailyAssigneeTwoSheets(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rep := model.DailyAssigneeReport{
		From: "2026-08-10", To: "2026-08-16", FileDay: "20260814", WorkingDays: 4,
		Days: []model.DailyAssigneeDay{
			{Date: "2026-08-10", Label: "08-10(월)"},
			{Date: "2026-08-11", Label: "08-11(화)"},
			{Date: "2026-08-12", Label: "08-12(수)", Off: true},
			{Date: "2026-08-13", Label: "08-13(목)"},
		},
		People: []model.DailyAssigneePerson{
			{
				Label:  "양기헌",
				Counts: []int{2, 1, 0, 0}, Minutes: []int{40, 20, 0, 0},
				CountTotal: 3, HasCountAvg: true, CountAvg: 0.8, CountMax: 2,
				MinuteTotal: 60, HasMinuteAvg: true, MinuteAvg: 15, MinuteMax: 40,
			},
		},
		TeamCounts: []int{2, 1, 0, 0}, TeamMinutes: []int{40, 20, 0, 0},
		TeamCountTotal: 3, HasTeamCountAvg: true, TeamCountAvg: 0.8, TeamCountMax: 2,
		TeamMinuteTotal: 60, HasTeamMinuteAvg: true, TeamMinuteAvg: 15, TeamMinuteMax: 40,
		Details: []model.DailyAssigneeDetailRow{
			{Assignee: "양기헌", Date: "2026-08-10", Weekday: "월", Kind: "AS", WorkNo: "R1", Customer: "도서관", Content: "게이트", Status: "완료", Minutes: 40, Cumulative: 40, NewPerson: true},
			{Assignee: "양기헌", Date: "2026-08-10", Weekday: "월", Kind: "합계", Minutes: 40, Cumulative: 40, IsDayTotal: true},
		},
	}
	if err := writeDailyMatrixSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	if err := writeDailyDetailSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	_ = f.DeleteSheet("Sheet1")
	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("시트=%v", sheets)
	}
	hasM, hasD := false, false
	for _, s := range sheets {
		if s == dailyMatrixSheet {
			hasM = true
		}
		if s == dailyDetailSheet {
			hasD = true
		}
	}
	if !hasM || !hasD {
		t.Fatalf("시트=%v want 일자별·담당자별 상세", sheets)
	}
	a1, _ := f.GetCellValue(dailyMatrixSheet, "A1")
	if !strings.Contains(a1, "완료 건수") {
		t.Fatalf("A1=%q", a1)
	}
	text := weeklySheetJoined(t, f, dailyMatrixSheet)
	if !strings.Contains(text, "소요시간(분)") {
		t.Fatalf("소요시간 표 없음:\n%s", text)
	}
	if !strings.Contains(text, "양기헌") || !strings.Contains(text, "합계") {
		t.Fatalf("담당자/합계 없음:\n%s", text)
	}
	detail := weeklySheetJoined(t, f, dailyDetailSheet)
	if !strings.Contains(detail, "R1") || !strings.Contains(detail, "합계") {
		t.Fatalf("상세:\n%s", detail)
	}
	h, _ := f.GetCellValue(dailyDetailSheet, "A2")
	if h != "담당자" {
		t.Fatalf("상세 헤더 A2=%q", h)
	}
	assertExcelizeRGB8(t, excelizeFileBytes(t, f))
}

func TestWriteDailyAssigneeLeaveMark(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rep := model.DailyAssigneeReport{
		From: "2026-08-10", To: "2026-08-13", FileDay: "20260813", WorkingDays: 3,
		Days: []model.DailyAssigneeDay{
			{Date: "2026-08-10", Label: "08-10(월)"},
			{Date: "2026-08-11", Label: "08-11(화)"},
			{Date: "2026-08-12", Label: "08-12(수)", Off: true},
			{Date: "2026-08-13", Label: "08-13(목)"},
		},
		People: []model.DailyAssigneePerson{
			{
				Label: "양기헌", Counts: []int{1, 1, 0, 0}, Minutes: []int{40, 20, 0, 0},
				CountTotal: 2, HasCountAvg: true, CountAvg: 0.7, CountMax: 1,
				MinuteTotal: 60, HasMinuteAvg: true, MinuteAvg: 20, MinuteMax: 40,
				LeaveMarks: []string{"", "", "", "연차"},
			},
		},
		TeamCounts: []int{1, 1, 0, 0}, TeamMinutes: []int{40, 20, 0, 0},
		TeamCountTotal: 2, HasTeamCountAvg: true, TeamCountAvg: 0.5, TeamCountMax: 1,
		TeamMinuteTotal: 60, HasTeamMinuteAvg: true, TeamMinuteAvg: 15, TeamMinuteMax: 40,
	}
	if err := writeDailyMatrixSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	got, _ := f.GetCellValue(dailyMatrixSheet, "E3")
	if got != "연차" {
		t.Fatalf("연차 칸=%q", got)
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "E3") != "FF4D4F" {
		t.Fatalf("연차 바탕=%s", cellFillRGB(t, f, dailyMatrixSheet, "E3"))
	}
}

func TestWriteDailyAssigneeHolidayLeaveFills(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rep := model.DailyAssigneeReport{
		From: "2026-10-01", To: "2026-10-07", FileDay: "20261007", WorkingDays: 4,
		HolidayYearMissing: false,
		Days: []model.DailyAssigneeDay{
			{Date: "2026-10-01", Label: "10-01(목)"},
			{Date: "2026-10-02", Label: "10-02(금)", Off: true},
			{Date: "2026-10-03", Label: "10-03(토)", Off: true},
			{Date: "2026-10-04", Label: "10-04(일)", Off: true},
			{Date: "2026-10-05", Label: "10-05(월)"},
			{Date: "2026-10-06", Label: "10-06(화)"},
			{Date: "2026-10-07", Label: "10-07(수)"},
		},
		People: []model.DailyAssigneePerson{
			{
				Label: "A", Counts: []int{2, 0, 0, 0, 0, 0, 0}, Minutes: []int{40, 0, 0, 0, 0, 0, 0},
				CountTotal: 2, HasCountAvg: true, CountAvg: 0.5, CountMax: 2,
				MinuteTotal: 40, HasMinuteAvg: true, MinuteAvg: 10, MinuteMax: 40,
				LeaveMarks: []string{"", "", "", "", "연차", "", ""},
			},
			{
				Label: "B", Counts: []int{0, 0, 0, 0, 1, 0, 1}, Minutes: []int{0, 0, 0, 0, 20, 0, 10},
				CountTotal: 2, HasCountAvg: true, CountAvg: 0.5, CountMax: 1,
				MinuteTotal: 30, HasMinuteAvg: true, MinuteAvg: 7.5, MinuteMax: 20,
				LeaveMarks: []string{"", "", "", "", "", "오전반차", ""},
			},
		},
		TeamCounts: []int{2, 0, 0, 0, 1, 0, 1}, TeamMinutes: []int{40, 0, 0, 0, 20, 0, 10},
		TeamCountTotal: 4, HasTeamCountAvg: true, TeamCountAvg: 1, TeamCountMax: 2,
		TeamMinuteTotal: 70, HasTeamMinuteAvg: true, TeamMinuteAvg: 17.5, TeamMinuteMax: 40,
		Details: []model.DailyAssigneeDetailRow{
			{Assignee: "A", Date: "2026-10-02", Weekday: "금", Kind: "AS", WorkNo: "R-off", NewPerson: true},
			{Assignee: "A", Date: "2026-10-02", Weekday: "금", Kind: "합계", IsDayTotal: true},
			{Assignee: "B", Date: "2026-10-05", Weekday: "월", Kind: "AS", WorkNo: "R-b", NewPerson: true},
		},
	}
	if err := writeDailyMatrixSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	if err := writeDailyDetailSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	_ = f.DeleteSheet("Sheet1")
	assertExcelizeRGB8(t, excelizeFileBytes(t, f))

	a1, _ := f.GetCellValue(dailyMatrixSheet, "A1")
	if !strings.Contains(a1, "빨강=토·일·공휴일·휴가") {
		t.Fatalf("범례 없음: %s", a1)
	}

	// 열: B=목 C=금공휴 D=토 E=일 F=월 G=화 H=수 / 행2 헤더, 행3 A, 행4 B, 행5 합계
	for _, cell := range []string{"C2", "C3", "C4", "C5", "D2", "D3", "E2", "E5"} {
		if got := cellFillRGB(t, f, dailyMatrixSheet, cell); got != "FF4D4F" {
			t.Fatalf("%s 바탕=%s want FF4D4F", cell, got)
		}
	}
	annual, _ := f.GetCellValue(dailyMatrixSheet, "F3")
	if annual != "연차" {
		t.Fatalf("A 연차=%q", annual)
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "F3") != "FF4D4F" {
		t.Fatalf("A 연차 바탕=%s", cellFillRGB(t, f, dailyMatrixSheet, "F3"))
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "F4") != "" {
		t.Fatalf("같은 날 B는 빨강이면 안 됨: %s", cellFillRGB(t, f, dailyMatrixSheet, "F4"))
	}
	half, _ := f.GetCellValue(dailyMatrixSheet, "G4")
	if half != "오전반차" {
		t.Fatalf("B 반차=%q", half)
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "G4") != "FF4D4F" {
		t.Fatalf("B 반차 바탕=%s", cellFillRGB(t, f, dailyMatrixSheet, "G4"))
	}
	zero, _ := f.GetCellValue(dailyMatrixSheet, "H3")
	if zero != "0" {
		t.Fatalf("평일 0건 값=%q", zero)
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "H3") != cellFillRGB(t, f, dailyMatrixSheet, "B3") {
		t.Fatalf("0건 바탕=%s 일반=%s", cellFillRGB(t, f, dailyMatrixSheet, "H3"), cellFillRGB(t, f, dailyMatrixSheet, "B3"))
	}
	sat, _ := f.GetCellValue(dailyMatrixSheet, "D3")
	if sat == "연차" {
		t.Fatal("주말 칸에 휴가 문구")
	}
	if cellFillRGB(t, f, dailyDetailSheet, "B3") != "FF4D4F" {
		t.Fatalf("상세 공휴일 날짜 칸=%s", cellFillRGB(t, f, dailyDetailSheet, "B3"))
	}

	rep.HolidayYearMissing = true
	f2 := excelize.NewFile()
	defer f2.Close()
	if err := writeDailyMatrixSheet(f2, rep); err != nil {
		t.Fatal(err)
	}
	assertExcelizeRGB8(t, excelizeFileBytes(t, f2))
}
