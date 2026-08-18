package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestBuildCardStatsCountsTodayAndRemain(t *testing.T) {
	today := "2026-08-14"
	cards := []model.WBCard{
		// 오늘 · 완료 AS
		{Category: model.WBSourceAS, WorkDate: today, Status: "completed"},
		// 오늘 · 진행중 AS → 남은
		{Category: model.WBSourceAS, WorkDate: today, Status: "in_progress"},
		// 오늘 · 취소 AS → 남은에서 제외
		{Category: model.WBSourceAS, WorkDate: today, Status: "cancelled"},
		// 다른 날 · 미완료 점검 → 남은
		{Category: model.WBSourceMaintenance, WorkDate: "2026-08-17", Status: model.WBTaskWaiting},
		// 다른 날 · 완료 점검
		{Category: model.WBSourceMaintenance, WorkDate: "2026-08-15", Status: model.WBTaskComplete},
		// 아직 배치 전이라 예정일만 있는 행정 업무 → 오늘 · 남은
		{Category: model.WBWorkAdmin, PlannedDay: today, Status: model.WBTaskWaiting},
	}

	got := buildCardStats(cards, today)
	if got.Total != 6 {
		t.Fatalf("총 건수=%d", got.Total)
	}
	if got.Today != 4 {
		t.Fatalf("오늘 건수=%d", got.Today)
	}
	if got.Remain != 3 {
		t.Fatalf("남은 건수=%d", got.Remain)
	}

	empty := buildCardStats(nil, today)
	if empty.Total != 0 || empty.Today != 0 || empty.Remain != 0 {
		t.Fatalf("빈 목록=%+v", empty)
	}
}

func TestCardStatsAddCombinesGroups(t *testing.T) {
	a := wbCardStats{Total: 3, Today: 2, Remain: 1}
	b := wbCardStats{Total: 4, Today: 1, Remain: 4}
	if got := a.add(b); got != (wbCardStats{Total: 7, Today: 3, Remain: 5}) {
		t.Fatalf("%+v", got)
	}
}

// 화면 머리말에 총·오늘·남은 건수가 함께 나와야 한다(일간·주간 동일).
func TestWorkStatusPaletteShowsCounts(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_counts.db")

	seedVisit(t, db, "mvs_c1", "2026-08-14", "최혜영", "KLAS", "나성동도서관")
	seedVisitTask(t, db, "WT-C1", "mvs_c1", "2026-08-14", "09:00", "09:30", "최혜영",
		"[점검]나성동도서관_2026-08-14 · KLAS")
	// 같은 주(월~일) 안의 다른 날
	seedVisit(t, db, "mvs_c2", "2026-08-12", "최혜영", "KLAS", "보람동도서관")
	seedVisitTask(t, db, "WT-C2", "mvs_c2", "2026-08-12", "10:00", "10:30", "최혜영",
		"[점검]보람동도서관_2026-08-12 · KLAS")

	for _, path := range []string{
		"/work-status?view=day&date=2026-08-14&kind=planned",
		"/work-status?view=week&date=2026-08-14&kind=planned",
		"/work-status?view=day&date=2026-08-14&kind=action",
	} {
		body := doGet(t, e, path).Body.String()
		if !strings.Contains(body, "단위 업무별 현황") {
			t.Fatalf("%s 단위 업무별 현황 제목 없음", path)
		}
		if !strings.Contains(body, "총") || !strings.Contains(body, "오늘") || !strings.Contains(body, "남은") {
			t.Fatalf("%s 머리말에 총·오늘·남은 표기 없음", path)
		}
	}

	// 주간 보기에서는 두 건 모두 잡히고, 그중 하나만 8/14
	week := doGet(t, e, "/work-status?view=week&date=2026-08-14&kind=planned").Body.String()
	if !strings.Contains(week, "나성동도서관") || !strings.Contains(week, "보람동도서관") {
		t.Fatal("주간 집계에 기간 내 점검이 빠짐")
	}
}
