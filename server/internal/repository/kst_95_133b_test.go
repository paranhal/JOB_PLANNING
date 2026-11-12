package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 95-133B: 새 업무의 created_at 은 서버 TZ 와 무관하게 한국 시각이다.
func TestCreateTaskCreatedAtIsKST(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	prev := time.Local
	defer func() { time.Local = prev }()
	// UTC 2026-11-05 15:30 = KST 2026-11-06 00:30 (UTC 날짜와 KST 날짜가 다른 시각)
	fixed := time.Date(2026, 11, 5, 15, 30, 0, 0, time.UTC)
	restore := model.SetNowForTest(func() time.Time { return fixed })
	defer restore()
	for i, local := range []*time.Location{time.UTC, model.KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "등록일" + string(rune('A'+i)), DueDate: "2026-11-06", Status: model.WBTaskWaiting}
		if err := wb.CreateTask(task); err != nil {
			t.Fatal(err)
		}
		var raw string
		if err := db.QueryRow(`SELECT strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_tasks WHERE task_id=?`, task.TaskID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if raw != "2026-11-06 00:30:00" {
			t.Fatalf("Local=%s created_at=%s (KST 00:30 기대)", local, raw)
		}
		got, err := wb.GetTask(task.TaskID)
		if err != nil || got == nil {
			t.Fatalf("GetTask %v", err)
		}
		if d := got.CreatedAt.Format("2006-01-02"); d != "2026-11-06" {
			t.Fatalf("Local=%s 업무 등록일 %s (2026-11-06 기대)", local, d)
		}
	}
}

// 같은 업무를 연속 저장하는 중복 방지(FindRecentDirectTask)는 created_at(KST)과 같은 기준으로 비교한다.
// 운영 서버(TZ=Asia/Seoul)에서 created_at 이 UTC 기본값이던 때는 방금 만든 업무도 찾지 못했다.
func TestFindRecentDirectTaskMatchesJustCreated(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	prev := time.Local
	defer func() { time.Local = prev }()
	for _, local := range []*time.Location{time.UTC, model.KST()} {
		time.Local = local
		title := "방금저장" + local.String()
		task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: title, DueDate: "2026-11-06", Status: model.WBTaskWaiting}
		if err := wb.CreateTask(task); err != nil {
			t.Fatal(err)
		}
		got, err := wb.FindRecentDirectTask("", title, "", model.WBWorkAdmin, "", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.TaskID != task.TaskID {
			t.Fatalf("Local=%s 방금 만든 업무를 못 찾는다", local)
		}
	}
}

// 쓰기가 계속 UTC(CURRENT_TIMESTAMP) 기본값인 열은 FormatUTCDateTime 으로 보여 준다.
// 이 열들의 기본값이 UTC 가 아니게 되면(예: localtime) 표시가 9시간 어긋나므로 여기서 잡는다.
func TestUTCDefaultColumnsAreUTC(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ins := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	ins(`INSERT INTO cooperation_events(cooperation_id, actor_id, action) VALUES ('C1','u','x')`)
	ins(`INSERT INTO work_group_events(group_id, actor_id, action) VALUES ('G1','u','x')`)
	ins(`INSERT INTO leave_absence_records(absence_id, user_id, leave_req_id, minutes, kind, note) VALUES ('A1','u','R1',60,'k','n')`)
	for _, q := range []string{
		`SELECT CAST(strftime('%s','now') AS INTEGER) - CAST(strftime('%s', at) AS INTEGER) FROM cooperation_events WHERE cooperation_id='C1'`,
		`SELECT CAST(strftime('%s','now') AS INTEGER) - CAST(strftime('%s', at) AS INTEGER) FROM work_group_events WHERE group_id='G1'`,
		`SELECT CAST(strftime('%s','now') AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER) FROM leave_absence_records WHERE absence_id='A1'`,
	} {
		var diff int
		if err := db.QueryRow(q).Scan(&diff); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if diff < -5 || diff > 5 {
			t.Fatalf("기본값이 UTC 가 아니다(now 와 %d초 차이): %s", diff, q)
		}
	}
}
