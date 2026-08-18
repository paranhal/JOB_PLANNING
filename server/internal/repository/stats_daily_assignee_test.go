package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestBuildDailyAssigneeReportHolidayGrayAndWorkingTotal(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "daily.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-08-12', 2026, '임시휴일', 'company')`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime,
			complete_datetime, status, assigned_to, urgency, priority
		) VALUES
		('a1','R1','c1','2026-08-10 09:00:00','2026-08-10 12:00:00','completed','양기헌','high','high'),
		('a2','R2','c1','2026-08-11 09:00:00','2026-08-11 15:00:00','completed','양기헌','normal','normal'),
		('a3','R3','c1','2026-08-12 09:00:00','2026-08-12 11:00:00','completed','최혜경','normal','normal')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_processes (process_id, as_id, process_datetime, worker, time_spent)
		VALUES ('p1','a1','2026-08-10 10:00:00','양기헌',40),
		       ('p2','a2','2026-08-11 10:00:00','양기헌',20)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	rep, err := repo.BuildDailyAssigneeReport("2026-08-10", "2026-08-17", "20260814")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Days) != 7 {
		t.Fatalf("days=%d want 7", len(rep.Days))
	}
	wed := -1
	thu := -1
	for i, d := range rep.Days {
		if d.Date == "2026-08-12" {
			wed = i
			if !d.Off {
				t.Fatal("공휴일 열이 회색(Off)이 아님")
			}
		}
		if d.Date == "2026-08-13" {
			thu = i
			if d.Off {
				t.Fatal("평일 목이 휴무로 잡힘")
			}
		}
		if d.Date == "2026-08-15" && !d.Off { // 토
			t.Fatal("토요일이 Off가 아님")
		}
	}
	if wed < 0 || thu < 0 {
		t.Fatal("수·목 열 없음")
	}
	if rep.WorkingDays != 4 { // 월~금 5 − 수 공휴일
		t.Fatalf("워킹데이=%d want 4", rep.WorkingDays)
	}

	var yang *model.DailyAssigneePerson
	for i := range rep.People {
		if rep.People[i].Label == "양기헌" {
			yang = &rep.People[i]
		}
	}
	if yang == nil {
		t.Fatalf("양기헌 행 없음: %+v", labelsOf(rep.People))
	}
	if yang.Counts[0] != 1 || yang.Counts[1] != 1 {
		t.Fatalf("양기헌 월·화 건수=%v", yang.Counts)
	}
	if yang.CountTotal != 2 {
		t.Fatalf("양기헌 합계=%d want 2 (공휴일 제외)", yang.CountTotal)
	}
	if !yang.HasCountAvg || yang.CountAvg != 0.5 {
		t.Fatalf("양기헌 일평균=%v has=%v want 0.5", yang.CountAvg, yang.HasCountAvg)
	}
	if yang.Counts[thu] != 0 {
		t.Fatalf("목 0이어야 함: %d", yang.Counts[thu])
	}
	if yang.Minutes[0] != 40 || yang.MinuteTotal != 60 {
		t.Fatalf("양기헌 분=%v total=%d", yang.Minutes, yang.MinuteTotal)
	}
	if rep.TeamCounts[wed] != 1 {
		t.Fatalf("수 팀 합계(표시)=%d want 1", rep.TeamCounts[wed])
	}
	if rep.TeamCountTotal != 2 {
		t.Fatalf("팀 합계(워킹데이)=%d want 2", rep.TeamCountTotal)
	}

	hasTotal := false
	for _, d := range rep.Details {
		if d.IsDayTotal && d.Assignee == "양기헌" && d.Date == "2026-08-10" {
			hasTotal = true
			if d.Kind != model.DailyDetailKindTotal {
				t.Fatalf("합계 행 구분=%q", d.Kind)
			}
		}
	}
	if !hasTotal {
		t.Fatal("그날 합계 행 없음")
	}
}

func labelsOf(ps []model.DailyAssigneePerson) []string {
	var s []string
	for _, p := range ps {
		s = append(s, p.Label)
	}
	return s
}

func TestBuildDailyAssigneeReportLeaveMarkAndDenom(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "leaveavg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime,
			complete_datetime, status, assigned_to, urgency, priority
		) VALUES
		('a1','R1','c1','2026-08-10 09:00:00','2026-08-10 12:00:00','completed','양기헌','high','high'),
		('a2','R2','c1','2026-08-11 09:00:00','2026-08-11 15:00:00','completed','양기헌','normal','normal')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewStaffLeaveRepo(db).Create(model.StaffLeave{
		UserID: u.UserID, UserName: u.FullName, Date: "2026-08-13", Kind: model.LeaveKindAnnual,
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := NewStatsRepo(db).BuildDailyAssigneeReport("2026-08-10", "2026-08-15", "20260814")
	if err != nil {
		t.Fatal(err)
	}
	// 월10 화11 수12 목13 금14. 수·토 없음(toEx=15). 워킹데이 5. 목 연차 → 개인 분모 4.
	if rep.WorkingDays != 5 {
		t.Fatalf("팀 워킹데이=%d want 5", rep.WorkingDays)
	}
	var yang *model.DailyAssigneePerson
	for i := range rep.People {
		if rep.People[i].Label == "양기헌" {
			yang = &rep.People[i]
		}
	}
	if yang == nil {
		t.Fatalf("양기헌 행 없음: %+v", labelsOf(rep.People))
	}
	thu := -1
	for i, d := range rep.Days {
		if d.Date == "2026-08-13" {
			thu = i
		}
	}
	if thu < 0 {
		t.Fatal("목 열 없음")
	}
	if yang.LeaveMarks[thu] != "연차" {
		t.Fatalf("연차 칸=%q", yang.LeaveMarks[thu])
	}
	if yang.CountTotal != 2 {
		t.Fatalf("합계=%d", yang.CountTotal)
	}
	if !yang.HasCountAvg || yang.CountAvg != 0.5 {
		t.Fatalf("일평균=%v has=%v want 0.5 (2÷4실근무일)", yang.CountAvg, yang.HasCountAvg)
	}
}
