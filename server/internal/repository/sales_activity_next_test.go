package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesActivityNextActionCreatesSecondTask(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_next.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "다음 할 일", IsTentativeName: true}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	nextDay := time.Now().AddDate(0, 0, 10).Format("2006-01-02")

	act := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "10:00",
		DurationMin: 30, ActivityType: "visit", Title: "방문",
		OurMembers: "최혜영", NextAction: "제안 방안 협의", NextActionDate: nextDay,
	}
	if err := sales.CreateActivity(act, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	if n := countSourceTasks(t, db, act.ActivityID); n != 2 {
		t.Fatalf("업무 %d건 want 2", n)
	}
	done, _ := wb.GetTaskBySource(model.WBSourceSalesActivity, act.ActivityID, model.WBSourceRoleDone)
	nxt, _ := wb.GetTaskBySource(model.WBSourceSalesActivity, act.ActivityID, model.WBSourceRoleNext)
	if done == nil || nxt == nil {
		t.Fatalf("done=%v next=%v", done, nxt)
	}
	if done.Status != model.WBTaskComplete || nxt.Status != model.WBTaskWaiting {
		t.Fatalf("상태 done=%s next=%s", done.Status, nxt.Status)
	}
	if nxt.DueDate != nextDay || nxt.WorkDate != nextDay {
		t.Fatalf("다음 예정일 due=%s work=%s", nxt.DueDate, nxt.WorkDate)
	}
	if done.WorkType != model.WBWorkSales || model.WBWorkTypeLabel(done.WorkType) != "영업활동" {
		t.Fatalf("구분 %s / %s", done.WorkType, model.WBWorkTypeLabel(done.WorkType))
	}
	badge := model.WBCategoryLabel(model.WBCategory(done.SourceType))
	if !containsSalesWord(badge) || model.WBWorkTypeLabel(done.WorkType) == "행정업무" {
		t.Fatalf("배지 %q 상세 %q", badge, model.WBWorkTypeLabel(done.WorkType))
	}

	monthAct := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "11:00",
		DurationMin: 30, ActivityType: "visit", Title: "월만",
		OurMembers: "최혜영", NextAction: "다음달 협의", NextActionDate: "2026-10",
	}
	if err := sales.CreateActivity(monthAct, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	if n := countSourceTasks(t, db, monthAct.ActivityID); n != 1 {
		t.Fatalf("월만 날짜 업무 %d건 want 1", n)
	}

	act.NextAction = ""
	act.NextActionDate = ""
	act.Title = "방문"
	if err := sales.UpdateActivity(act); err != nil {
		t.Fatal(err)
	}
	if n := countSourceTasks(t, db, act.ActivityID); n != 1 {
		t.Fatalf("다음 지운 뒤 %d건", n)
	}

	keep := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "14:00",
		DurationMin: 30, ActivityType: "visit", Title: "완료 유지",
		OurMembers: "최혜영", NextAction: "견적 요청", NextActionDate: nextDay,
	}
	if err := sales.CreateActivity(keep, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	nxt2, _ := wb.GetTaskBySource(model.WBSourceSalesActivity, keep.ActivityID, model.WBSourceRoleNext)
	if nxt2 == nil {
		t.Fatal("다음 업무 없음")
	}
	nxt2.Status = model.WBTaskComplete
	nxt2.Progress = 100
	nxt2.CompleteDate = today
	if err := wb.UpdateTask(nxt2); err != nil {
		t.Fatal(err)
	}
	keep.NextAction = ""
	keep.NextActionDate = ""
	keep.Title = "완료 유지"
	if err := sales.UpdateActivity(keep); err != nil {
		t.Fatal(err)
	}
	still, _ := wb.GetTaskBySource(model.WBSourceSalesActivity, keep.ActivityID, model.WBSourceRoleNext)
	if still == nil || still.Status != model.WBTaskComplete {
		t.Fatalf("완료된 다음 업무가 지워졌다 %+v", still)
	}
}

func countSourceTasks(t *testing.T, db *sql.DB, activityID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`,
		model.WBSourceSalesActivity, activityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func containsSalesWord(s string) bool {
	return len(s) >= 2 && (s == "영업" || s == "영업활동")
}
