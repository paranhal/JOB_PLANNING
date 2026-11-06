package handler

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_127_CompleteWithoutProcessInActView(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d127c.db"))
	ck := jwtCookieUser(t, me)
	day := "2026-11-05"
	outside := "2026-10-01"

	admin := &model.WorkTask{
		Title: "완료일만127", WorkDate: day, DueDate: day, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskComplete,
		WorkType: model.WBWorkAdmin, CompleteDate: day, StartTime: "09:00", EndTime: "09:30",
	}
	if err := repo.CreateTask(admin); err != nil {
		t.Fatal(err)
	}
	out := &model.WorkTask{
		Title: "기간밖완료127", WorkDate: outside, DueDate: outside, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskComplete,
		WorkType: model.WBWorkAdmin, CompleteDate: outside,
	}
	if err := repo.CreateTask(out); err != nil {
		t.Fatal(err)
	}
	canc := &model.WorkTask{
		Title: "취소127", WorkDate: day, DueDate: day, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskCancelled,
		WorkType: model.WBWorkAdmin, CancelReason: "중복",
	}
	if err := repo.CreateTask(canc); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c127','기관127','기관127',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, status, assigned_to, complete_datetime, org_id, symptom)
		VALUES ('as127','R127','c127',?,'`+day+`','completed',?,?,'`+model.OrgIDLibrary+`','증상')`,
		day+" 09:00:00", me.FullName, day+" 16:00:00"); err != nil {
		t.Fatal(err)
	}
	asTask := &model.WorkTask{
		Title: "[AS]완료만127", WorkDate: day, DueDate: day, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskWaiting,
		WorkType: model.WBWorkAS, SourceType: model.WBSourceAS, SourceID: "as127",
	}
	if err := repo.CreateTask(asTask); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P127', 2026, 'p', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, completed_date, assignee, product_type)
		VALUES ('mv127','P127',?, 'c127', 1, ?, ?, 'KLAS')`, day, day, me.FullName); err != nil {
		t.Fatal(err)
	}
	mnt := &model.WorkTask{
		Title: "[점검]완료만127", WorkDate: day, DueDate: day, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskWaiting,
		WorkType: model.WBWorkMaintenance, SourceType: model.WBSourceMaintenance, SourceID: "mv127",
		CompleteDate: "2099-01-01",
	}
	if err := repo.CreateTask(mnt); err != nil {
		t.Fatal(err)
	}

	act := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+"&auto=0&show=act", ck)
	ab := act.Body.String()
	if !strings.Contains(ab, "완료일만127") {
		t.Fatal("일반 완료일만 없음")
	}
	if !strings.Contains(ab, "완료만127") {
		t.Fatal("AS·점검 완료가 없음")
	}
	if strings.Contains(ab, "기간밖완료127") {
		t.Fatal("기간 밖 완료가 보임")
	}
	if strings.Contains(ab, "취소127") {
		t.Fatal("취소가 완료 보기에 있음")
	}
	if !strings.Contains(ab, "완료 3건") {
		t.Fatalf("완료 건수 %s", snippetShow(ab))
	}
}
