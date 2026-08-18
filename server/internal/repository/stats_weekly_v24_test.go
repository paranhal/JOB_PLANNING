package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCountWorkingDaysHolidayTable(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "wd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewStatsRepo(db)

	n, missing, err := repo.CountWorkingDays("2028-08-10", "2028-08-17")
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 || !missing {
		t.Fatalf("공휴일 미등록: days=%d missing=%v want 5/true", n, missing)
	}

	if _, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2028-08-14', 2028, '임시휴일', 'company')`); err != nil {
		t.Fatal(err)
	}
	n, missing, err = repo.CountWorkingDays("2028-08-10", "2028-08-17")
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || missing {
		t.Fatalf("공휴일 등록: days=%d missing=%v want 4/false", n, missing)
	}
}

func TestWeeklyReportTeamNotSumAndDayMaxTie(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin
		) VALUES
		('a1','R1','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('a2','R2','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('a3','R3','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('b1','R4','c1','2026-08-10 09:00:00','2026-08-12','2026-08-13 10:00:00','2026-08-13 16:00:00','completed','양기헌','app'),
		('u1','R5','c1','2026-08-10 09:00:00',NULL,'2026-08-11 10:00:00','2026-08-11 12:00:00','completed','','app'),
		('u2','R6','c1','2026-08-12 09:00:00',NULL,NULL,NULL,'in_progress','','app')
	`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	rep, err := repo.BuildWeeklyReport(time.Date(2026, 8, 12, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if rep.WeekFrom != "2026-08-10" || rep.WeekTo != "2026-08-16" {
		t.Fatalf("기간 %s~%s", rep.WeekFrom, rep.WeekTo)
	}
	if rep.Friday != "2026-08-14" {
		t.Fatalf("금요일=%s", rep.Friday)
	}
	if rep.WorkingDays != 5 {
		t.Fatalf("워킹데이=%d", rep.WorkingDays)
	}
	if rep.HolidayYearMissing {
		t.Fatal("2026은 시드되어 미등록이 아니어야 함")
	}

	if len(rep.PersonRows) < 2 || !rep.PersonRows[0].IsTeam {
		t.Fatalf("행 %+v", rep.PersonRows)
	}
	team := rep.PersonRows[0]
	var hye, ki, unassigned *model.WeeklyPersonRow
	for i := range rep.PersonRows[1:] {
		p := &rep.PersonRows[i+1]
		switch p.Label {
		case "최혜경":
			hye = p
		case "양기헌":
			ki = p
		case model.StatsUnassignedLabel:
			unassigned = p
		}
	}
	if hye == nil || ki == nil {
		t.Fatalf("담당자 행 없음 %+v", rep.PersonRows)
	}
	if hye.Completed != 3 || ki.Completed != 1 {
		t.Fatalf("완료 최혜경=%d 양기헌=%d", hye.Completed, ki.Completed)
	}
	if hye.ExecDisplay.Value != 100 || ki.ExecDisplay.Value != 0 {
		t.Fatalf("개인 실행률 최혜경=%.1f 양기헌=%.1f", hye.ExecDisplay.Value, ki.ExecDisplay.Value)
	}
	if team.ExecDisplay.Value != 75 {
		t.Fatalf("팀 실행률=%.1f want 75 (3/4, 개인 평균 50이 아님)", team.ExecDisplay.Value)
	}
	if hye.HasDayAvg && hye.DayAvg != 0.6 {
		t.Fatalf("최혜경 일평균=%.1f want 0.6", hye.DayAvg)
	}
	if team.DayMax != 4 || team.DayMaxDate != "2026-08-11" {
		t.Fatalf("팀 일최대=%d %s want 4 2026-08-11 (동점이면 이른 날)", team.DayMax, team.DayMaxDate)
	}
	if unassigned == nil {
		t.Fatal("미배정 행 없음")
	}
	last := rep.PersonRows[len(rep.PersonRows)-1]
	if last.Label != model.StatsUnassignedLabel {
		t.Fatalf("마지막 행=%q want 미배정", last.Label)
	}
	if hye.Receipt != 3 {
		t.Fatalf("접수는 배정 기준이어야 함 최혜경=%d", hye.Receipt)
	}
}

func TestWeeklyEventRowsReceiptActionComplete(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "ev.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','충남교육청','충남교육청',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin, symptom, action_taken
		) VALUES
		('a1','R2608-051','c1','2026-08-11 09:00:00','2026-08-12','2026-08-12 10:00:00','2026-08-12 16:00:00','completed','최혜경','app','대출반납기 오류','부품 교체'),
		('a2','R2608-010','c1','2026-08-04 09:00:00','2026-08-12','2026-08-12 10:00:00','2026-08-12 16:00:00','completed','양기헌','app','지난주 접수','원격 재기동'),
		('imp','R-IMP','c1','2026-08-11 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','import','이관','이관')
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('p1','a1','2026-08-12 10:00:00','최혜경','부품 교체',40)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	rep, err := repo.BuildWeeklyReport(time.Date(2026, 8, 12, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, ev := range rep.Events {
		if ev.WorkNo == "R2608-051" {
			kinds = append(kinds, ev.Kind)
		}
		if ev.WorkNo == "R-IMP" {
			t.Fatal("이관 이벤트")
		}
		if ev.WorkNo == "R2608-010" && ev.Kind == model.WeeklyEventReceipt {
			t.Fatal("지난주 접수 행이 이번 주에 나오면 안 됨")
		}
	}
	if len(kinds) != 3 {
		t.Fatalf("같은 건 접수·조치·완료 3행 want, got %v", kinds)
	}
	want := []string{model.WeeklyEventReceipt, model.WeeklyEventAction, model.WeeklyEventComplete}
	for i, k := range want {
		if kinds[i] != k {
			t.Fatalf("순서 %v want %v", kinds, want)
		}
	}
	var prevComplete bool
	for _, ev := range rep.Events {
		if ev.WorkNo == "R2608-010" && ev.Kind == model.WeeklyEventComplete {
			prevComplete = true
		}
	}
	if !prevComplete {
		t.Fatal("지난주 접수건의 이번주 완료 행이 없음")
	}
}
