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
}
