package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestMntVisitState(t *testing.T) {
	today := "2026-10-22"
	if mntVisitState(model.MaintenanceVisit{Completed: true, VisitDate: "2026-10-01", Assignee: "김"}, today) != mntStateDone {
		t.Fatal("완료")
	}
	if mntVisitState(model.MaintenanceVisit{VisitDate: today, Assignee: "김"}, today) != mntStatePlanned {
		t.Fatal("오늘 예정")
	}
	if mntVisitState(model.MaintenanceVisit{VisitDate: "2026-10-23", Assignee: "김"}, today) != mntStatePlanned {
		t.Fatal("미래 예정")
	}
	if mntVisitState(model.MaintenanceVisit{VisitDate: "2026-10-21", Assignee: "김"}, today) != mntStateCheck {
		t.Fatal("경과")
	}
	if mntVisitState(model.MaintenanceVisit{VisitDate: "2026-10-23"}, today) != mntStateCheck {
		t.Fatal("담당 없음")
	}
	if mntVisitState(model.MaintenanceVisit{VisitDate: "2026-10-23", Assignee: "김", DupReason: "월2"}, today) != mntStateCheck {
		t.Fatal("중복")
	}
	people := model.MaintenanceVisit{VisitDate: "2026-10-23", People: []model.MaintenanceVisitAssignee{{UserID: "u1", AssigneeName: "박"}}}
	if mntVisitState(people, today) != mntStatePlanned {
		t.Fatal("복수 담당은 예정")
	}
}

func TestFilterVisitsByAssigneeKey(t *testing.T) {
	a := model.MaintenanceVisit{VisitID: "1", Assignee: "김서준", People: []model.MaintenanceVisitAssignee{{UserID: "u1", AssigneeName: "김서준"}, {UserID: "u2", AssigneeName: "박지훈"}}}
	b := model.MaintenanceVisit{VisitID: "2", WorkGroupID: "g1", Assignee: "최유진", People: []model.MaintenanceVisitAssignee{{UserID: "u3", AssigneeName: "최유진"}}}
	c := model.MaintenanceVisit{VisitID: "3"}
	visits := []model.MaintenanceVisit{a, b, c}
	mem := map[string]map[string]bool{"g1": {"u3": true}}

	if n := filterVisitsByAssigneeKey(visits, "all", mem, ""); len(n) != 3 {
		t.Fatalf("all %d", len(n))
	}
	got := filterVisitsByAssigneeKey(visits, "user:u2", mem, "")
	if len(got) != 1 || got[0].VisitID != "1" {
		t.Fatalf("user 복수 중 1명 %+v", got)
	}
	got = filterVisitsByAssigneeKey(visits, "group:g1", mem, "")
	if len(got) != 1 || got[0].VisitID != "2" {
		t.Fatalf("group %+v", got)
	}
	got = filterVisitsByAssigneeKey(visits, "none", mem, "")
	if len(got) != 1 || got[0].VisitID != "3" {
		t.Fatalf("none %+v", got)
	}
}

func TestMntMonthStatsIdentity(t *testing.T) {
	today := "2026-10-22"
	visits := []model.MaintenanceVisit{
		{Completed: true, VisitDate: "2026-10-01", Assignee: "a"},
		{VisitDate: "2026-10-23", Assignee: "a"},
		{VisitDate: "2026-10-01"},
	}
	s := mntMonthStats(visits, today)
	if s.Total != 3 || s.Done != 1 || s.Open != 2 {
		t.Fatalf("%+v", s)
	}
	if s.Total != s.Done+s.Open {
		t.Fatal("합계")
	}
	filtered := filterVisitsByAssigneeKey(visits, "none", nil, "")
	fs := mntMonthStats(filtered, today)
	if fs.Total != fs.Done+fs.Open || fs.Total != 1 {
		t.Fatalf("필터 통계 %+v", fs)
	}
}

func TestBuildDayChipsMore(t *testing.T) {
	var vs []model.MaintenanceVisit
	for i := 0; i < 5; i++ {
		vs = append(vs, model.MaintenanceVisit{VisitID: "v", ShortName: "사이트", VisitDate: "2026-10-10", Assignee: "김"})
	}
	chips, more := buildDayChips(vs, "2026-10-01", 3)
	if len(chips) != 3 || more != 2 {
		t.Fatalf("chips=%d more=%d", len(chips), more)
	}
}

func TestDefaultSelectedDay(t *testing.T) {
	visits := []model.MaintenanceVisit{{VisitDate: "2026-10-08"}, {VisitDate: "2026-10-03"}}
	if d := defaultSelectedDay(2026, 10, "2026-10-22", visits); d != "2026-10-22" {
		t.Fatalf("이번 달 오늘 %s", d)
	}
	if d := defaultSelectedDay(2026, 9, "2026-10-22", visits); d != "2026-09-01" {
		t.Fatalf("방문 없는 달 %s", d)
	}
	if d := defaultSelectedDay(2026, 10, "2026-09-01", visits); d != "2026-10-03" {
		t.Fatalf("첫 방문일 %s", d)
	}
}

func TestBuildVisitCalendarOtherMonthLinks(t *testing.T) {
	weeks := buildVisitCalendar(2026, 10, nil, "2026-10-22")
	if len(weeks) != 6 {
		t.Fatal("6주")
	}
	// 2026-10-01 목요일 → 첫 칸은 9월 27일
	if weeks[0][0].Date != "2026-09-27" || !weeks[0][0].OtherMonth {
		t.Fatalf("이전달 %+v", weeks[0][0])
	}
	if weeks[0][0].Month != 9 || weeks[0][0].OtherYear {
		t.Fatalf("9월 칸 %+v", weeks[0][0])
	}
}

func TestCalQueryPreservesAssigneeAll(t *testing.T) {
	got := calQuery("mpl_1", "calendar", 9, "user:u1", "1", "2026-09-30")
	for _, w := range []string{"view=calendar", "month=9", "assignee=user", "all=1", "day=2026-09-30"} {
		if !strings.Contains(got, w) {
			t.Fatalf("%s 없음 %s", w, got)
		}
	}
}
