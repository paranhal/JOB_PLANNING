package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestBuildRegisterAssigneeColumnsSamePersonSameLeft(t *testing.T) {
	dateCol := RegisterColumn{Label: "화", Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "A", Title: "07:30", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "07:30", EndTime: "08:00", DurationMin: 30},
		{TaskID: "B", Title: "08:00", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "08:00", EndTime: "08:30", DurationMin: 30},
		{TaskID: "C", Title: "09:00", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "D", Title: "양", Assignee: "양기헌", WorkDate: "2026-08-18", StartTime: "08:45", EndTime: "09:15", DurationMin: 30},
	}
	order := []string{"양기헌", "최혜영", "태자운"}
	cols := buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, order, "")
	if len(cols) != 2 {
		t.Fatalf("열=%d want 2 (최혜영·양기헌)", len(cols))
	}
	if cols[0].Assignee != "양기헌" || cols[1].Assignee != "최혜영" {
		t.Fatalf("열 순서=%s,%s want 양기헌,최혜영", cols[0].Assignee, cols[1].Assignee)
	}
	if cols[1].Count != 3 {
		t.Fatalf("최혜영 건수=%d", cols[1].Count)
	}
	lefts := map[float64]bool{}
	for _, b := range cols[1].Blocks {
		lefts[b.LeftPct] = true
		if b.WidthPct < 90 {
			t.Fatalf("혼자인 열은 전폭이어야 함 width=%.1f task=%s", b.WidthPct, b.Card.TaskID)
		}
		if strings.Contains(string(b.Style), "max-width") {
			t.Fatalf("일일 열에 max-width 가 있으면 안 됨: %s", b.Style)
		}
		if b.OverlapWarn {
			t.Fatalf("겹치지 않는데 ⚠: %s", b.Card.TaskID)
		}
	}
	if len(lefts) != 1 {
		t.Fatalf("최혜영 카드 가로 위치가 흩어짐 lefts=%v", lefts)
	}
}

func TestBuildRegisterAssigneeColumnsUnassignedAndFilter(t *testing.T) {
	dateCol := RegisterColumn{Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "A", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "U", Assignee: "", WorkDate: "2026-08-18", StartTime: "10:00", EndTime: "10:30", DurationMin: 30},
	}
	cols := buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, []string{"최혜영"}, "")
	if len(cols) != 2 || !cols[1].Unassigned || cols[1].Label != registerUnassignedLabel {
		t.Fatalf("미배정 열이 맨 오른쪽이어야 함: %+v", labelsOfCols(cols))
	}

	one := buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, []string{"최혜영"}, "최혜영")
	if len(one) != 1 || one[0].Assignee != "최혜영" {
		t.Fatalf("필터 1열=%+v", labelsOfCols(one))
	}

	empty := buildRegisterAssigneeColumns(dateCol, nil, taskCardFromWork, []string{"최혜영"}, "")
	if len(empty) != 0 {
		t.Fatalf("0건이면 열 없음: %d", len(empty))
	}

	filteredEmpty := buildRegisterAssigneeColumns(dateCol, nil, taskCardFromWork, []string{"이해진"}, "이해진")
	if len(filteredEmpty) != 1 || filteredEmpty[0].Assignee != "이해진" || filteredEmpty[0].Count != 0 {
		t.Fatalf("필터한 사람은 0건이어도 1열: %+v", filteredEmpty)
	}

	forced := buildRegisterAssigneeColumnsAlways(dateCol, nil, taskCardFromWork, []string{"최혜영", "태자운"}, "", nil, []string{"최혜영", "태자운"})
	if len(forced) != 2 {
		t.Fatalf("영업 빈 열=%d want 2 labels=%v", len(forced), labelsOfCols(forced))
	}
}

func TestBuildRegisterAssigneeColumnsOverlapWarn(t *testing.T) {
	dateCol := RegisterColumn{Date: "2026-08-10", From: "2026-08-10", To: "2026-08-10"}
	placed := []model.WorkTask{
		{TaskID: "A", Assignee: "양기헌", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "B", Assignee: "양기헌", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
	}
	cols := buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, []string{"양기헌"}, "")
	if len(cols) != 1 || len(cols[0].Blocks) != 2 {
		t.Fatalf("cols=%d blocks=%d", len(cols), len(cols[0].Blocks))
	}
	if cols[0].Blocks[0].Lane == cols[0].Blocks[1].Lane {
		t.Fatal("같은 담당자 겹침은 열 안에서 좌우로 쪼개야 함")
	}
	for _, b := range cols[0].Blocks {
		if !b.OverlapWarn {
			t.Fatalf("겹침 표시 없음 task=%s", b.Card.TaskID)
		}
		if b.WidthPct > 55 {
			t.Fatalf("2등분인데 너무 넓음 width=%.1f", b.WidthPct)
		}
	}
}

func TestBuildRegisterAssigneeColumnsHeaderStats(t *testing.T) {
	dateCol := RegisterColumn{Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "A", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "10:00", DurationMin: 60},
		{TaskID: "B", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "10:00", EndTime: "11:00", DurationMin: 60},
	}
	cols := buildRegisterAssigneeColumns(dateCol, placed, taskCardFromWork, []string{"최혜영"}, "")
	if cols[0].Sub != "2건 · 2시간" {
		t.Fatalf("머리글=%q", cols[0].Sub)
	}
}

func TestAssigneeOverlapMessage(t *testing.T) {
	got := assigneeOverlapMessage("최혜영", "09:00", "09:30")
	if got != "최혜영의 09:00~09:30에 다른 업무가 있습니다" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(assigneeOverlapMessage("", "10:00:00", "10:30:00"), "미배정의 10:00~10:30") {
		t.Fatal("미배정 시각 문구")
	}
}

func TestExtraAssigneesForDay(t *testing.T) {
	users := []model.User{{FullName: "이해진"}, {FullName: "최혜영"}}
	cols := []RegisterDayColumn{{Assignee: "최혜영"}}
	if got := extraAssigneesForDay(regViewWeek, "", users, cols); got != nil {
		t.Fatalf("주간은 ExtraAssignees 없음: %v", got)
	}
	if got := extraAssigneesForDay(regViewDay, "최혜영", users, cols); got != nil {
		t.Fatalf("필터 켜면 ＋담당자 목록 없음: %v", got)
	}
	got := extraAssigneesForDay(regViewDay, "", users, cols)
	if len(got) != 1 || got[0] != "이해진" {
		t.Fatalf("빈 열 후보=%v want [이해진]", got)
	}
}

func TestBuildRegisterAssigneeColumnsSupportCards(t *testing.T) {
	dateCol := RegisterColumn{Date: "2026-08-18", From: "2026-08-18", To: "2026-08-18"}
	placed := []model.WorkTask{
		{TaskID: "T1", Title: "CRM 자산정비", Assignee: "최혜영", WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "11:00", DurationMin: 120},
	}
	members := map[string][]model.WorkTaskMember{
		"T1": {
			{TaskID: "T1", Assignee: "최혜영", Role: model.WBMemberOwner},
			{TaskID: "T1", Assignee: "양기헌", Role: model.WBMemberSupport, DurationMin: 120},
			{TaskID: "T1", Assignee: "태자운", Role: model.WBMemberSupport, DurationMin: 60},
		},
	}
	cols := buildRegisterAssigneeColumnsMembers(dateCol, placed, taskCardFromWork, []string{"최혜영", "양기헌", "태자운"}, "", members)
	if len(cols) != 3 {
		t.Fatalf("열=%d want 3 %+v", len(cols), labelsOfCols(cols))
	}
	var owner, support int
	for _, c := range cols {
		if len(c.Blocks) != 1 {
			t.Fatalf("%s blocks=%d", c.Assignee, len(c.Blocks))
		}
		b := c.Blocks[0]
		if b.ExtraCount != 2 {
			t.Fatalf("%s extra=%d want 2", c.Assignee, b.ExtraCount)
		}
		if c.Assignee == "최혜영" {
			if b.IsSupport {
				t.Fatal("주담당 카드가 지원이면 안 됨")
			}
			owner++
		} else {
			if !b.IsSupport {
				t.Fatalf("%s 지원 카드여야 함", c.Assignee)
			}
			support++
		}
	}
	if owner != 1 || support != 2 {
		t.Fatalf("owner=%d support=%d", owner, support)
	}
}

func TestFilterTasksByParticipantsIncludesSupport(t *testing.T) {
	tasks := []model.WorkTask{
		{TaskID: "T1", Assignee: "최혜영"},
		{TaskID: "T2", Assignee: "양기헌"},
	}
	members := map[string][]model.WorkTaskMember{
		"T1": {{Assignee: "최혜영", Role: model.WBMemberOwner}, {Assignee: "이해진", Role: model.WBMemberSupport}},
	}
	got := filterTasksByParticipants(tasks, members, "이해진")
	if len(got) != 1 || got[0].TaskID != "T1" {
		t.Fatalf("지원 필터: %+v", got)
	}
}

func labelsOfCols(cols []RegisterDayColumn) []string {
	var s []string
	for _, c := range cols {
		s = append(s, c.Label)
	}
	return s
}
