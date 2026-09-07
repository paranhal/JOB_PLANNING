package model

import "testing"

func TestWBKanbanBucket(t *testing.T) {
	cases := map[string]string{
		WBTaskWaiting:    WBTaskWaiting,
		WBTaskInProgress: WBTaskInProgress,
		WBTaskHold:       WBTaskInProgress,
		WBTaskTransfer:   WBTaskInProgress,
		WBTaskReview:     WBTaskInProgress,
		WBTaskWaitingFor: WBTaskInProgress,
		WBTaskComplete:   WBTaskComplete,
		WBTaskInbox:      WBTaskWaiting,
		WBTaskCancelled:  "",
		"":               WBTaskWaiting,
	}
	for in, want := range cases {
		if got := WBKanbanBucket(in); got != want {
			t.Fatalf("WBKanbanBucket(%q)=%q want %q", in, got, want)
		}
	}
	if WBKanbanBadge(WBTaskHold) != "보류" || WBKanbanBadge(WBTaskWaitingFor) != "회신대기" {
		t.Fatal("badge")
	}
}

func TestMapASStatusToWB(t *testing.T) {
	cases := map[string]string{
		"hold":             WBTaskHold,
		"transfer":         WBTaskTransfer,
		"completed":        WBTaskComplete,
		"closed":           WBTaskComplete,
		"complete":         WBTaskComplete,
		"done":             WBTaskComplete,
		"in_progress":      WBTaskInProgress,
		"assigned":         WBTaskInProgress,
		"partial_complete": WBTaskInProgress,
		"received":         WBTaskWaiting,
		"waiting":          WBTaskWaiting,
		"planned":          WBTaskWaiting,
		"open":             WBTaskWaiting,
		WBTaskWaitingFor:   WBTaskWaitingFor,
		WBTaskReview:       WBTaskReview,
		"cancelled":        "",
	}
	for in, want := range cases {
		if got := MapASStatusToWB(in); got != want {
			t.Fatalf("MapASStatusToWB(%q)=%q want %q", in, got, want)
		}
	}
	if WBKanbanBucket(MapASStatusToWB("hold")) != WBKanbanBucket(WBTaskHold) {
		t.Fatal("AS 보류와 행정 보류가 다른 열이다")
	}
	if WBKanbanBucket(MapASStatusToWB("waiting")) != WBKanbanBucket(WBTaskWaiting) {
		t.Fatal("진행 예정 열이 행정 칸반과 다르다")
	}
	if WBKanbanBucket(MapASStatusToWB("transfer")) != WBKanbanBucket(WBTaskTransfer) {
		t.Fatal("이관이 행정 칸반과 다른 열이다")
	}
}

func TestApplyKanbanMapping(t *testing.T) {
	hold := WorkListItem{Status: "hold"}
	if !hold.ApplyKanbanMapping() || hold.Bucket != WBTaskInProgress || hold.MappedStatus != WBTaskHold {
		t.Fatalf("hold %+v", hold)
	}
	if WBKanbanBadge(hold.MappedStatus) != "보류" {
		t.Fatal("보류 뱃지")
	}
	waiting := WorkListItem{Status: "received"}
	if !waiting.ApplyKanbanMapping() || waiting.Bucket != WBTaskWaiting {
		t.Fatalf("received %+v", waiting)
	}
	done := WorkListItem{Status: "complete"}
	if !done.ApplyKanbanMapping() || done.Bucket != WBTaskComplete {
		t.Fatalf("complete %+v", done)
	}
	cancelled := WorkListItem{Status: "cancelled"}
	if cancelled.ApplyKanbanMapping() || cancelled.Bucket != "" {
		t.Fatalf("cancelled should drop %+v", cancelled)
	}
}

func TestAssignPlanKanbanExclusive(t *testing.T) {
	selected := "2026-08-20"
	// 지연이면서 진행중 → 진행중 + D+n
	prog := WorkListItem{
		Prefix: WorkPrefixAS, RefID: "p1",
		Status: WBTaskInProgress, ScheduledDate: "2026-08-18", Assignee: "양기헌",
	}
	if !prog.ApplyPlanKanbanMapping(selected) || prog.Bucket != WorkBucketInProgress {
		t.Fatalf("delayed+in_progress → 진행중: %+v", prog)
	}
	if prog.DaysOverdue != 2 {
		t.Fatalf("D+n DaysOverdue=%d", prog.DaysOverdue)
	}
	label, _ := PlanDelayBadge(prog.DaysOverdue, true)
	if label != "진행중 D+2" {
		t.Fatalf("뱃지 %q", label)
	}

	// 3열은 그대로 진행중 (지연 열 없음)
	exec := WorkListItem{Status: WBTaskInProgress, ScheduledDate: "2026-08-18", Assignee: "양기헌"}
	if !exec.ApplyKanbanMapping() || exec.Bucket != WBTaskInProgress {
		t.Fatalf("3열 %+v", exec)
	}

	done := WorkListItem{Prefix: WorkPrefixAS, RefID: "d1", Status: "completed", CompleteDate: selected, ScheduledDate: "2026-08-18", Assignee: "양기헌"}
	if !done.ApplyPlanKanbanMapping(selected) || done.Bucket != WorkBucketCompletedToday {
		t.Fatalf("완료 %+v", done)
	}

	unassigned := WorkListItem{Prefix: WorkPrefixAS, RefID: "u1", Status: "received", ScheduledDate: selected, Assignee: ""}
	if !unassigned.ApplyPlanKanbanMapping(selected) || unassigned.Bucket != WorkBucketUnplanned {
		t.Fatalf("미배정 %+v", unassigned)
	}

	needPlan := WorkListItem{Prefix: WorkPrefixAS, RefID: "n1", Status: "received", ScheduledDate: "", Assignee: "양기헌", NeedPlan: true}
	if !needPlan.ApplyPlanKanbanMapping(selected) || needPlan.Bucket != WorkBucketUnplanned {
		t.Fatalf("미계획 %+v", needPlan)
	}

	delayed := WorkListItem{Prefix: WorkPrefixAS, RefID: "l1", Status: "received", ScheduledDate: "2026-08-18", Assignee: "양기헌"}
	if !delayed.ApplyPlanKanbanMapping(selected) || delayed.Bucket != WorkBucketDelayed {
		t.Fatalf("지연 %+v", delayed)
	}

	today := WorkListItem{Prefix: WorkPrefixAS, RefID: "t1", Status: "received", ScheduledDate: selected, Assignee: "양기헌"}
	if !today.ApplyPlanKanbanMapping(selected) || today.Bucket != WorkBucketToday {
		t.Fatalf("오늘 예정 %+v", today)
	}

	future := WorkListItem{Prefix: WorkPrefixAS, RefID: "f1", Status: "received", ScheduledDate: "2026-08-25", Assignee: "양기헌"}
	if future.ApplyPlanKanbanMapping(selected) {
		t.Fatalf("미래 건은 숨김 %+v", future)
	}

	// 한 카드 한 열 — 같은 키를 두 열에 넣지 않는다
	items := []WorkListItem{prog, done, unassigned, delayed, today}
	seen := map[string]string{}
	counts := map[string]int{}
	for i := range items {
		if !items[i].ApplyPlanKanbanMapping(selected) {
			t.Fatalf("배정 실패 %+v", items[i])
		}
		k := WorkListItemKey(items[i])
		if prev, ok := seen[k]; ok {
			t.Fatalf("중복 열 %s: %s 와 %s", k, prev, items[i].Bucket)
		}
		seen[k] = items[i].Bucket
		counts[items[i].Bucket]++
	}
	sum := 0
	for _, n := range counts {
		sum += n
	}
	if sum != len(items) {
		t.Fatalf("합계 %d != %d", sum, len(items))
	}
}

func TestPlanKanbanColumnTitleSelectedDate(t *testing.T) {
	if got := PlanKanbanColumnTitle(WorkBucketToday, "2026-08-18", "2026-08-19"); got != "8/18 예정" {
		t.Fatalf("어제 예정 라벨: %q", got)
	}
	if got := PlanKanbanColumnTitle(WorkBucketCompletedToday, "2026-08-20", "2026-08-20"); got != "오늘 완료" {
		t.Fatalf("오늘 완료 라벨: %q", got)
	}
	if got := PlanKanbanColumnTitle(WorkBucketToday, "2026-08-20", "2026-08-20"); got != "오늘 예정" {
		t.Fatalf("오늘 예정 라벨: %q", got)
	}
}

func TestSortWorkListItemsDefaultDueDate(t *testing.T) {
	items := []WorkListItem{
		{RefNumber: "b", DueDate: "2026-08-20"},
		{RefNumber: "a", DueDate: "2026-08-10"},
		{RefNumber: "c", DueDate: ""},
	}
	SortWorkListItems(items, "", "")
	if items[0].RefNumber != "a" || items[1].RefNumber != "b" || items[2].RefNumber != "c" {
		t.Fatalf("order %+v", items)
	}
}

func TestWBTaskOnDate(t *testing.T) {
	if !WBTaskOnDate(WorkTask{WorkDate: "2026-08-07"}, "2026-08-07") {
		t.Fatal("work_date")
	}
	if !WBTaskOnDate(WorkTask{DueDate: "2026-08-07"}, "2026-08-07") {
		t.Fatal("due_date fallback")
	}
	if WBTaskOnDate(WorkTask{WorkDate: "2026-08-06", DueDate: "2026-08-07"}, "2026-08-07") {
		t.Fatal("work_date should win over due_date")
	}
}
