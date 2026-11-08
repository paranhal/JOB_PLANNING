package handler

import (
	"testing"

	"customer-support/internal/model"
)

func Test95_147_T1_StatBoxesFromItems(t *testing.T) {
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixMaintenance, RefID: "m1", Status: "completed", Title: "점검"},
		{Prefix: model.WorkPrefixMaintenance, RefID: "m2", Status: "planned", Title: "미점검"},
		{Prefix: model.WorkPrefixAS, RefID: "a1", Status: "in_progress", Title: "AS진행"},
		{Prefix: model.WorkPrefixAS, RefID: "a2", Status: "completed", Title: "AS완료"},
		{Prefix: model.WorkPrefixProjectWork, RefID: "w1", Status: model.WBTaskComplete, Title: "작업"},
		{Prefix: model.WorkPrefixGeneral, RefID: "g1", Status: model.WBTaskComplete, Title: "행정"},
		{Prefix: model.WorkPrefixSupport, RefID: "s1", Status: model.WBTaskWaiting, Title: "지원"},
		{Prefix: model.WorkPrefixSales, RefID: "sa1", Status: model.WBTaskWaiting, Title: "영업"},
	}
	g := meetingProjectGroup{Label: "본사업", Count: len(items), Items: items}
	boxes := buildMeetingProjectStatBoxes(items)
	if len(boxes) != 5 {
		t.Fatalf("박스 %d", len(boxes))
	}
	by := map[string]meetingStatBox{}
	sum := 0
	for _, b := range boxes {
		by[b.Key] = b
		sum += b.Total
	}
	if sum != g.Count {
		t.Fatalf("합계 %d Count %d", sum, g.Count)
	}
	if by["mnt"].Total != 2 || by["mnt"].Done != 1 || by["mnt"].Rate != "50%" || by["mnt"].Line != "완료 1 / 총 2건" {
		t.Fatalf("정기점검 %+v", by["mnt"])
	}
	if by["as"].Total != 2 || by["as"].Done != 1 || by["as"].Line != "방문 2 · 완료 1" {
		t.Fatalf("AS %+v", by["as"])
	}
	if by["work"].Total != 1 || by["work"].Done != 1 {
		t.Fatalf("작업에 정기점검/AS가 들어감 %+v", by["work"])
	}
	if by["admin"].Total != 2 || by["admin"].Done != 1 {
		t.Fatalf("행정/지원 %+v", by["admin"])
	}
	if by["sales"].Total != 1 || by["sales"].Done != 0 || by["sales"].Rate != "0%" {
		t.Fatalf("영업 %+v", by["sales"])
	}
	empty := buildMeetingProjectStatBoxes(nil)
	if empty[0].Rate != "—" || empty[0].Line != "완료 0 / 총 0건" {
		t.Fatalf("분모 0 %+v", empty[0])
	}
}

func Test95_147_T4_BoxesDoNotNeedRepo(t *testing.T) {
	h := &MeetingHandler{}
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixAS, RefID: "a", ProjectID: "WP1", Status: "in_progress"},
		{Prefix: model.WorkPrefixProjectWork, RefID: "w", ProjectID: "WP1", Status: model.WBTaskWaiting},
	}
	groups, _, _ := h.groupWorkItemsByProject(items)
	attachMeetingProjectStatBoxes(groups)
	if len(groups) != 1 || groups[0].Count != 2 {
		t.Fatalf("%+v", groups)
	}
	sum := 0
	for _, b := range groups[0].StatBoxes {
		sum += b.Total
	}
	if sum != groups[0].Count {
		t.Fatalf("합 %d", sum)
	}
}
