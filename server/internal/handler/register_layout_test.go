package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestAssignAssigneeLanesSideBySide(t *testing.T) {
	evs := []layoutEvent{
		{assignee: "최혜영", startMin: 9 * 60, endMin: 9*60 + 90, lane: -1},
		{assignee: "양기헌", startMin: 9*60 + 15, endMin: 9*60 + 45, lane: -1},
		{assignee: "최혜영", startMin: 11 * 60, endMin: 11*60 + 30, lane: -1},
	}
	assignAssigneeLanes(evs)
	if evs[0].lanes < 2 || evs[1].lanes < 2 {
		t.Fatalf("다른 담당자 겹침은 2레인 이상: lanes=%d,%d", evs[0].lanes, evs[1].lanes)
	}
	if evs[0].lane == evs[1].lane {
		t.Fatalf("겹치는 다른 담당자는 다른 레인: %d==%d", evs[0].lane, evs[1].lane)
	}
	if evs[2].lanes != 1 {
		t.Fatalf("단독 업무는 전폭: lanes=%d", evs[2].lanes)
	}
}

func TestAssignAssigneeLanesSameAssigneeSideBySide(t *testing.T) {
	// 같은 담당자라도 시간이 겹치면 나란히 표시(업무처리현황에서 숨김 방지)
	evs := []layoutEvent{
		{assignee: "양기헌", startMin: 9 * 60, endMin: 9*60 + 30, lane: -1, card: model.WBCard{TaskID: "A"}},
		{assignee: "양기헌", startMin: 9 * 60, endMin: 9*60 + 30, lane: -1, card: model.WBCard{TaskID: "B"}},
	}
	assignAssigneeLanes(evs)
	if evs[0].lanes < 2 || evs[1].lanes < 2 {
		t.Fatalf("같은 담당자 겹침도 2레인 이상: %d,%d", evs[0].lanes, evs[1].lanes)
	}
	if evs[0].lane == evs[1].lane {
		t.Fatalf("겹치는 같은 담당자도 다른 레인: %d==%d", evs[0].lane, evs[1].lane)
	}
}

func TestBuildRegisterDayColumnsSameAssigneeAllVisible(t *testing.T) {
	cols := []RegisterColumn{{
		Label: "월", Date: "2026-08-10", From: "2026-08-10", To: "2026-08-10",
	}}
	placed := []model.WorkTask{
		{TaskID: "WT-121", Title: "AS1", Assignee: "양기헌", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "WT-122", Title: "AS2", Assignee: "양기헌", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "WT-050", Title: "점검", Assignee: "최혜영", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "WT-124", Title: "AS3", Assignee: "최혜영", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
		{TaskID: "WT-110", Title: "행정", Assignee: "태자운", WorkDate: "2026-08-10", StartTime: "09:00", EndTime: "09:30", DurationMin: 30},
	}
	days := buildRegisterDayColumns(cols, placed, taskCardFromWork)
	if len(days[0].Blocks) != 5 {
		t.Fatalf("blocks=%d want 5", len(days[0].Blocks))
	}
	lanes := map[int]bool{}
	for _, b := range days[0].Blocks {
		lanes[b.Lane] = true
		if b.Lanes < 5 {
			t.Fatalf("task %s lanes=%d want >=5", b.Card.TaskID, b.Lanes)
		}
	}
	if len(lanes) != 5 {
		t.Fatalf("distinct lanes=%d want 5", len(lanes))
	}
}

func TestAssignAssigneeLanesNoTransitiveAllDay(t *testing.T) {
	// 종일 업무가 있어도, 겹치지 않는 오후 다른 담당자는 전폭
	evs := []layoutEvent{
		{assignee: "태자운", startMin: 9 * 60, endMin: 18 * 60, lane: -1},
		{assignee: "최혜영", startMin: 9 * 60, endMin: 9*60 + 30, lane: -1},
		{assignee: "양기헌", startMin: 15 * 60, endMin: 15*60 + 30, lane: -1}, // 태자운과만 겹침
	}
	assignAssigneeLanes(evs)
	if evs[1].lanes < 2 {
		t.Fatalf("오전 겹침 분할: %d", evs[1].lanes)
	}
	// 양기헌은 태자운과만 겹침 → 2레인, 최혜영과 전이로 3레인이 되면 안 됨
	if evs[2].lanes > 2 {
		t.Fatalf("전이적 종일 묶음 금지: lanes=%d", evs[2].lanes)
	}
}

func TestBuildRegisterDayColumnsPosition(t *testing.T) {
	cols := []RegisterColumn{{
		Label: "화", Date: "2026-08-04", From: "2026-08-04", To: "2026-08-04",
	}}
	placed := []model.WorkTask{
		{TaskID: "A", Title: "A", Assignee: "최혜영", WorkDate: "2026-08-04", StartTime: "09:00", EndTime: "10:00", DurationMin: 60},
		{TaskID: "B", Title: "B", Assignee: "양기헌", WorkDate: "2026-08-04", StartTime: "09:30", EndTime: "10:00", DurationMin: 30},
		{TaskID: "C", Title: "C", Assignee: "태자운", WorkDate: "2026-08-04", StartTime: "14:00", EndTime: "14:30", DurationMin: 30},
	}
	days := buildRegisterDayColumns(cols, placed, taskCardFromWork)
	if len(days[0].Blocks) != 3 {
		t.Fatalf("blocks=%d", len(days[0].Blocks))
	}
	var topA, topC float64
	for _, b := range days[0].Blocks {
		if b.Card.TaskID == "A" {
			topA = b.TopRem
			if !strings.Contains(string(b.Style), "top:") {
				t.Fatal("style missing top")
			}
			if b.TopRem < 1 {
				t.Fatalf("09:00 should not be at top(07:00): topRem=%v", b.TopRem)
			}
		}
		if b.Card.TaskID == "C" {
			topC = b.TopRem
		}
	}
	if topC <= topA {
		t.Fatalf("14:00(%v) should be below 09:00(%v)", topC, topA)
	}
}

func TestBuildAssigneeLegendIncludesUsers(t *testing.T) {
	users := []model.User{
		{FullName: "양기헌"}, {FullName: "이해진"}, {FullName: "최혜영"},
		{FullName: "최혜경"}, {FullName: "태자운"},
	}
	leg := buildAssigneeLegend(nil, users)
	if len(leg) != 5 {
		t.Fatalf("legend=%d %+v", len(leg), leg)
	}
	names := map[string]bool{}
	for _, l := range leg {
		names[l.Name] = true
		if l.Border == "" {
			t.Fatalf("no color for %s", l.Name)
		}
	}
	for _, n := range []string{"최혜경", "태자운", "양기헌"} {
		if !names[n] {
			t.Fatalf("missing %s", n)
		}
	}
}

func TestParseHHMMSeconds(t *testing.T) {
	m, ok := parseHHMMToMin("09:30:00")
	if !ok || m != 9*60+30 {
		t.Fatalf("got %d %v", m, ok)
	}
}
