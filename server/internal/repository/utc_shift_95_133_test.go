package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test95_133ShiftUTCIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "shift.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "보정", DueDate: "2026-11-06", Status: model.WBTaskWaiting}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	cutoff := "2026-11-06 16:50:00"
	if _, err := db.Exec(`UPDATE app_settings SET setting_value=?, updated_at=? WHERE setting_key=?`,
		cutoff, cutoff, settingLocalNowSince); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at)
		VALUES ('WY-OLD', ?, 'other', '옛UTC', 30, '2026-11-06 07:48:45')`, task.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at)
		VALUES ('WY-NEW', ?, 'other', '신규현지', 30, '2026-11-06 16:51:00')`, task.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mail_outbox(mail_id, to_addr, subject, body, created_at)
		VALUES ('M-OLD', 'a@b.c', 's', 'b', '2026-11-06 07:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sms_outbox(sms_id, to_addr, subject, body, created_at)
		VALUES ('S-OLD', '010', 's', 'b', '2026-11-06 07:00:00')`); err != nil {
		t.Fatal(err)
	}

	a, m, s, err := Apply95_133ShiftUTC(db)
	if err != nil {
		t.Fatal(err)
	}
	if a != 1 || m != 1 || s != 1 {
		t.Fatalf("건수 a=%d m=%d s=%d", a, m, s)
	}
	assertCreated := func(q, want string) {
		t.Helper()
		var got string
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("got=%q want=%q sql=%s", got, want, q)
		}
	}
	assertCreated(`SELECT COALESCE(created_at,'') FROM work_activities WHERE activity_id='WY-OLD'`, "2026-11-06 16:48:45")
	assertCreated(`SELECT COALESCE(created_at,'') FROM work_activities WHERE activity_id='WY-NEW'`, "2026-11-06 16:51:00")
	assertCreated(`SELECT COALESCE(created_at,'') FROM mail_outbox WHERE mail_id='M-OLD'`, "2026-11-06 16:00:00")
	assertCreated(`SELECT COALESCE(created_at,'') FROM sms_outbox WHERE sms_id='S-OLD'`, "2026-11-06 16:00:00")

	a2, m2, s2, err := Apply95_133ShiftUTC(db)
	if err != nil {
		t.Fatal(err)
	}
	if a2 != 0 || m2 != 0 || s2 != 0 {
		t.Fatalf("멱등 실패 a=%d m=%d s=%d", a2, m2, s2)
	}
	assertCreated(`SELECT COALESCE(created_at,'') FROM work_activities WHERE activity_id='WY-OLD'`, "2026-11-06 16:48:45")
}
