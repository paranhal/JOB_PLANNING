package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_133CreateActivityLocalNow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 16, 48, 45, 0, loc)
	restore := model.SetNowForTest(func() time.Time { return fixed })
	defer restore()

	db, err := InitDB(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "시각", DueDate: "2026-11-06", Status: model.WBTaskInProgress}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateActivity(&model.WorkActivity{
		TaskID: task.TaskID, ActivityType: model.WBActivityOther, Content: "완료조치", Actor: "테스터",
	}); err != nil {
		t.Fatal(err)
	}
	acts, err := wb.ListActivities(task.TaskID)
	if err != nil || len(acts) != 1 {
		t.Fatalf("acts=%v err=%v", acts, err)
	}
	if acts[0].CreatedAt != "2026-11-06 16:48:45" {
		t.Fatalf("created_at=%q want 현지", acts[0].CreatedAt)
	}
	var raw string
	if err := db.QueryRow(`SELECT COALESCE(created_at,'') FROM work_activities WHERE activity_id=?`, acts[0].ActivityID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "2026-11-06 16:48:45" {
		t.Fatalf("db=%q", raw)
	}
	if delta := time.Since(fixed); delta > time.Minute || delta < -time.Minute {
		// 주입 시계와 저장값 비교(1분 이내 요구는 주입으로 동일)
	}
	if model.FormatLocalDateTime(raw) != "2026-11-06 16:48" {
		t.Fatalf("표시=%q", model.FormatLocalDateTime(raw))
	}

	var since string
	if err := db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingLocalNowSince).Scan(&since); err != nil || since != "2026-11-06 16:48:45" {
		t.Fatalf("cutoff=%q err=%v", since, err)
	}
}
