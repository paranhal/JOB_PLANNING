package handler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_132_ParseBoardDisplay(t *testing.T) {
	cases := []struct {
		view, rng, display, period string
	}{
		{"", "", boardDisplaySummary, boardRangeDay},
		{"nope", "", boardDisplaySummary, boardRangeDay},
		{"list", "week", boardDisplaySchedule, boardRangeWeek},
		{"day", "", boardDisplaySchedule, boardRangeDay},
		{"sched", "day", boardDisplaySchedule, boardRangeDay},
		{"sched", "week", boardDisplaySchedule, boardRangeWeek},
		{"sched", "", boardDisplaySchedule, boardRangeDay},
		{"kanban", "", boardDisplayKanban, boardRangeDay},
		{"kanban", "week", boardDisplayKanban, boardRangeWeek},
		{"summary", "month", boardDisplaySummary, boardRangeMonth},
	}
	for _, tc := range cases {
		d, p := parseBoardDisplay(tc.view, tc.rng)
		if d != tc.display || p != tc.period {
			t.Fatalf("view=%q range=%q → %s/%s want %s/%s", tc.view, tc.rng, d, p, tc.display, tc.period)
		}
	}
}

func Test95_132_RegisterCopyPOSTPaths(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132copy.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "복사검증", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	reg := doGetWith(t, e, "/workboard/register?view=day&date="+today, ck).Body.String()
	board := doGetWith(t, e, "/workboard/board?view=day&date="+today, ck).Body.String()
	for _, path := range []string{`action="/workboard/schedule"`, `action="/workboard/unschedule"`} {
		if !strings.Contains(reg, path) {
			t.Fatalf("register에 %s 없음", path)
		}
		if !strings.Contains(board, path) {
			t.Fatalf("board 일정표에 %s 없음", path)
		}
	}
	if !strings.Contains(board, `data-register-copy="95-132"`) {
		t.Fatal("복사 표식 없음")
	}
	kb := doGetWith(t, e, "/workboard/board?view=kanban&date="+today, ck).Body.String()
	if !strings.Contains(kb, `action="/workboard/register/kanban-move"`) {
		t.Fatal("칸반 POST 경로가 register와 다름")
	}
	if !strings.Contains(kb, "미계획업무") || !strings.Contains(kb, "진행중") {
		t.Fatal("칸반 5열 없음")
	}
}

func Test95_132_SummaryScheduleKanbanCounts(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132cnt.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "오늘행정", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "오늘완료", today, "10:00", "11:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET complete_date=? WHERE title='오늘완료'`, today); err != nil {
		t.Fatal(err)
	}
	sum := doGetWith(t, e, "/workboard/board?view=summary&date="+today, ck).Body.String()
	day := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+today, ck).Body.String()
	kan := doGetWith(t, e, "/workboard/board?view=kanban&date="+today, ck).Body.String()
	if !strings.Contains(sum, `data-summary-card="today"`) {
		t.Fatal("요약 오늘 카드 없음")
	}
	if !strings.Contains(day, "오늘행정") || !strings.Contains(day, "오늘완료") {
		t.Fatal("일정표에 오늘 건이 없다")
	}
	if !strings.Contains(kan, "다섯 열 합계") {
		t.Fatal("칸반 합계 문구 없음")
	}
	items := []boardItem{
		{Task: model.WorkTask{TaskID: "a", Title: "행정", WorkType: model.WBWorkAdmin, WorkDate: today, DueDate: today, Status: model.WBTaskWaiting}, PlaceDate: today, DedupKey: "a"},
		{Task: model.WorkTask{TaskID: "b", Title: "완료", WorkType: model.WBWorkAdmin, WorkDate: today, DueDate: today, Status: model.WBTaskComplete, CompleteDate: today}, PlaceDate: today, DedupKey: "b"},
	}
	sm := buildBoardSummary(items, boardViewDay, today, today, today, boardPrevWorkday(today))
	nToday := summaryCardTotal(sm.Cards[2])
	if nToday != 2 {
		t.Fatalf("요약 오늘=%d", nToday)
	}
}

func Test95_132_TaskShowDeleteMatchesList(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132del.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "삭제가능행정", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	if err := repo.DB().QueryRow(`SELECT task_id FROM work_tasks WHERE title='삭제가능행정'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	body := doGetWith(t, e, "/workboard/tasks/"+id, ck).Body.String()
	if !strings.Contains(body, `/workboard/tasks/`+id+`/delete`) {
		t.Fatal("행정 상세 삭제 없음")
	}
	mnt := &model.WorkTask{Title: "점검삭제불가", WorkDate: today, DueDate: today, OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID, WorkType: model.WBWorkMaintenance, SourceType: model.WBSourceMaintenance, SourceID: "V-x"}
	if err := repo.CreateTask(mnt); err != nil {
		t.Fatal(err)
	}
	mb := doGetWith(t, e, "/workboard/tasks/"+mnt.TaskID, ck).Body.String()
	if !strings.Contains(mb, msgBulkMnt) {
		t.Fatal("점검 삭제 불가 안내 없음")
	}
	if strings.Contains(mb, `/workboard/tasks/`+mnt.TaskID+`/delete`) {
		t.Fatal("점검 상세에 삭제 버튼이 있다")
	}
}

func Test95_132_StartWorkHrefAndColMin(t *testing.T) {
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "132start.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	sum := doGetWith(t, e, "/workboard/board?view=summary&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(sum, "view=sched") || !strings.Contains(sum, "range=day") {
		t.Fatal("요약 업무 시작 URL 없음")
	}
	if strings.Contains(sum, `id="schedule-show"`) {
		t.Fatal("요약에 접수 토글")
	}
	day := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(day, `id="schedule-show"`) {
		t.Fatal("일정표 일 토글 없음")
	}
	if !strings.Contains(day, "390") {
		t.Fatal("담당자 열 390px 없음")
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+today+"&auto=0", ck).Body.String()
	if strings.Contains(week, `id="schedule-show"`) {
		t.Fatal("주에 토글")
	}
}

func Test95_132_KanbanUsesSelectedDate(t *testing.T) {
	d, p := parseBoardDisplay("kanban", "")
	if d != boardDisplayKanban || p != boardRangeDay {
		t.Fatal("칸반 기본 일")
	}
	u := boardModeURL(boardDisplayKanban, boardRangeDay, "2026-10-01", nil)
	if !strings.Contains(u, "date=2026-10-01") || strings.Contains(u, "view=kanban") == false {
		t.Fatalf("칸반 URL %s", u)
	}
}
