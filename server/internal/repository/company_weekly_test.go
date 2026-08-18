package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeeklyPeriodAugust2ndWeek(t *testing.T) {
	p := CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	if p.Anchor != "2026-08-17" || p.PrevFrom != "2026-08-10" || p.PrevTo != "2026-08-16" {
		t.Fatalf("전주=%s~%s anchor=%s", p.PrevFrom, p.PrevTo, p.Anchor)
	}
	if p.ThisFrom != "2026-08-17" || p.ThisTo != "2026-08-23" {
		t.Fatalf("금주=%s~%s", p.ThisFrom, p.ThisTo)
	}
	if p.WeekN != 2 || p.SheetNameDefault != "26.8월2주(업무)" {
		t.Fatalf("시트이름=%q weekN=%d want 26.8월2주(업무)", p.SheetNameDefault, p.WeekN)
	}
	if p.PrevRangeLabel != "8.10(월)~8.16(일)" || p.ThisRangeLabel != "8.17(월)~8.23(일)" {
		t.Fatalf("기간라벨 prev=%q this=%q", p.PrevRangeLabel, p.ThisRangeLabel)
	}

	july := CompanyWeeklyPeriod(time.Date(2026, 7, 27, 0, 0, 0, 0, time.Local))
	if july.PrevFrom != "2026-07-20" || july.WeekN != 3 || july.SheetNameDefault != "26.7월3주(업무)" {
		t.Fatalf("7월 전주월요일 주차: from=%s n=%d name=%s", july.PrevFrom, july.WeekN, july.SheetNameDefault)
	}
}

func TestCompanyWeeklyDraftAugust2CompleteBaseline(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp-cw', 2033, 't', 'draft')`); err != nil {
		t.Fatal(err)
	}

	insertCust := func(id string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
			VALUES (?,?,?,1)`, id, id, id); err != nil {
			t.Fatal(err)
		}
	}
	insertMnt := func(id, cust, date, pid string, done bool) {
		t.Helper()
		comp, cdate := 0, ""
		if done {
			comp, cdate = 1, date
		}
		if _, err := db.Exec(`
			INSERT INTO maintenance_visits
			(visit_id, plan_id, visit_date, customer_id, product_type, completed, completed_date, project_id)
			VALUES (?,?,?,?,?,?,?,?)`, id, "mp-cw", date, cust, "KLAS", comp, nullStr(cdate), nullStr(pid)); err != nil {
			t.Fatal(err)
		}
	}
	insertAS := func(id, cust, complete, pid string) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, complete_datetime, status, project_id)
			VALUES (?,?,?,?,?,'completed',?)`, id, id, cust, complete, complete, nullStr(pid)); err != nil {
			t.Fatal(err)
		}
	}

	for i := 1; i <= 5; i++ {
		cid := fmt.Sprintf("CN-%d", i)
		insertCust(cid)
		insertMnt(fmt.Sprintf("V-CN-%d", i), cid, fmt.Sprintf("2026-08-1%d", i), "WPSEED01", true)
		insertAS(fmt.Sprintf("R-CN-%d", i), cid, fmt.Sprintf("2026-08-1%d 14:00:00", i), "WPSEED01")
	}
	for i := 1; i <= 2; i++ {
		cid := fmt.Sprintf("SJ-%d", i)
		insertCust(cid)
		insertMnt(fmt.Sprintf("V-SJ-%d", i), cid, fmt.Sprintf("2026-08-1%d", i), "WPSEED02", true)
	}
	for i := 1; i <= 6; i++ {
		cid := fmt.Sprintf("AN-%d", i)
		insertCust(cid)
		d := fmt.Sprintf("2026-08-1%d", ((i-1)%6)+0)
		if i <= 6 {
			d = []string{"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14", "2026-08-15"}[i-1]
		}
		insertMnt(fmt.Sprintf("V-AN-%d", i), cid, d, "WPSEED03", true)
	}
	for i := 1; i <= 4; i++ {
		insertAS(fmt.Sprintf("R-AN-%d", i), fmt.Sprintf("AN-%d", i), fmt.Sprintf("2026-08-1%d 10:00:00", i), "WPSEED03")
	}

	insertCust("FREE")
	insertMnt("V-FREE", "FREE", "2026-08-12", "", true)
	insertCust("U05")
	insertMnt("V-U05", "U05", "2026-08-13", "WPSEED05", true)

	insertCust("PLAN-CN")
	insertMnt("V-PLAN", "PLAN-CN", "2026-08-18", "WPSEED01", false)

	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, project_id, title, status, complete_date, due_date)
		VALUES
		('WT-done','admin','WPSEED01','계약변경 회신','complete','2026-08-12','2026-08-12'),
		('WT-plan','admin','WPSEED01','현장점검 준비','waiting','','2026-08-19'),
		('WT-carry','admin','WPSEED01','세금계산서','waiting','','2026-08-14'),
		('WT-free','admin','WPSEED05','무상 점검 협의','complete','2026-08-11','2026-08-11')`); err != nil {
		t.Fatal(err)
	}

	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraft(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Rows) != 6 {
		t.Fatalf("행 수=%d want 6", len(draft.Rows))
	}
	byKey := map[string]model.CompanyWeeklyRow{}
	for _, row := range draft.Rows {
		byKey[row.RowKey] = row
	}
	if byKey["401"].SheetRow != 20 || byKey["403"].SheetRow != 22 || byKey["X01"].SheetRow != 24 {
		t.Fatalf("sheet_row 매핑: 401=%d 403=%d X01=%d", byKey["401"].SheetRow, byKey["403"].SheetRow, byKey["X01"].SheetRow)
	}
	if byKey["401"].MntDoneSites != 5 || byKey["401"].ASDoneCount != 5 {
		t.Fatalf("충남 F20 완료 기준 mnt=%d as=%d want 5/5", byKey["401"].MntDoneSites, byKey["401"].ASDoneCount)
	}
	if byKey["403"].MntDoneSites != 2 {
		t.Fatalf("세종 F22 정기점검=%d want 2", byKey["403"].MntDoneSites)
	}
	if byKey["X01"].MntDoneSites != 6 || byKey["X01"].ASDoneCount != 4 {
		t.Fatalf("앤로 F24 mnt=%d as=%d want 6/4", byKey["X01"].MntDoneSites, byKey["X01"].ASDoneCount)
	}
	wantF20 := "·정기점검 5사이트, AS 5건 처리완료\n·계약변경 회신(8/12)"
	if byKey["401"].PrevText != wantF20 {
		t.Fatalf("F20=%q want %q", byKey["401"].PrevText, wantF20)
	}
	if byKey["403"].PrevText != "·정기점검 2사이트" {
		t.Fatalf("F22=%q", byKey["403"].PrevText)
	}
	if byKey["X01"].PrevText != "·정기점검 6사이트, AS 4건 처리완료" {
		t.Fatalf("F24=%q", byKey["X01"].PrevText)
	}
	if byKey["402"].PrevText != "" || byKey["404"].PrevText != "" || byKey["X02"].PrevText != "" {
		t.Fatalf("실적 없는 행은 비워야 함 402=%q 404=%q X02=%q", byKey["402"].PrevText, byKey["404"].PrevText, byKey["X02"].PrevText)
	}
	if !byKey["404"].MissingProject {
		t.Fatal("404행은 대응 사업 없음으로 표시")
	}
	if !strings.Contains(byKey["401"].PlanText, "·정기점검 1사이트") || !strings.Contains(byKey["401"].PlanText, "·현장점검 준비") {
		t.Fatalf("G20 금주=%q", byKey["401"].PlanText)
	}
	if !strings.Contains(byKey["401"].PlanText, "·세금계산서(이월)") {
		t.Fatalf("이월분이 없다: %q", byKey["401"].PlanText)
	}

	if len(draft.Unassigned) == 0 {
		t.Fatal("미배정 활동이 비었다")
	}
	var sawFree, sawU05, sawAdmin bool
	for _, u := range draft.Unassigned {
		if u.ID == "V-FREE" {
			sawFree = true
		}
		if u.ID == "V-U05" || u.ProjectID == "WPSEED05" {
			sawU05 = true
		}
		if u.ID == "WT-free" {
			sawAdmin = true
		}
	}
	if !sawFree || !sawU05 || !sawAdmin {
		t.Fatalf("미배정 누락 free=%v wp05=%v admin=%v n=%d", sawFree, sawU05, sawAdmin, len(draft.Unassigned))
	}
}

func TestWeeklyReportRowsSeeded(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "rows.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM weekly_report_rows`).Scan(&n); err != nil || n != 6 {
		t.Fatalf("초기 행=%d err=%v want 6", n, err)
	}
	applyWeeklyReportRows(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM weekly_report_rows`).Scan(&n); err != nil || n != 6 {
		t.Fatalf("시드 재실행 후=%d", n)
	}
}
