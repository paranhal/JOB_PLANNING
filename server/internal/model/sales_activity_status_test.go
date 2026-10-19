package model

import "testing"

func TestDeriveSalesActivityStatusTable(t *testing.T) {
	today := "2026-10-19"
	latest := map[string]string{"S1": "A1", "S2": "A2"}
	rows := []struct {
		name string
		a    SalesActivity
		want string
	}{
		{"예정", SalesActivity{ActivityID: "A9", SalesID: "S1", ActivityDate: "2026-10-20"}, ActStatusPlanned},
		{"완료", SalesActivity{ActivityID: "A0", SalesID: "S1", ActivityDate: "2026-10-18"}, ActStatusDone},
		{"후속지연", SalesActivity{ActivityID: "A1", SalesID: "S1", ActivityDate: "2026-10-18", NextActionDate: "2026-10-18"}, ActStatusFollowup},
		{"오늘지연아님", SalesActivity{ActivityID: "A1", SalesID: "S1", ActivityDate: "2026-10-18", NextActionDate: "2026-10-19"}, ActStatusDone},
		{"다음일정없음", SalesActivity{ActivityID: "A1", SalesID: "S1", ActivityDate: "2026-10-18"}, ActStatusDone},
		{"이후활동해소", SalesActivity{ActivityID: "A0", SalesID: "S2", ActivityDate: "2026-10-10", NextActionDate: "2026-10-01"}, ActStatusDone},
	}
	for _, r := range rows {
		if got := DeriveSalesActivityStatus(r.a, latest, today); got != r.want {
			t.Errorf("%s: got %s want %s", r.name, got, r.want)
		}
	}
}

func TestBuildSalesActivityKPIWeekAndRate(t *testing.T) {
	today := "2026-10-19" // Monday
	acts := []SalesActivity{
		{ActivityID: "1", SalesID: "A", ActivityDate: "2026-10-22", CreatedAt: "2026-10-01"}, // planned this week
		{ActivityID: "2", SalesID: "B", ActivityDate: "2026-10-10", CreatedAt: "2026-09-01"}, // done
		{ActivityID: "3", SalesID: "C", ActivityDate: "2026-10-10", NextActionDate: "2026-10-01", CreatedAt: "2026-10-05"},
	}
	latest := LatestActivityIDBySales(acts)
	k := BuildSalesActivityKPI(acts, latest, today)
	if k.Total != 3 || k.WeekPlanned != 1 || k.Followup != 1 || k.Done != 2 {
		t.Fatalf("kpi=%+v", k)
	}
	if k.DoneRate != "66%" {
		t.Fatalf("rate=%s", k.DoneRate)
	}
	if k.MonthNew != 2 {
		t.Fatalf("month new=%d", k.MonthNew)
	}
}

func TestPeriodRange(t *testing.T) {
	from, to := PeriodRange("month", "2026-10-19")
	if from != "2026-10-01" || to != "2026-10-31" {
		t.Fatalf("month %s %s", from, to)
	}
	from, to = PeriodRange("30", "2026-10-19")
	if from != "2026-09-20" || to != "2026-10-19" {
		t.Fatalf("30 %s %s", from, to)
	}
	from, to = PeriodRange("all", "2026-10-19")
	if from != "" || to != "" {
		t.Fatalf("all %s %s", from, to)
	}
}
