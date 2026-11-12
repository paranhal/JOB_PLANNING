package handler

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/service"
)

func Test95_84_ClassifyKanban(t *testing.T) {
	today := time.Now().Format(dateLayout)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	tom := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	start := time.Now().AddDate(0, 0, -3).Format(dateLayout)
	items := []boardItem{
		{Task: model.WorkTask{TaskID: "d1", Title: "지연", DueDate: yest, Status: model.WBTaskWaiting, WorkDate: yest}, DedupKey: "d1"},
		{Task: model.WorkTask{TaskID: "dt", Title: "지연오늘일정", DueDate: yest, Status: model.WBTaskWaiting, WorkDate: today}, DedupKey: "dt"},
		{Task: model.WorkTask{TaskID: "p1", Title: "과거부분", DueDate: yest, Status: model.StatusPartialComplete, WorkDate: yest}, DedupKey: "p1"},
		{Task: model.WorkTask{TaskID: "p2", Title: "오늘부분", DueDate: today, Status: model.StatusPartialComplete, WorkDate: today}, DedupKey: "p2"},
		{Task: model.WorkTask{TaskID: "off", Title: "일정외", Status: model.WBTaskInProgress}, DedupKey: "off"},
		{Task: model.WorkTask{TaskID: "span", Title: "기간중간", WorkDate: start, DueDate: tom, Status: model.WBTaskInProgress}, DedupKey: "span"},
		{Task: model.WorkTask{TaskID: "plan", Title: "오늘계획", DueDate: today, Status: model.WBTaskWaiting, WorkDate: today, StartTime: "09:00", EndTime: "10:00"}, DedupKey: "plan", HasTime: true, StartMin: 540},
		{Task: model.WorkTask{TaskID: "prog", Title: "오늘진행", DueDate: today, Status: model.WBTaskInProgress, WorkDate: today}, DedupKey: "prog"},
		{Task: model.WorkTask{TaskID: "done", Title: "오늘완료", DueDate: today, Status: model.WBTaskComplete, WorkDate: today, CompleteDate: today}, DedupKey: "done"},
		{Task: model.WorkTask{TaskID: "old", Title: "과거완료", DueDate: yest, Status: model.WBTaskComplete, WorkDate: yest, CompleteDate: yest}, DedupKey: "old"},
		{Task: model.WorkTask{TaskID: "cx", Title: "취소", DueDate: yest, Status: model.WBTaskCancelled, WorkDate: yest}, DedupKey: "cx"},
		{Task: model.WorkTask{TaskID: "nodue", Title: "종료없음", WorkDate: start, Status: model.WBTaskWaiting}, DedupKey: "nodue"},
		{Task: model.WorkTask{TaskID: "dueToday", Title: "종료오늘", DueDate: today, Status: model.WBTaskWaiting, WorkDate: start}, DedupKey: "dueToday"},
	}
	cols := classifyKanban(items, today)
	if len(cols[kanbanColDelay]) != 2 {
		t.Fatalf("지연 %+v", titles84(cols[kanbanColDelay]))
	}
	var badge bool
	for _, it := range cols[kanbanColDelay] {
		if it.Task.Title == "지연오늘일정" && it.KanbanBadgeToday {
			badge = true
		}
	}
	if !badge {
		t.Fatal("지연+오늘 일정 배지 없음")
	}
	if len(cols[kanbanColPartial]) != 1 || cols[kanbanColPartial][0].Task.Title != "오늘부분" {
		t.Fatalf("부분 완료 %+v", titles84(cols[kanbanColPartial]))
	}
	if len(cols[kanbanColOff]) != 2 {
		t.Fatalf("진행중(일정 외) %+v", titles84(cols[kanbanColOff]))
	}
	if len(cols[kanbanColPlan]) < 2 {
		t.Fatalf("오늘 계획 %+v", titles84(cols[kanbanColPlan]))
	}
	if len(cols[kanbanColProgress]) != 1 {
		t.Fatalf("오늘 진행 %+v", titles84(cols[kanbanColProgress]))
	}
	if len(cols[kanbanColDone]) != 1 || cols[kanbanColDone][0].Task.Title != "오늘완료" {
		t.Fatalf("완료 %+v", titles84(cols[kanbanColDone]))
	}
}

func titles84(items []boardItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Task.Title)
	}
	return out
}

func Test95_84_BoardViewsAndListRange(t *testing.T) {
	e, _, _, ck, repo, me := new80(t)
	today := time.Now()
	day := today.Format(dateLayout)
	mustCreateBoardTask(t, repo, "리스트일", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mon := today.AddDate(0, 0, -mondayOffset(today))
	weekDayT := mon.AddDate(0, 0, 2)
	if weekDayT.Format(dateLayout) == day {
		weekDayT = mon.AddDate(0, 0, 3)
	}
	weekDay := weekDayT.Format(dateLayout)
	mustCreateBoardTask(t, repo, "리스트주", weekDay, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	page := doGetWith(t, e, "/workboard/board?auto=0", ck).Body.String()
	if !strings.Contains(page, ">요약</a>") || !strings.Contains(page, ">일정표</a>") || !strings.Contains(page, ">칸반</a>") {
		t.Fatal("요약·일정표·칸반 세그 없음")
	}
	idxDay := strings.Index(page, ">일</a>")
	idxWeek := strings.Index(page, ">주</a>")
	idxMonth := strings.Index(page, ">월</a>")
	if idxDay < 0 || idxWeek < 0 || idxMonth < 0 || !(idxDay < idxWeek && idxWeek < idxMonth) {
		t.Fatalf("일/주/월 순서 day=%d week=%d month=%d", idxDay, idxWeek, idxMonth)
	}
	if !strings.Contains(page, `data-summary-card="today"`) {
		t.Fatal("기본이 요약이 아님")
	}
	week := doGetWith(t, e, "/workboard/board?view=sched&range=week&date="+day+"&auto=0", ck).Body.String()
	if !strings.Contains(week, "리스트일") {
		t.Fatal("주 범위에 오늘 항목 없음")
	}
	if !strings.Contains(week, "리스트주") {
		t.Fatal("주 범위에 같은 주 항목 없음")
	}
	if !strings.Contains(week, "내 업무") {
		t.Fatal("scope 유지 안 됨")
	}
	cal := doGetWith(t, e, "/workboard/board?view=day&auto=0", ck).Body.String()
	if !strings.Contains(cal, ">칸반</a>") {
		t.Fatal("일정표에 칸반 링크 없음")
	}
	pw := &model.WorkTask{
		Title: "후속영업일", WorkDate: day, DueDate: day, StartTime: "16:00", EndTime: "17:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkProjectWork, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(pw); err != nil {
		t.Fatal(err)
	}
	act := doGetWith(t, e, "/workboard/tasks/"+pw.TaskID+"/action", ck).Body.String()
	next, _ := service.NextWorkday(time.Now(), nil, "", false)
	if !strings.Contains(act, next) {
		t.Fatal("부분 완료 후속 기본 날짜가 영업일이 아님")
	}
}

func Test95_84_KanbanHTMLAndMove(t *testing.T) {
	e, _, h, ck, repo, me := new80(t)
	e.POST("/workboard/board/kanban-move", h.Workboard.BoardKanbanMove, h.Auth.AuthMiddleware)
	e.GET("/workboard/kanban", h.Workboard.Kanban, h.Auth.AuthMiddleware)
	today := time.Now().Format(dateLayout)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	tom := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	mustCreateBoardTask(t, repo, "지연칸반", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "오늘계획칸반", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "계획유지칸반", today, "10:00", "10:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "오늘진행칸반", today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskInProgress, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "일정외칸반", today, "08:00", "08:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskInProgress, model.WBWorkAdmin)
	off := taskByTitle(t, repo, "일정외칸반")
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET work_date='', due_date='' WHERE task_id=?`, off.TaskID); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "부분칸반", today, "13:00", "14:00", model.OrgIDLibrary, me.FullName, me.UserID, model.StatusPartialComplete, model.WBWorkProjectWork)
	part := taskByTitle(t, repo, "부분칸반")
	if err := repo.ReplaceItems(part.TaskID, []model.WorkTaskItem{{Title: "남음", Kind: model.ItemKindWork, Status: model.ItemTodo, Qty: 1}}); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "완료칸반", today, "15:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	done := taskByTitle(t, repo, "완료칸반")
	done.CompleteDate = today
	_ = repo.UpdateTask(done)
	body := doGetWith(t, e, "/workboard/board?view=kanban&auto=0", ck).Body.String()
	for _, want := range []string{"미계획업무", "진행중", "지연"} {
		if !strings.Contains(body, want) {
			t.Errorf("칸반 HTML에 %q 없음", want)
		}
	}
	if !strings.Contains(body, "지연칸반") || !strings.Contains(body, "오늘계획칸반") {
		t.Fatal("칸반 카드 누락")
	}
	keep := taskByTitle(t, repo, "계획유지칸반")
	rej := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + keep.TaskID}, "from": {kanbanColPlan}, "to": {kanbanColPartial},
	}, ck)
	if loc := rej.Header().Get("Location"); !strings.Contains(loc, "partial") {
		t.Fatalf("계획→부분 거부 loc=%s", loc)
	}
	plan := taskByTitle(t, repo, "오늘계획칸반")
	mv := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + plan.TaskID}, "from": {kanbanColPlan}, "to": {kanbanColProgress},
	}, ck)
	if mv.Code != http.StatusSeeOther && mv.Code != http.StatusOK {
		t.Fatalf("계획→진행중 %d %s", mv.Code, mv.Body.String())
	}
	got, _ := repo.GetTask(plan.TaskID)
	if got.Status != model.WBTaskInProgress {
		t.Fatalf("상태 %s", got.Status)
	}
	prog := taskByTitle(t, repo, "오늘진행칸반")
	need := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + prog.TaskID}, "from": {kanbanColProgress}, "to": {kanbanColDone},
	}, ck)
	if loc := need.Header().Get("Location"); !strings.Contains(loc, "kanban_complete") {
		t.Fatalf("완료 내용 필수 loc=%s", loc)
	}
	if err := repo.ReplaceItems(prog.TaskID, []model.WorkTaskItem{{Title: "미완", Kind: model.ItemKindWork, Status: model.ItemTodo, Qty: 1}}); err != nil {
		t.Fatal(err)
	}
	partAsk := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + prog.TaskID}, "from": {kanbanColProgress}, "to": {kanbanColDone}, "complete_note": {"끝"},
	}, ck)
	if loc := partAsk.Header().Get("Location"); !strings.Contains(loc, "kanban_partial") {
		t.Fatalf("부분 완료 확인 loc=%s", loc)
	}
	okPart := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + prog.TaskID}, "from": {kanbanColProgress}, "to": {kanbanColPartial},
		"complete_note": {"일부"}, "next_date": {tom},
	}, ck)
	if okPart.Code != http.StatusSeeOther && okPart.Code != http.StatusOK {
		t.Fatalf("부분 완료 %d %s", okPart.Code, okPart.Body.String())
	}
	forbid := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + plan.TaskID}, "from": {kanbanColPlan}, "to": {kanbanColDelay},
	}, ck)
	if loc := forbid.Header().Get("Location"); !strings.Contains(loc, "forbidden") {
		t.Fatalf("금지 이동 loc=%s", loc)
	}
	delay := taskByTitle(t, repo, "지연칸반")
	noCarry := doFormJSONWith(t, e, "/workboard/board/kanban-move", url.Values{
		"item_key": {"task:" + delay.TaskID}, "from": {kanbanColDelay}, "to": {kanbanColPlan},
	}, ck)
	if loc := noCarry.Header().Get("Location"); !strings.Contains(loc, "kanban_carry") {
		t.Fatalf("이월 확인 loc=%s", loc)
	}
	old := doGetWith(t, e, "/workboard/kanban", ck)
	if old.Code != http.StatusOK {
		t.Fatalf("구 칸반 %d", old.Code)
	}
}

func Test95_84_CarryModalNextWorkdayAndQueryCap(t *testing.T) {
	var n int64
	qcount9520N.Store(&n)
	t.Cleanup(func() { qcount9520N.Store(nil) })
	e, _, me, _, repo := seedPersonalBoard(t, sqliteQCount9520, filepath.Join(t.TempDir(), "q84.db"))
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	for i := 0; i < 200; i++ {
		mustCreateBoardTask(t, repo, "cap84-"+strconv.Itoa(i), today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	atomic.StoreInt64(&n, 0)
	list := doGetWith(t, e, "/workboard/board?view=summary&auto=0", ck)
	if list.Code != 200 {
		t.Fatal(list.Code)
	}
	qList := atomic.LoadInt64(&n)
	atomic.StoreInt64(&n, 0)
	kb := doGetWith(t, e, "/workboard/board?view=kanban&auto=0", ck)
	if kb.Code != 200 {
		t.Fatal(kb.Code)
	}
	qK := atomic.LoadInt64(&n)
	if qList < 1 || qK < 1 {
		t.Fatalf("쿼리 미계측 요약=%d 칸반=%d", qList, qK)
	}
	if qK > 4000 {
		t.Fatalf("칸반 쿼리 폭주 %d", qK)
	}
	if !strings.Contains(list.Body.String(), `id="carry-new-date"`) {
		t.Fatal("이월 모달 없음")
	}
	next, _ := service.NextWorkday(time.Now(), nil, "", false)
	if !strings.Contains(list.Body.String(), `value="`+next+`"`) {
		t.Fatalf("모달 초기값 영업일 아님 want %s", next)
	}
	css, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cls := range []string{".min-w-\\[260px\\]", ".snap-x", ".snap-mandatory"} {
		if !strings.Contains(string(css), cls) {
			t.Errorf("번들 %s 없음", cls)
		}
	}
}

func Test95_84_OldKanbanStillUsesSharedMove(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "workboard_kanban.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "service.ApplyKanbanMove") {
		t.Fatal("구 칸반이 공유 이동을 안 씀")
	}
	if !strings.Contains(string(src), "handlerKanbanOps") {
		t.Fatal("KanbanOps 어댑터 없음")
	}
}
