package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestBuildStatsPeriodColumnsDay(t *testing.T) {
	anchor := time.Date(2026, 8, 7, 12, 0, 0, 0, time.Local)
	cols := buildStatsPeriodColumns(model.StatsViewDay, anchor, anchor)
	if len(cols) != 3 {
		t.Fatalf("len=%d", len(cols))
	}
	if cols[0].Label != "전일" || cols[1].Label != "오늘" || cols[2].Label != "익일" {
		t.Fatalf("labels: %v %v %v", cols[0].Label, cols[1].Label, cols[2].Label)
	}
	if cols[0].From != "2026-08-06" || cols[1].From != "2026-08-07" || cols[2].From != "2026-08-08" {
		t.Fatalf("from: %s %s %s", cols[0].From, cols[1].From, cols[2].From)
	}
	if !cols[0].ShowActual || !cols[1].ShowActual || cols[2].ShowActual {
		t.Fatalf("ShowActual: prev=%v current=%v next=%v", cols[0].ShowActual, cols[1].ShowActual, cols[2].ShowActual)
	}
}

func TestPeriodShowActualPastAnchorShowsAll(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.Local)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := buildStatsPeriodColumns(model.StatsViewDay, anchor, now)
	if !cols[0].ShowActual || !cols[1].ShowActual || !cols[2].ShowActual {
		t.Fatalf("past day columns should show actuals: %+v", cols)
	}
	week := buildStatsPeriodColumns(model.StatsViewWeek, now, now)
	if !week[0].ShowActual || !week[1].ShowActual || week[2].ShowActual {
		t.Fatalf("week ShowActual prev=%v cur=%v next=%v", week[0].ShowActual, week[1].ShowActual, week[2].ShowActual)
	}
	month := buildStatsPeriodColumns(model.StatsViewMonth, now, now)
	if !month[0].ShowActual || !month[1].ShowActual || month[2].ShowActual {
		t.Fatalf("month ShowActual prev=%v cur=%v next=%v", month[0].ShowActual, month[1].ShowActual, month[2].ShowActual)
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

func TestBuildStatsRangeColumns(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.Local)
	cols := buildStatsRangeColumns(from, to, now)
	if len(cols) != 3 {
		t.Fatalf("len=%d", len(cols))
	}
	if cols[1].From != "2026-08-01" || cols[1].ToExclusive != "2026-08-08" {
		t.Fatalf("current %s ~ %s", cols[1].From, cols[1].ToExclusive)
	}
	if cols[0].From != "2026-07-25" || cols[0].ToExclusive != "2026-08-01" {
		t.Fatalf("prev %s ~ %s", cols[0].From, cols[0].ToExclusive)
	}
	if cols[2].From != "2026-08-08" || cols[2].ToExclusive != "2026-08-15" {
		t.Fatalf("next %s ~ %s", cols[2].From, cols[2].ToExclusive)
	}
	if !cols[0].ShowActual || !cols[1].ShowActual || !cols[2].ShowActual {
		t.Fatal("started range columns should show actuals")
	}
}

func TestPeriodOverviewExcludesCancelledAndASSourcedAdmin(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "ov.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, status, assigned_to, complete_datetime)
		VALUES
		('a1','R1','c1','2026-08-07 09:00:00','2026-08-07','completed','양기헌','2026-08-07 15:00:00'),
		('a2','R2','c1','2026-08-07 10:00:00','2026-08-07','cancelled','양기헌',NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, due_date, work_date, status, source_type, source_id)
		VALUES
		('t1','admin','행정','2026-08-07','2026-08-07','complete','',''),
		('t2','admin','AS잘못분류','2026-08-07','2026-08-07','waiting','as','a1')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := buildStatsPeriodColumns(model.StatsViewDay, anchor, anchor)
	if err := repo.FillPeriodOverview(cols, ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatal(err)
	}
	cur := cols[1].Counts
	if cur.AS.Receipt != 1 {
		t.Fatalf("AS receipt=%d want 1 (cancelled excluded)", cur.AS.Receipt)
	}
	if cur.AS.Planned != 1 {
		t.Fatalf("AS planned=%d want 1 (cancelled excluded)", cur.AS.Planned)
	}
	if cur.Admin.Planned != 1 {
		t.Fatalf("admin planned=%d want 1 (AS source excluded)", cur.Admin.Planned)
	}
	if cur.Admin.Receipt != 1 {
		t.Fatalf("admin receipt=%d want 1", cur.Admin.Receipt)
	}
}

func TestParseStatsRangeBoundsSwapAndDefault(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.Local)
	from, to := ParseStatsRangeBounds("", "", now)
	if from.Format("2006-01-02") != "2026-08-01" || to.Format("2006-01-02") != "2026-08-14" {
		t.Fatalf("default %v ~ %v", from, to)
	}
	from, to = ParseStatsRangeBounds("2026-08-10", "2026-08-01", now)
	if from.Format("2006-01-02") != "2026-08-01" || to.Format("2006-01-02") != "2026-08-10" {
		t.Fatalf("swap %v ~ %v", from, to)
	}
}
