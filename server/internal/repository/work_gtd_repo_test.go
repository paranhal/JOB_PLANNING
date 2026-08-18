package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestWorkGTDActionsAndCompleteBlock(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "gtd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)

	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "실적보고", DueDate: "2026-08-20", Status: model.WBTaskInProgress}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}

	a := &model.WorkAction{TaskID: task.TaskID, Title: "엑셀 취합", Status: model.WBActionTodo, Required: true}
	if err := wb.CreateAction(a); err != nil {
		t.Fatal(err)
	}
	if a.ActionID == "" {
		t.Fatal("action id")
	}

	open, wait, err := wb.AdminCompleteBlockers(task.TaskID)
	if err != nil || open != 1 || wait != 0 {
		t.Fatalf("blockers open=%d wait=%d err=%v", open, wait, err)
	}

	a.Status = model.WBActionComplete
	if err := wb.UpdateAction(a); err != nil {
		t.Fatal(err)
	}
	open, _, err = wb.AdminCompleteBlockers(task.TaskID)
	if err != nil || open != 0 {
		t.Fatalf("after complete open=%d err=%v", open, err)
	}

	if err := wb.CreateActivity(&model.WorkActivity{
		TaskID: task.TaskID, ActivityType: model.WBActivityWrite, Content: "초안 작성", Actor: "테스터",
	}); err != nil {
		t.Fatal(err)
	}
	acts, err := wb.ListActivities(task.TaskID)
	if err != nil || len(acts) != 1 || acts[0].Content != "초안 작성" {
		t.Fatalf("activities %+v err=%v", acts, err)
	}
}

func TestInboxExcludedFromPalette(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "gtd_inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)

	inbox := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "빠른메모", Status: model.WBTaskInbox}
	if err := wb.CreateTask(inbox); err != nil {
		t.Fatal(err)
	}
	ready := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "할 일", Status: model.WBTaskWaiting, DueDate: "2026-08-20"}
	if err := wb.CreateTask(ready); err != nil {
		t.Fatal(err)
	}

	items, err := wb.ListUnplacedAdminTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "할 일" {
		t.Fatalf("palette %+v", items)
	}

	listed, err := wb.ListAdminWork("inbox", "")
	if err != nil || len(listed) != 1 {
		t.Fatalf("inbox list len=%d err=%v", len(listed), err)
	}
	st, err := wb.CountAdminWorkStats()
	if err != nil || st.Inbox != 1 || st.Waiting != 1 {
		t.Fatalf("stats %+v err=%v", st, err)
	}

	hold := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "보류건", Status: model.WBTaskHold,
		HoldReason: "일정", ReviewDate: "2026-08-30", DueDate: "2026-08-20",
	}
	if err := wb.CreateTask(hold); err != nil {
		t.Fatal(err)
	}
	xfer := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "이관건", Status: model.WBTaskTransfer, DueDate: "2026-08-20",
	}
	if err := wb.CreateTask(xfer); err != nil {
		t.Fatal(err)
	}
	st, err = wb.CountAdminWorkStats()
	if err != nil || st.Hold != 1 || st.Transfer != 1 {
		t.Fatalf("hold/transfer stats %+v err=%v", st, err)
	}
	holds, err := wb.ListAdminWork("hold", "")
	if err != nil || len(holds) != 1 || holds[0].Title != "보류건" {
		t.Fatalf("hold list %+v err=%v", holds, err)
	}
}

func TestSyncAdminActionProgressAndResume(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "gtd_prog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "복합", DueDate: "2026-08-20", Status: model.WBTaskWaitingFor}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	a := &model.WorkAction{TaskID: task.TaskID, Title: "A", Required: true, Status: model.WBActionWaiting,
		WaitParty: "업체", WaitRequest: "자료", ReplyDueDate: "2026-08-20"}
	if err := wb.CreateAction(a); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncAdminActionProgress(task.TaskID); err != nil {
		t.Fatal(err)
	}
	got, _ := wb.GetTask(task.TaskID)
	if got.Progress != 0 {
		t.Fatalf("0/1 progress=%d", got.Progress)
	}
	a.Status = model.WBActionComplete
	a.Confirmed = true
	if err := wb.UpdateAction(a); err != nil {
		t.Fatal(err)
	}
	if err := wb.SyncAdminActionProgress(task.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := wb.ResumeInProgressIfReady(task.TaskID); err != nil {
		t.Fatal(err)
	}
	got, _ = wb.GetTask(task.TaskID)
	if got.Progress != 100 {
		t.Fatalf("1/1 progress=%d", got.Progress)
	}
	if got.Status != model.WBTaskInProgress {
		t.Fatalf("status=%s want in_progress", got.Status)
	}
}
