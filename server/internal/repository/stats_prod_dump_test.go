package repository

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 운영 사본 통계 비교용. STATS_DUMP_DB 가 있을 때만 실행한다. 커밋하지 않는다.
func TestDumpProdStatsSnapshot(t *testing.T) {
	src := os.Getenv("STATS_DUMP_DB")
	if src == "" {
		t.Skip("STATS_DUMP_DB")
	}
	dst := filepath.Join(t.TempDir(), "app.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()

	var db *sql.DB
	if os.Getenv("STATS_DUMP_NO_INIT") == "1" {
		db, err = sql.Open("sqlite", dst+"?_pragma=foreign_keys(1)")
	} else {
		db, err = InitDB(dst)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var nProj, nMaint, nOther int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&nProj)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE COALESCE(project_kind,'maintenance')='maintenance'`).Scan(&nMaint)
	nOther = nProj - nMaint

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 10, 16, 12, 0, 0, 0, time.Local)
	dumpWindow := func(tag, from, toEx string, includeImport bool) {
		t.Helper()
		f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
		f.IncludeImport = includeImport
		cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
		if err := repo.FillPeriodOverview(cols, f); err != nil {
			t.Fatal(tag, err)
		}
		kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
		if err != nil {
			t.Fatal(tag, err)
		}
		an, err := repo.LoadStatsWorkAnalysis(from, toEx, f)
		if err != nil {
			t.Fatal(tag, err)
		}
		asE, err := repo.listWeeklyASEvents(from, toEx)
		if err != nil {
			t.Fatal(tag, err)
		}
		mntE, err := repo.listWeeklyMntEvents(from, toEx)
		if err != nil {
			t.Fatal(tag, err)
		}
		adE, err := repo.listWeeklyAdminEvents(from, toEx)
		if err != nil {
			t.Fatal(tag, err)
		}
		fmt.Printf("DUMP %s kpi visit=%.6f visitN=%d complete=%.6f completeN=%d planOpen=%d planPlanned=%d planRate=%.4f adminMoved=%d\n",
			tag, kpi.VisitAvgDays, kpi.VisitSample, kpi.CompleteAvgDays, kpi.CompleteSample,
			kpi.PlanningOpen, kpi.PlanningPlanned, kpi.PlanningRate, kpi.AdminMovedN)
		fmt.Printf("DUMP %s AS r=%d v=%d c=%d | Mnt r=%d v=%d c=%d | Admin r=%d v=%d c=%d\n",
			tag, an.AS.Receipt, an.AS.Visit, an.AS.Completed,
			an.Mnt.Receipt, an.Mnt.Visit, an.Mnt.Completed,
			an.Admin.Receipt, an.Admin.Visit, an.Admin.Completed)
		fmt.Printf("DUMP %s events as=%d mnt=%d admin=%d\n", tag, len(asE), len(mntE), len(adE))
	}

	fmt.Printf("DUMP projects=%d maintenance=%d other_kind=%d\n", nProj, nMaint, nOther)
	dumpWindow("oct_app", "2026-10-01", "2026-11-01", false)
	dumpWindow("oct_all", "2026-10-01", "2026-11-01", true)
	dumpWindow("y2026_app", "2026-01-01", "2027-01-01", false)
	wp := CompanyWeeklyPeriod(anchor)
	dumpWindow("week_app", wp.ThisFrom, wp.ThisToEx, false)
	dumpWindow("week_all", wp.ThisFrom, wp.ThisToEx, true)
	fmt.Printf("DUMP week_range %s..%s\n", wp.ThisFrom, wp.ThisTo)
}
