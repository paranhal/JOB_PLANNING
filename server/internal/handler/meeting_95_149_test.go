package handler

import (
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_149_T1_ResolveNeedSelectOnlyWhenTwoPlus(t *testing.T) {
	site := map[string][]string{"C1": {"WP1"}, "C3": {"WP1", "WP2"}}
	pid, via, need := repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C0"}, site)
	if pid != "" || via || need {
		t.Fatalf("후보 0 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C1"}, site)
	if pid != "WP1" || !via || need {
		t.Fatalf("후보 1 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{CustomerID: "C3"}, site)
	if pid != "" || via || !need {
		t.Fatalf("후보 2 %s via=%v need=%v", pid, via, need)
	}
	pid, via, need = repository.ResolveMeetingDisplayProject(model.WorkListItem{}, site)
	if pid != "" || via || need {
		t.Fatalf("고객 ID 없음 %s via=%v need=%v", pid, via, need)
	}
}

func Test95_149_T3_SameNameKeepsTwoBlocks(t *testing.T) {
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixAS, RefID: "A1", ProjectID: "WPSEED02", Title: "시드"},
		{Prefix: model.WorkPrefixAS, RefID: "A2", ProjectID: "WP-011", Title: "등록"},
		{Prefix: model.WorkPrefixAS, RefID: "A3", ProjectID: "WP-011", Title: "등록2"},
	}
	h := &MeetingHandler{}
	groups, need, siteN := h.groupWorkItemsByProject(items)
	if need != 0 || siteN != 0 {
		t.Fatalf("need=%d site=%d", need, siteN)
	}
	if len(groups) != 2 {
		t.Fatalf("블록 %d", len(groups))
	}
	ids := map[string]int{}
	for _, g := range groups {
		ids[g.ProjectID] = g.Count
		if g.ProjectID == "" {
			t.Fatal("병합되어 미연결")
		}
	}
	if ids["WPSEED02"] != 1 || ids["WP-011"] != 2 {
		t.Fatalf("%+v", ids)
	}
}

func Test95_149_StatSiteKeepsTitle(t *testing.T) {
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixGeneral, RefID: "g1", OrgName: "해미초", Title: "네트워크점검", Status: model.WBTaskWaiting},
	}
	boxes := buildMeetingProjectStatBoxes(items)
	var admin meetingStatBox
	for _, b := range boxes {
		if b.Key == "admin" {
			admin = b
		}
	}
	if len(admin.Items) != 1 || admin.Items[0].Site != "해미초" || admin.Items[0].Title != "네트워크점검" {
		t.Fatalf("%+v", admin.Items)
	}
}
