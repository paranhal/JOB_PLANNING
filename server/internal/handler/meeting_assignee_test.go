package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestGroupWorkItemsByAssigneeOrderAndUnassignedLast(t *testing.T) {
	items := []model.WorkListItem{
		{Assignee: "", RefNumber: "U1", Prefix: model.WorkPrefixAS, ScheduledDate: "09:00"},
		{Assignee: "최혜영", RefNumber: "C2", Prefix: model.WorkPrefixGeneral, ScheduledDate: "10:00"},
		{Assignee: "양기헌", RefNumber: "A1", Prefix: model.WorkPrefixAS, ScheduledDate: "11:00"},
		{Assignee: "최혜영", RefNumber: "C1", Prefix: model.WorkPrefixAS, ScheduledDate: "08:00"},
		{Assignee: "양기헌", RefNumber: "A2", Prefix: model.WorkPrefixMaintenance, ScheduledDate: "11:00"},
	}
	order := []string{"양기헌", "최혜영", "태자운"}
	g := groupWorkItemsByAssignee(items, order)
	if len(g) != 3 {
		t.Fatalf("groups=%d want 3", len(g))
	}
	if g[0].Label != "양기헌" || g[0].Count != 2 {
		t.Fatalf("first=%s n=%d", g[0].Label, g[0].Count)
	}
	if g[1].Label != "최혜영" || g[1].Count != 2 {
		t.Fatalf("second=%s n=%d", g[1].Label, g[1].Count)
	}
	if !g[2].Unassigned || g[2].Label != "미배정" || g[2].Count != 1 {
		t.Fatalf("last=%+v", g[2])
	}
	if g[1].Items[0].RefNumber != "C1" {
		t.Fatalf("최혜영 시각 순: %s", g[1].Items[0].RefNumber)
	}
	if g[0].Items[0].Prefix != model.WorkPrefixAS || g[0].Items[1].Prefix != model.WorkPrefixMaintenance {
		t.Fatalf("같은 시각이면 유형 순: %s %s", g[0].Items[0].Prefix, g[0].Items[1].Prefix)
	}
}

func TestMeetingFilterQueryKeepsDateAssigneeDisplay(t *testing.T) {
	got := meetingFilterQuery("2026-08-11", "양기헌", "kanban")
	if !strings.Contains(got, "date=2026-08-11") || !strings.Contains(got, "assignee=") || !strings.Contains(got, "display=kanban") {
		t.Fatalf("%s", got)
	}
	list := meetingFilterQuery("2026-08-11", "양기헌", "list")
	if !strings.Contains(list, "display=list") || !strings.Contains(list, "date=2026-08-11") {
		t.Fatalf("list %s", list)
	}
	nav := meetingNavQuery("양기헌", "kanban")
	if strings.Contains(nav, "date=") || !strings.Contains(nav, "display=kanban") {
		t.Fatalf("nav %s", nav)
	}
}

func TestFilterWorkItemsByAssignee(t *testing.T) {
	items := []model.WorkListItem{
		{Assignee: "양기헌", RefNumber: "A"},
		{Assignee: "최혜영", RefNumber: "C"},
	}
	got := filterWorkItemsByAssignee(items, "양기헌")
	if len(got) != 1 || got[0].RefNumber != "A" {
		t.Fatalf("%+v", got)
	}
	if n := len(filterWorkItemsByAssignee(items, "")); n != 2 {
		t.Fatalf("팀전체=%d", n)
	}
}

func TestAssignableNameOrderHangul(t *testing.T) {
	users := []model.User{{FullName: "최혜영"}, {FullName: "양기헌"}, {FullName: "태자운"}}
	got := assignableNameOrder(users)
	want := []string{"양기헌", "최혜영", "태자운"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("%v want %v", got, want)
	}
}
