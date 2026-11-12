package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func newShiftDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "shift.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "보정", DueDate: "2026-11-06", Status: model.WBTaskWaiting}
	if err := NewWBRepo(db).CreateTask(task); err != nil {
		t.Fatal(err)
	}
	ins := func(q string, args ...interface{}) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// cutoff 2026-11-06 16:50:00(KST) 기준. 앞 3건은 UTC 로 저장된 옛 행, 뒤는 현지로 저장된 새 행·경계.
	ins(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at)
		VALUES ('WY-OLD', ?, 'other', '옛UTC', 30, '2026-11-06 07:48:45')`, task.TaskID)
	ins(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at)
		VALUES ('WY-NEW', ?, 'other', '신규현지', 30, '2026-11-06 16:51:00')`, task.TaskID)
	ins(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at)
		VALUES ('WY-EDGE', ?, 'other', '경계', 30, '2026-11-06 16:50:00')`, task.TaskID)
	ins(`INSERT INTO mail_outbox(mail_id, to_addr, subject, body, created_at)
		VALUES ('M-OLD', 'a@b.c', 's', 'b', '2026-11-06 07:00:00')`)
	ins(`INSERT INTO sms_outbox(sms_id, to_addr, subject, body, created_at)
		VALUES ('S-OLD', '010', 's', 'b', '2026-11-06 07:00:00')`)
	return db
}

func shiftVal(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var got string
	if err := db.QueryRow(q).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func withNow(t *testing.T, now string) {
	t.Helper()
	// cutoff 가 미래가 아니도록 현재 시각을 고정한다.
	tm, err := time.ParseInLocation("2006-01-02 15:04:05", now, model.KST())
	if err != nil {
		t.Fatal(err)
	}
	restore := model.SetNowForTest(func() time.Time { return tm })
	t.Cleanup(restore)
}

func TestShiftUTCToKSTPreviewWritesNothing(t *testing.T) {
	db := newShiftDB(t)
	withNow(t, "2026-11-12 09:00:00")
	rep, err := ShiftUTCToKST(db, "2026-11-06 16:50", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Applied || rep.Cutoff != "2026-11-06 16:50:00" {
		t.Fatalf("rep=%+v", rep)
	}
	want := map[string][3]int{"work_activities": {1, 2, 0}, "mail_outbox": {1, 0, 0}, "sms_outbox": {1, 0, 0}}
	for _, tr := range rep.Tables {
		w := want[tr.Table]
		if tr.Target != w[0] || tr.After != w[1] || tr.Shifted != 0 {
			t.Fatalf("%s target=%d after=%d shifted=%d", tr.Table, tr.Target, tr.After, tr.Shifted)
		}
	}
	if got := shiftVal(t, db, `SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-OLD'`); got != "2026-11-06 07:48:45" {
		t.Fatalf("미리보기가 썼다: %s", got)
	}
	if got := shiftVal(t, db, `SELECT COUNT(*) FROM app_settings WHERE setting_key='95_133_utc_shift_done'`); got != "0" {
		t.Fatalf("미리보기가 마커를 남겼다: %s", got)
	}
}

func TestShiftUTCToKSTApplyOnceAndBoundary(t *testing.T) {
	db := newShiftDB(t)
	withNow(t, "2026-11-12 09:00:00")
	rep, err := ShiftUTCToKST(db, "2026-11-06 16:50:00", true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Applied {
		t.Fatalf("rep=%+v", rep)
	}
	for _, tr := range rep.Tables {
		if tr.Shifted != tr.Target {
			t.Fatalf("%s shifted=%d target=%d", tr.Table, tr.Shifted, tr.Target)
		}
	}
	chk := func(q, want string) {
		t.Helper()
		if got := shiftVal(t, db, q); got != want {
			t.Fatalf("got=%q want=%q sql=%s", got, want, q)
		}
	}
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-OLD'`, "2026-11-06 16:48:45")
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-NEW'`, "2026-11-06 16:51:00")
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-EDGE'`, "2026-11-06 16:50:00") // cutoff 와 같으면 현지로 보고 그대로
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM mail_outbox WHERE mail_id='M-OLD'`, "2026-11-06 16:00:00")
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM sms_outbox WHERE sms_id='S-OLD'`, "2026-11-06 16:00:00")

	_, err = ShiftUTCToKST(db, "2026-11-06 16:50:00", true)
	if err == nil || !strings.Contains(err.Error(), "이미 반영") {
		t.Fatalf("두 번째 반영은 거절돼야 한다: %v", err)
	}
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-OLD'`, "2026-11-06 16:48:45")
}

func TestShiftUTCToKSTRejectsBadCutoff(t *testing.T) {
	db := newShiftDB(t)
	withNow(t, "2026-11-12 09:00:00")
	for _, c := range []string{"", "어제", "2026-11-13 00:00:00"} {
		if _, err := ShiftUTCToKST(db, c, true); err == nil {
			t.Fatalf("cutoff %q 는 거절돼야 한다", c)
		}
	}
	if got := shiftVal(t, db, `SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities WHERE activity_id='WY-OLD'`); got != "2026-11-06 07:48:45" {
		t.Fatalf("거절됐는데 썼다: %s", got)
	}
}
