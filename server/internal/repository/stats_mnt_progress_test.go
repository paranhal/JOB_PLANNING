package repository

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestMntMonthProgressWeekCumulative(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "mnt_prog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 73곳 KLAS만 + 1곳 KLAS+RFID = 방문 목표 75건 (8월·월 주기)
	for i := 0; i < 73; i++ {
		id := fmt.Sprintf("c%03d", i)
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
			id, "기관"+id, "기관"+id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name, has_klas, has_rfid, inspection_cycle)
			VALUES (?,?,1,0,'monthly')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c_both','둘다','둘다',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name, has_klas, has_rfid, inspection_cycle)
		VALUES ('c_both','둘다',1,1,'monthly')`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('p1',2026,'t','draft')`); err != nil {
		t.Fatal(err)
	}

	insertDone := func(id, customer, visitDate string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO maintenance_visits
			(visit_id, plan_id, visit_date, customer_id, completed, completed_date, product_type)
			VALUES (?,?,?,?,1,?,?)`, id, "p1", visitDate, customer, visitDate, "KLAS")
		if err != nil {
			t.Fatal(err)
		}
	}
	// 8월 1주(3~9일) 10건, 2주(10~16일) 11건
	for i := 0; i < 10; i++ {
		insertDone(fmt.Sprintf("w1_%d", i), fmt.Sprintf("c%03d", i), "2026-08-04")
	}
	for i := 0; i < 11; i++ {
		insertDone(fmt.Sprintf("w2_%d", i), fmt.Sprintf("c%03d", i+10), "2026-08-11")
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local) // 금주 = 8/3~8/9
	cols := buildStatsPeriodColumns(model.StatsViewWeek, anchor, anchor)
	if err := repo.FillPeriodOverview(cols, ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatal(err)
	}
	gold := cols[1].Counts.Mnt
	if gold.Quota != 75 {
		t.Fatalf("금주 월목표=%d want 75", gold.Quota)
	}
	if gold.Planned != 10 || gold.Receipt != 10 || gold.Process != 10 {
		t.Fatalf("금주 planned/receipt/process=%d/%d/%d want 10/10/10", gold.Planned, gold.Receipt, gold.Process)
	}
	if gold.Cumulative != 10 || gold.Remaining != 65 {
		t.Fatalf("금주 누적/남은=%d/%d want 10/65", gold.Cumulative, gold.Remaining)
	}

	anchor2 := time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local) // 금주 = 8/10~8/16
	cols2 := buildStatsPeriodColumns(model.StatsViewWeek, anchor2, anchor2)
	if err := repo.FillPeriodOverview(cols2, ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatal(err)
	}
	w2 := cols2[1].Counts.Mnt
	if w2.Planned != 11 || w2.Process != 11 {
		t.Fatalf("2주 planned/process=%d/%d want 11", w2.Planned, w2.Process)
	}
	if w2.Cumulative != 21 || w2.Remaining != 54 {
		t.Fatalf("2주 누적/남은=%d/%d want 21/54", w2.Cumulative, w2.Remaining)
	}
	prev := cols2[0].Counts.Mnt
	if prev.Process != 10 || prev.Cumulative != 10 || prev.Remaining != 65 {
		t.Fatalf("전주 process/누적/남은=%d/%d/%d want 10/10/65", prev.Process, prev.Cumulative, prev.Remaining)
	}

	rep, err := repo.BuildWeeklyReport(anchor2)
	if err != nil {
		t.Fatal(err)
	}
	if rep.MntReceipt != 11 || rep.MntCumulative != 21 || rep.MntRemaining != 54 || rep.MntQuota != 75 {
		t.Fatalf("보고서 이번주 receipt=%d 누적=%d 남은=%d 목표=%d",
			rep.MntReceipt, rep.MntCumulative, rep.MntRemaining, rep.MntQuota)
	}
}

func TestCountMonthMntQuotaDualProduct(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "quota.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','A','A',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name, has_klas, has_rfid, inspection_cycle)
		VALUES ('c1','A',1,1,'monthly')`); err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	n, err := repo.CountMonthMntQuota(2026, 8, ParseMeetingFilter(model.StatsScopeTeam, "", ""))
	if err != nil || n != 2 {
		t.Fatalf("quota=%d err=%v want 2", n, err)
	}
}
