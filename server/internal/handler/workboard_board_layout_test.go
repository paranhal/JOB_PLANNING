package handler

import "testing"

func TestLayoutDayEventsNoOverlap(t *testing.T) {
	got := layoutDayEvents([]DayLayoutEvent{
		{StartMin: 9 * 60, EndMin: 10 * 60},
		{StartMin: 11 * 60, EndMin: 12 * 60},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Cols != 1 || got[1].Cols != 1 || got[0].Col != 0 || got[1].Col != 0 {
		t.Fatalf("겹침 없음 %+v", got)
	}
}

func TestLayoutDayEventsTwoOverlap(t *testing.T) {
	got := layoutDayEvents([]DayLayoutEvent{
		{StartMin: 14 * 60, EndMin: 17 * 60},
		{StartMin: 15 * 60, EndMin: 16 * 60},
	})
	if got[0].Cols != 2 || got[1].Cols != 2 {
		t.Fatalf("2건 겹침 cols %+v", got)
	}
	if got[0].Col == got[1].Col {
		t.Fatalf("같은 열 %+v", got)
	}
}

func TestLayoutDayEventsThreeChain(t *testing.T) {
	got := layoutDayEvents([]DayLayoutEvent{
		{StartMin: 8 * 60, EndMin: 10 * 60},
		{StartMin: 9 * 60, EndMin: 11 * 60},
		{StartMin: 10 * 60, EndMin: 12 * 60},
	})
	if got[0].Cols < 2 || got[1].Cols < 2 {
		t.Fatalf("연쇄 겹침 %+v", got)
	}
	max := 0
	used := map[int]bool{}
	for _, e := range got {
		if e.Cols > max {
			max = e.Cols
		}
		used[e.Col] = true
	}
	if max < 2 {
		t.Fatalf("열 수 %d", max)
	}
	if len(used) < 2 {
		t.Fatalf("사용한 열 %d", len(used))
	}
}

func TestLayoutDayEventsTouchingBoundary(t *testing.T) {
	got := layoutDayEvents([]DayLayoutEvent{
		{StartMin: 9 * 60, EndMin: 10 * 60},
		{StartMin: 10 * 60, EndMin: 11 * 60},
	})
	if got[0].Cols != 1 || got[1].Cols != 1 {
		t.Fatalf("경계는 겹치지 않아야 함 %+v", got)
	}
}

func TestLayoutDayEventsMissingEnd(t *testing.T) {
	got := layoutDayEvents([]DayLayoutEvent{
		{StartMin: 9 * 60, EndMin: 0},
		{StartMin: 9 * 60, EndMin: 9*60 + 10},
	})
	if got[0].EndMin <= got[0].StartMin {
		t.Fatalf("종료 보정 실패 %+v", got[0])
	}
	if got[0].Cols < 2 {
		t.Fatalf("시작만 있는 건도 겹침 처리 %+v", got)
	}
}
