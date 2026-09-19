package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesActivityMarksWorkTaskComplete(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_act_complete.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "활동 완료 연동", IsTentativeName: true}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	todayAct := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "10:00",
		DurationMin: 60, ActivityType: "visit", Title: "오늘 방문",
		OurMembers: "최혜영",
	}
	if err := sales.CreateActivity(todayAct, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetTaskBySource(model.WBSourceSalesActivity, todayAct.ActivityID)
	if err != nil || got == nil {
		t.Fatalf("오늘 활동 업무 없음: err=%v", err)
	}
	if got.Status != model.WBTaskComplete || got.Progress != 100 || got.CompleteDate != today {
		t.Fatalf("오늘 활동: status=%s progress=%d complete=%s", got.Status, got.Progress, got.CompleteDate)
	}

	futureAct := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: future, StartTime: "11:00",
		DurationMin: 30, ActivityType: "visit", Title: "예정 방문",
		OurMembers: "최혜영",
	}
	if err := sales.CreateActivity(futureAct, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	waiting, err := wb.GetTaskBySource(model.WBSourceSalesActivity, futureAct.ActivityID)
	if err != nil || waiting == nil {
		t.Fatalf("미래 활동 업무 없음: err=%v", err)
	}
	if waiting.Status != model.WBTaskWaiting || waiting.CompleteDate != "" {
		t.Fatalf("미래 활동: status=%s complete=%s", waiting.Status, waiting.CompleteDate)
	}

	futureAct.ActivityDate = today
	futureAct.Title = "예정 방문"
	if err := sales.UpdateActivity(futureAct); err != nil {
		t.Fatal(err)
	}
	moved, err := wb.GetTaskBySource(model.WBSourceSalesActivity, futureAct.ActivityID)
	if err != nil || moved == nil {
		t.Fatalf("날짜 수정 후 업무 없음: err=%v", err)
	}
	if moved.Status != model.WBTaskComplete || moved.Progress != 100 || moved.CompleteDate != today {
		t.Fatalf("오늘로 옮긴 뒤: status=%s progress=%d complete=%s", moved.Status, moved.Progress, moved.CompleteDate)
	}

	manualAct := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: future, StartTime: "14:00",
		DurationMin: 30, ActivityType: "visit", Title: "진행중 유지",
		OurMembers: "최혜영",
	}
	if err := sales.CreateActivity(manualAct, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	manual, err := wb.GetTaskBySource(model.WBSourceSalesActivity, manualAct.ActivityID)
	if err != nil || manual == nil {
		t.Fatalf("수동 상태 업무 없음: err=%v", err)
	}
	manual.Status = model.WBTaskInProgress
	manual.Progress = 40
	if err := wb.UpdateTask(manual); err != nil {
		t.Fatal(err)
	}
	manualAct.Title = "진행중 유지"
	manualAct.Content = "메모"
	if err := sales.UpdateActivity(manualAct); err != nil {
		t.Fatal(err)
	}
	kept, err := wb.GetTaskBySource(model.WBSourceSalesActivity, manualAct.ActivityID)
	if err != nil || kept == nil {
		t.Fatalf("수정 후 업무 없음: err=%v", err)
	}
	if kept.Status != model.WBTaskInProgress {
		t.Fatalf("사람이 고른 상태가 덮였다 status=%s", kept.Status)
	}
}
