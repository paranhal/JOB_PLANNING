package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListWorkToday_LimitsToTodayAndDelayed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "work-today-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-wt','오늘도서관','오늘도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-today','R-TODAY','c-wt','2026-08-01',?,1,'assigned','양기헌',NULL,'오늘예정',?),
		('as-done','R-DONE','c-wt','2026-08-01',?,1,'completed','양기헌',?,'오늘완료',?),
		('as-late','R-LATE','c-wt','2026-08-01',?,1,'in_progress','양기헌',NULL,'지연AS',?),
		('as-nodate','R-NODATE','c-wt','2026-08-01',NULL,0,'assigned','양기헌',NULL,'예정없음',?)`,
		today, today+" 09:00:00",
		today, today+" 16:00:00", today+" 16:00:00",
		yesterday, today+" 09:00:00",
		today+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp-wt', 2026, '점검', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (
		visit_id, plan_id, visit_date, customer_id, sort_order, completed, assignee, product_type
	) VALUES
		('mv-today','mp-wt',?,'c-wt',1,0,'양기헌','KLAS'),
		('mv-future','mp-wt',?,'c-wt',2,0,'양기헌','KLAS')`, today, tomorrow); err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정오늘", Status: model.WBTaskWaiting,
		Assignee: "양기헌", DueDate: today, WorkDate: today,
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정내일", Status: model.WBTaskWaiting,
		Assignee: "양기헌", DueDate: tomorrow, WorkDate: tomorrow,
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정지연", Status: model.WBTaskWaiting,
		Assignee: "양기헌", DueDate: yesterday, WorkDate: yesterday,
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정미정", Status: model.WBTaskWaiting,
		Assignee: "양기헌",
	}); err != nil {
		t.Fatal(err)
	}

	repo := NewWorkBoardRepo(db)
	todayItems, err := repo.ListWorkToday("", []string{"양기헌"}, today)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkRef(todayItems, "as-today") || !hasWorkRef(todayItems, "as-done") || !hasWorkRef(todayItems, "as-late") {
		t.Fatalf("오늘 예정·완료·지연 AS 없음: %v", refsOf(todayItems))
	}
	if !hasWorkRef(todayItems, "mv-today") {
		t.Fatalf("오늘 점검 없음: %v", refsOf(todayItems))
	}
	if hasWorkRef(todayItems, "mv-future") {
		t.Fatal("내일 이후 정기점검이 오늘 내 업무에 있다")
	}
	if hasWorkRef(todayItems, "as-nodate") {
		t.Fatal("예정일 없는 AS가 오늘 내 업무에 있다")
	}
	if hasWorkTitle(todayItems, "행정내일") || hasWorkTitle(todayItems, "행정미정") {
		t.Fatal("내일·미정 행정이 오늘 내 업무에 있다")
	}
	if !hasWorkTitle(todayItems, "행정오늘") || !hasWorkTitle(todayItems, "행정지연") {
		t.Fatal("오늘·지연 행정이 없다")
	}
	var lateAdmin *model.WorkListItem
	for i := range todayItems {
		if todayItems[i].Title == "행정지연" {
			lateAdmin = &todayItems[i]
			break
		}
	}
	if lateAdmin == nil || lateAdmin.DaysOverdue < 1 {
		t.Fatalf("행정 지연 뱃지용 경과일=%v", lateAdmin)
	}
	var late *model.WorkListItem
	for i := range todayItems {
		if todayItems[i].RefID == "as-late" {
			late = &todayItems[i]
			break
		}
	}
	if late == nil || late.DaysOverdue < 1 {
		t.Fatalf("지연 뱃지용 경과일=%v", late)
	}

	all, err := repo.ListUnified("", []string{"양기헌"}, false, today, today)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkRef(all, "mv-future") {
		t.Fatalf("ListUnified에 미래점검이 없다: %v", refsOf(all))
	}
	if !hasWorkRef(all, "as-nodate") {
		t.Fatalf("ListUnified에 미정 AS가 없다: %v", refsOf(all))
	}
	if !hasWorkTitle(all, "행정내일") {
		t.Fatalf("ListUnified에 내일 행정이 없다: %v", refsOf(all))
	}

	unplanned, _, err := repo.ListUnplanned("", []string{"양기헌"}, "")
	if err != nil {
		t.Fatal(err)
	}
	foundNoDate := false
	for _, it := range unplanned {
		if it.RefID == "as-nodate" || it.Title == "행정미정" {
			foundNoDate = true
			break
		}
	}
	if !foundNoDate {
		t.Fatal("미계획 업무함에 예정일 없는 건이 없다")
	}
}

func hasWorkTitle(items []model.WorkListItem, title string) bool {
	for _, it := range items {
		if it.Title == title {
			return true
		}
	}
	return false
}
