package repository

import (
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestBuildStatsPeriodColumnsDay(t *testing.T) {
	anchor := time.Date(2026, 8, 7, 12, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewDay, anchor)
	if len(cols) != 3 {
		t.Fatalf("len=%d", len(cols))
	}
	if cols[0].Label != "전일" || cols[1].Label != "오늘" || cols[2].Label != "익일" {
		t.Fatalf("labels: %v %v %v", cols[0].Label, cols[1].Label, cols[2].Label)
	}
	if cols[0].From != "2026-08-06" || cols[1].From != "2026-08-07" || cols[2].From != "2026-08-08" {
		t.Fatalf("from: %s %s %s", cols[0].From, cols[1].From, cols[2].From)
	}
	if !cols[0].ShowActual || cols[1].ShowActual || cols[2].ShowActual {
		t.Fatalf("ShowActual: prev=%v current=%v next=%v", cols[0].ShowActual, cols[1].ShowActual, cols[2].ShowActual)
	}
}

func TestBuildStatsPeriodColumnsWeek(t *testing.T) {
	// 2026-08-07 = Friday → week Mon 8/3 ~ Sun 8/9
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if cols[1].From != "2026-08-03" || cols[1].ToExclusive != "2026-08-10" {
		t.Fatalf("금주: %s ~ %s", cols[1].From, cols[1].ToExclusive)
	}
	if cols[0].Label != "전주" || cols[2].Label != "차주" {
		t.Fatalf("labels week")
	}
}

func TestBuildStatsPeriodColumnsMonth(t *testing.T) {
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if cols[0].Label != "전월" || cols[1].From != "2026-08-01" || cols[2].Label != "익월" {
		t.Fatalf("%+v", cols)
	}
}
