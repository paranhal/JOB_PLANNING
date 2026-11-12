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
	// 95-133B: 업무 등록일·팀장 변경 이력. 업무는 UTC 15:30 = KST 다음 날 00:30 이라 날짜가 바뀐다.
	ins(`UPDATE work_tasks SET created_at='2026-11-06 15:30:00' WHERE task_id=?`, task.TaskID)
	ins(`INSERT INTO org_head_history(history_id, org_id, acted_by, acted_at, pending_mode, pending_moved)
		VALUES ('H-OLD', 'O01', 'u1', '2026-11-06 07:00:00', 'keep', 0)`)
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
	// 표별 {대상, cutoff 이후, 날짜가 바뀌는 건수}
	want := map[string][3]int{
		"work_activities": {1, 2, 0}, "mail_outbox": {1, 0, 0}, "sms_outbox": {1, 0, 0},
		"work_tasks": {1, 0, 1}, "org_head_history": {1, 0, 0},
	}
	if len(rep.Tables) != len(want) {
		t.Fatalf("표 수 %d", len(rep.Tables))
	}
	for _, tr := range rep.Tables {
		w, ok := want[tr.Table]
		if !ok {
			t.Fatalf("예상 못 한 표 %s", tr.Table)
		}
		if tr.Target != w[0] || tr.After != w[1] || tr.DateChanged != w[2] || tr.Shifted != 0 {
			t.Fatalf("%s target=%d after=%d dateChanged=%d shifted=%d", tr.Table, tr.Target, tr.After, tr.DateChanged, tr.Shifted)
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
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_tasks LIMIT 1`, "2026-11-07 00:30:00")
	chk(`SELECT strftime('%Y-%m-%d %H:%M:%S', acted_at) FROM org_head_history WHERE history_id='H-OLD'`, "2026-11-06 16:00:00")

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

// -detect: UTC 로 저장되던 행에서 KST 로 저장되기 시작하는 지점(숫자가 +9시간쯤 뛰는 곳)을 후보로 찾는다.
func TestDetectUTCShiftCutoffFindsJump(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "탐지", DueDate: "2026-11-05", Status: model.WBTaskWaiting}
	if err := NewWBRepo(db).CreateTask(task); err != nil {
		t.Fatal(err)
	}
	at := func(i int, base time.Time) string { return base.Add(time.Duration(i) * 30 * time.Minute).Format("2006-01-02 15:04:05") }
	utcStart := time.Date(2026, 11, 5, 0, 30, 0, 0, time.UTC)
	for i := 0; i < 15; i++ { // UTC 저장 시대: 00:30 ~ 07:30
		if _, err := db.Exec(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at) VALUES (?,?, 'other','u',0,?)`,
			"U"+string(rune('A'+i)), task.TaskID, at(i, utcStart)); err != nil {
			t.Fatal(err)
		}
	}
	// KST 저장이 시작: 다음 행은 30분 뒤인데 숫자는 +9시간(08:00 → 17:00)
	kstStart := utcStart.Add(15*30*time.Minute + 9*time.Hour)
	for i := 0; i < 15; i++ {
		if _, err := db.Exec(`INSERT INTO work_activities(activity_id, task_id, activity_type, content, spent_minutes, created_at) VALUES (?,?, 'other','k',0,?)`,
			"K"+string(rune('A'+i)), task.TaskID, at(i, kstStart)); err != nil {
			t.Fatal(err)
		}
	}
	jumps, n, err := DetectUTCShiftCutoff(db, 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 30 || len(jumps) != 1 {
		t.Fatalf("행 %d, 후보 %d: %+v", n, len(jumps), jumps)
	}
	j := jumps[0]
	if j.PrevID != "UO" || j.NextID != "KA" || j.NextAt != "2026-11-05 17:00:00" || j.Before != 15 || j.After != 15 {
		t.Fatalf("후보 %+v", j)
	}
	// 읽기 전용이라 아무것도 쓰지 않는다
	var cnt int
	_ = db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE setting_key='95_133_utc_shift_done'`).Scan(&cnt)
	if cnt != 0 {
		t.Fatal("탐지가 마커를 남겼다")
	}
	// 균일한 시계열(뛰는 곳 없음)은 후보가 없다
	if _, err := db.Exec(`DELETE FROM work_activities WHERE activity_id LIKE 'K%'`); err != nil {
		t.Fatal(err)
	}
	if jumps, _, _ = DetectUTCShiftCutoff(db, 5); len(jumps) != 0 {
		t.Fatalf("균일한 데이터에서 후보가 나왔다: %+v", jumps)
	}
}
