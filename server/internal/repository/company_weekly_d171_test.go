package repository

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeeklyD171ContentRules(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "d171.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('mp-d171', 2033, 't', 'draft')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-OPEN','열린도서관','열린',1),('C-INC','미완료관','미완료',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, product_type, completed, completed_date, project_id)
		VALUES ('V-INC','mp-d171','2026-08-12','C-INC','KLAS',0,'','WPSEED01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, complete_datetime, status, project_id, symptom)
		VALUES ('R-OPEN','R-OPEN','C-OPEN','2026-08-11 10:00:00','','processing','WPSEED01','리더기 무응답')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, project_id, title, status, complete_date, due_date, work_date, source_type)
		VALUES
		('WT-old','admin','WPSEED01','오래된 보고서','waiting','','2026-07-20','',''),
		('WT-future','admin','WPSEED01','먼 미래 작업','waiting','','2026-09-10','',''),
		('WT-next','admin','WPSEED01','차주 작업','waiting','','2026-08-19','',''),
		('WT-sales','admin','WPSEED01','영업 미팅','complete','2026-08-12','2026-08-12','2026-08-12','sales_activity'),
		('WT-work','work','WPSEED01','구축공사','complete','2026-08-13','2026-08-13','2026-08-13',''),
		('WT-pw','project_work','WPSEED01','현장 구축','complete','2026-08-13','2026-08-13','2026-08-13','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sales_projects (sales_id, name, stage, status) VALUES ('S-MAP','매핑영업','propose','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET sales_project_id='S-MAP' WHERE project_id='WPSEED01'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sales_activities (activity_id, sales_id, activity_date, activity_type, title)
		VALUES ('SA-MAP','S-MAP','2026-08-12','proposal','매핑된 제안')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, next_action_date, work_content, time_spent) VALUES ('P1','R-OPEN','2026-08-19','후속',30)`); err != nil {
		t.Fatal(err)
	}

	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraft(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]model.CompanyWeeklyRow{}
	for _, row := range draft.Rows {
		byKey[row.RowKey] = row
	}
	prev := byKey["401"].PrevText
	plan := byKey["401"].PlanText
	if !strings.Contains(prev, "AS 진행") || !strings.Contains(prev, "열린도서관") {
		t.Fatalf("진행 중 AS 없음: %q", prev)
	}
	if !strings.Contains(prev, "구축공사") && !strings.Contains(prev, "현장 구축") {
		t.Fatalf("work/project_work 유형 없음: %q", prev)
	}
	if strings.Contains(prev, "영업 미팅") {
		t.Fatalf("영업이 사업 카드에 있음: %q", prev)
	}
	if !strings.Contains(plan, "미완료관") || !strings.Contains(plan, "(이월)") {
		t.Fatalf("미완료 점검 이월 없음: %q", plan)
	}
	if !strings.Contains(plan, "차주 작업") {
		t.Fatalf("차주 계획 없음: %q", plan)
	}
	if !strings.Contains(plan, "오래된 보고서") || !strings.Contains(plan, "주 지연") {
		t.Fatalf("오래된 이월 없음: %q", plan)
	}
	if strings.Contains(plan, "먼 미래") {
		t.Fatalf("먼 미래가 계획에 있음: %q", plan)
	}
	if !strings.Contains(plan, "AS 후속") {
		t.Fatalf("추후 조치 예정일 없음: %q", plan)
	}
	if !strings.Contains(byKey["X03"].PrevText, "매핑된 제안") {
		t.Fatalf("매핑 영업이 영업 행에 없음: %q", byKey["X03"].PrevText)
	}
}

func TestCompanyWeeklyO01GoldenAllowlistedDiffOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "gold.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedAugust2Baseline(t, db)
	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraft(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]model.CompanyWeeklyRow{}
	for _, row := range draft.Rows {
		byKey[row.RowKey] = row
	}
	before := map[string]string{
		"401": "·정기점검 5사이트(CN-1·CN-2·CN-3·CN-4·CN-5), AS 5건 처리완료\n·계약변경 회신(8/12)",
		"403": "·정기점검 2사이트(SJ-1·SJ-2)",
		"X01": "·정기점검 6사이트(AN-1·AN-2·AN-3·AN-4·AN-5 외 1곳), AS 4건 처리완료",
		"402": "",
		"404": "",
		"X02": "",
	}
	for k, want := range before {
		got := byKey[k].PrevText
		if got == want {
			continue
		}
		if !d171AllowlistedDiff(want, got) {
			t.Fatalf("%s 5건 외 차이\n전=%q\n후=%q", k, want, got)
		}
	}
	if !strings.Contains(byKey["401"].PlanText, "·세금계산서(이월)") {
		t.Fatalf("이월 유지 실패 %q", byKey["401"].PlanText)
	}
}

func d171AllowlistedDiff(before, after string) bool {
	if after == before {
		return true
	}
	if strings.Contains(after, "AS 진행") && strings.HasPrefix(after, before+"\n") {
		return true
	}
	if strings.Contains(after, "(이월)") || strings.Contains(after, "주 지연") {
		core := strings.ReplaceAll(after, before, "")
		if strings.Contains(core, "AS 진행") || strings.Contains(core, "이월") || strings.Contains(core, "지연") {
			return strings.Contains(after, strings.Split(before, "\n")[0])
		}
	}
	return false
}

func seedAugust2Baseline(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('mp-cw', 2033, 't', 'draft')`); err != nil {
		t.Fatal(err)
	}
	insertCust := func(id string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`, id, id, id); err != nil {
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
		d := []string{"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14", "2026-08-15"}[i-1]
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
}

func TestMergeWeeklyLinesHideAndRestore(t *testing.T) {
	auto := []CompanyWeeklyLine{{Key: "task:1", Text: "자동A"}, {Key: "task:2", Text: "자동B"}}
	saved := []CompanyWeeklyLine{{Key: "task:1", Text: "고친A", Manual: true}}
	got := MergeWeeklyLinesKeyed(auto, saved, []string{"task:2"})
	if len(got) != 1 || !got[0].Manual || got[0].Text != "고친A" {
		t.Fatalf("%+v", got)
	}
	restored := MergeWeeklyLinesKeyed(auto, saved, nil)
	if len(restored) != 2 {
		t.Fatalf("복원 %+v", restored)
	}
}
