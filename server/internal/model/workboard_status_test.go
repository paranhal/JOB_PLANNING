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
		"hold":        WBTaskHold,
		"transfer":    WBTaskTransfer,
		"completed":   WBTaskComplete,
		"closed":      WBTaskComplete,
		"in_progress": WBTaskInProgress,
		"assigned":    WBTaskInProgress,
		"received":    WBTaskWaiting,
		"cancelled":   "",
	}
	for in, want := range cases {
		if got := MapASStatusToWB(in); got != want {
			t.Fatalf("MapASStatusToWB(%q)=%q want %q", in, got, want)
		}
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
