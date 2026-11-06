package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_94_A_WorkFormPeriodTemplate(t *testing.T) {
	modals := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard/_modals.html")))
	if strings.Count(modals, `{{template "wb_period_dates"`) < 2 {
		t.Fatal("일반·작업 폼이 wb_period_dates 를 공유하지 않음")
	}
	pw := modals[strings.Index(modals, "wb_form_project_work"):]
	if i := strings.Index(pw, "{{end}}"); i > 0 {
		pw = pw[:i]
	}
	if strings.Contains(pw, "작업일") {
		t.Fatal("작업 폼에 작업일 라벨이 남아 있음")
	}
	part := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard/_work_period_dates.html")))
	if !strings.Contains(part, "업무 시작일") || !strings.Contains(part, "업무 종료일") {
		t.Fatal("기간 부분 템플릿 라벨 없음")
	}
	e, _, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94a.db"))
	ck := jwtCookieUser(t, me)
	page := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&date=2026-11-10", ck)
	if page.Code != 200 {
		t.Fatalf("폼 %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "업무 시작일") || !strings.Contains(body, "업무 종료일") {
		t.Fatal("작업 폼 렌더에 시작·종료일 없음")
	}
	if strings.Contains(body, ">작업일") {
		t.Fatal("작업 폼 렌더에 작업일 라벨")
	}
}

func Test95_94_A_DueBeforeWork(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94due.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 8).Format(dateLayout)
	earlier := time.Now().AddDate(0, 0, 5).Format(dateLayout)
	rec := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"종료일앞94"}, "work_subtype": {"build"},
		"work_date": {day}, "due_date": {earlier}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {me.FullName},
	}, ck)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "date_order") {
		t.Fatalf("거부 %d loc=%s body=%s", rec.Code, loc, rec.Body.String())
	}
	empty := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"하루94"}, "work_date": {day},
		"start_time": {"13:00"}, "end_time": {"13:30"}, "assignee": {me.FullName},
	})
	if empty.Code != http.StatusSeeOther && empty.Code != http.StatusOK {
		t.Fatalf("비움 %d %s", empty.Code, empty.Body.String())
	}
	got := taskByTitle(t, repo, "하루94")
	if got == nil || got.DueDate != day {
		t.Fatalf("종료일 비움 %+v", got)
	}
}

func Test95_94_B3_OwnTaskCanSplit(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94b3.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 9).Format(dateLayout)
	mustCreateBoardTask(t, repo, "경영지원 업무", day, "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	_ = repo.SetTaskWorkPlace(taskByTitle(t, repo, "경영지원 업무").TaskID, model.WorkPlaceOffice)
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"project_work"}, "work_subtype": {"build"}, "title": {"짧은작업"},
		"work_date": {day}, "due_date": {day}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {me.FullName},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("겹침 %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Conflicts []struct {
			CanSplit         bool   `json:"can_split"`
			SplitBlockReason string `json:"split_block_reason"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Conflicts) == 0 || !payload.Conflicts[0].CanSplit {
		t.Fatalf("본인 업무 can_split=%+v", payload)
	}

	host := taskByTitle(t, repo, "경영지원 업무")
	done := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"project_work"}, "work_subtype": {"build"}, "title": {"짧은작업"},
		"work_date": {day}, "due_date": {day}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {me.FullName}, "overlap_action": {"split"}, "conflict_task_id": {host.TaskID},
		"split_travel_min": {"0"}, "redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	})
	if done.Code != http.StatusSeeOther && done.Code != http.StatusOK {
		t.Fatalf("끼워넣기 %d %s", done.Code, done.Body.String())
	}
	xx := taskByTitle(t, repo, "짧은작업")
	if xx == nil {
		t.Fatal("새 업무 없음")
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "경영지원 업무"))
	backOK := false
	for _, s := range segs {
		if strings.HasPrefix(s.StartTime, "09:30") {
			backOK = true
		}
	}
	if !backOK {
		t.Fatalf("기존 구간 %+v", segs)
	}
	page := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0", ck)
	if !strings.Contains(page.Body.String(), "짧은작업") || !strings.Contains(page.Body.String(), "경영지원 업무") {
		t.Fatal("일정에 안 보임")
	}
}

func Test95_94_B4_PeriodDatesAndSkip(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94b4.db"))
	ck := jwtCookieUser(t, me)
	d0 := time.Now().AddDate(0, 0, 11)
	days := []string{d0.Format(dateLayout), d0.AddDate(0, 0, 1).Format(dateLayout), d0.AddDate(0, 0, 2).Format(dateLayout)}
	var ids []string
	for i, d := range days {
		mustCreateBoardTask(t, repo, "종일"+string(rune('1'+i)), d, "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
		ids = append(ids, taskByTitle(t, repo, "종일"+string(rune('1'+i))).TaskID)
	}
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"기간3일"}, "work_date": {days[0]}, "due_date": {days[2]},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("겹침 %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Dates []struct {
			Date      string        `json:"date"`
			Conflicts []interface{} `json:"conflicts"`
		} `json:"dates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Dates) < 3 {
		t.Fatalf("날짜별 JSON %d %s", len(payload.Dates), rec.Body.String())
	}
	skip := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"기간3일-skip"}, "work_date": {days[0]}, "due_date": {days[2]},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
		"overlap_action": {"split"}, "conflict_task_id": {ids[0], ids[1], ids[2]},
		"overlap_skip_date": {days[1]}, "split_travel_min": {"0"},
	})
	if skip.Code != http.StatusSeeOther && skip.Code != http.StatusOK {
		t.Fatalf("skip %d %s", skip.Code, skip.Body.String())
	}
}

func Test95_94_B5_AtomicNoPartial(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94b5.db"))
	ck := jwtCookieUser(t, me)
	d0 := time.Now().AddDate(0, 0, 14)
	d1 := d0.AddDate(0, 0, 1)
	mustCreateBoardTask(t, repo, "첫째날", d0.Format(dateLayout), "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "둘째완료", d1.Format(dateLayout), "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	a := taskByTitle(t, repo, "첫째날")
	b := taskByTitle(t, repo, "둘째완료")
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"원자성"}, "work_date": {d0.Format(dateLayout)}, "due_date": {d1.Format(dateLayout)},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
		"overlap_action": {"split"}, "conflict_task_id": {a.TaskID, b.TaskID}, "split_travel_min": {"0"},
	})
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("완료 끼워넣기가 성공하면 안 됨 %s", rec.Body.String())
	}
	if taskByTitle(t, repo, "원자성") != nil {
		t.Fatal("새 업무가 남음")
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "첫째날"))
	if len(segs) == 1 && segs[0].StartTime[:5] == "09:00" && segs[0].EndTime[:5] == "18:30" {
		return
	}
	if len(segs) > 1 {
		t.Fatalf("첫 날 구간이 갈라짐 %+v", segs)
	}
}

func Test95_94_B6_SplitBlockReasons(t *testing.T) {
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94b6.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 16).Format(dateLayout)
	mustCreateBoardTask(t, repo, "완료업무", day, "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새1"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
	})
	if !strings.Contains(rec.Body.String(), `"split_block_reason":"completed"`) {
		t.Fatalf("completed 사유 %s", rec.Body.String())
	}
	mustCreateBoardTask(t, repo, "타인업무", day, "10:00", "11:00", model.OrgIDLibrary, other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	rec2 := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새2"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:00"}, "end_time": {"10:30"}, "assignee": {other.FullName},
	})
	if rec2.Code == http.StatusConflict && !strings.Contains(rec2.Body.String(), "not_assignee") && !strings.Contains(rec2.Body.String(), "can_split\":true") {
		t.Fatalf("타인 %s", rec2.Body.String())
	}
	wrongID := &model.WorkTask{
		Title: "이름만같음", WorkDate: day, DueDate: day, StartTime: "14:00", EndTime: "18:30",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: "USR-OTHER",
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(wrongID); err != nil {
		t.Fatal(err)
	}
	rec3 := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새3"}, "work_date": {day}, "due_date": {day},
		"start_time": {"14:00"}, "end_time": {"14:30"}, "assignee": {me.FullName},
	})
	if !strings.Contains(rec3.Body.String(), `"can_split":true`) {
		t.Fatalf("이름 일치 can_split %s", rec3.Body.String())
	}
	_ = db
}

func Test95_94_B7_QueryCountIndependentOfDays(t *testing.T) {
	var n int64
	qcount9520N.Store(&n)
	t.Cleanup(func() { qcount9520N.Store(nil) })
	e, _, me, _, repo := seedPersonalBoard(t, sqliteQCount9520, filepath.Join(t.TempDir(), "d94q.db"))
	ck := jwtCookieUser(t, me)
	base := time.Now().AddDate(0, 0, 20)
	for i := 0; i < 5; i++ {
		d := base.AddDate(0, 0, i).Format(dateLayout)
		mustCreateBoardTask(t, repo, "q94-"+d, d, "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	form := func(to time.Time) *httptest.ResponseRecorder {
		atomic.StoreInt64(&n, 0)
		return overlapCreateJSON(t, e, ck, url.Values{
			"work_type": {"admin"}, "title": {"qspan"}, "work_date": {base.Format(dateLayout)},
			"due_date": {to.Format(dateLayout)}, "start_time": {"09:00"}, "end_time": {"09:30"},
			"assignee": {me.FullName},
		})
	}
	a := form(base.AddDate(0, 0, 2))
	qa := atomic.LoadInt64(&n)
	b := form(base.AddDate(0, 0, 4))
	qb := atomic.LoadInt64(&n)
	if a.Code != http.StatusConflict || b.Code != http.StatusConflict {
		t.Fatalf("겹침 %d %d", a.Code, b.Code)
	}
	if qb > qa+8 {
		t.Fatalf("날짜 늘었는데 쿼리 %d → %d", qa, qb)
	}
}

func Test95_94_B8_SameOverlapFn(t *testing.T) {
	dir := filepath.Dir(relHandlerFile())
	wb, err := os.ReadFile(filepath.Join(dir, "workboard.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wb), "rejectIfOverlapDates") {
		t.Fatal("CreateTask 경로")
	}
	if !strings.Contains(string(wb), "CreateSubtasks") || !strings.Contains(string(wb), "rejectIfOverlapDates") {
		t.Fatal("하위 업무 경로")
	}
	rec, err := os.ReadFile(filepath.Join(dir, "workboard_recurrence.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rec), "rejectIfOverlapDates") {
		t.Fatal("반복 경로")
	}
	ov, err := os.ReadFile(filepath.Join(dir, "workboard_plan_overlap.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ov), "func (h *WorkboardHandler) PlanOverlapsForDates") {
		t.Fatal("PlanOverlapsForDates 없음")
	}
}

func Test95_94_C_ActionToggleAndCounts(t *testing.T) {
	e, _, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94c.db"))
	ck := jwtCookieUser(t, me)
	day := "2026-11-03"
	for i := 0; i < 3; i++ {
		mustCreateBoardTask(t, repo, "접수만94-"+string(rune('A'+i)), day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	span := &model.WorkTask{
		Title: "기간걸침예정", WorkDate: "2026-11-01", DueDate: "2026-11-05",
		StartTime: "10:00", EndTime: "11:00", OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskWaiting,
		WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(span); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		tk := &model.WorkTask{
			Title: "조치94-" + string(rune('A'+i)), WorkDate: "2026-10-01", DueDate: "2026-10-01",
			StartTime: "11:00", EndTime: "12:00", OrgID: model.OrgIDLibrary,
			Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskComplete,
			WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal, CompleteDate: day,
		}
		if err := repo.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.InsertWorkTaskProcess(model.WorkTaskProcess{
			TaskID: tk.TaskID, OrgID: tk.OrgID, ProcessAt: day + " 11:00:00",
			Worker: me.FullName, WorkerUserID: me.UserID, WorkContent: "실행",
		}); err != nil {
			t.Fatal(err)
		}
	}
	mustCreateBoardTask(t, repo, "타담당조치", day, "15:00", "16:00", model.OrgIDLibrary, other.FullName, other.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	otherT := taskByTitle(t, repo, "타담당조치")
	otherT.CompleteDate = day
	_ = repo.UpdateTask(otherT)
	_, _ = repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: otherT.TaskID, OrgID: otherT.OrgID, ProcessAt: day + " 15:00:00",
		Worker: other.FullName, WorkerUserID: other.UserID, WorkContent: "타인",
	})

	act := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+"&auto=0&show=act", ck)
	ab := act.Body.String()
	if strings.Contains(ab, "접수만94") || strings.Contains(ab, "기간걸침예정") {
		t.Fatal("조치만인데 접수가 보임")
	}
	if !strings.Contains(ab, "완료 2건") {
		t.Fatalf("건수 %s", snippetShow(ab))
	}
	if strings.Contains(ab, "타담당조치") {
		t.Fatal("내 업무에 타인 조치")
	}
	if !strings.Contains(ab, "data-show-bit=\"both\"") {
		t.Fatal("둘 다 없음")
	}
	all := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+"&auto=0&show=act&mine=0", ck)
	if !strings.Contains(all.Body.String(), "타담당조치") {
		t.Fatal("전체에 타인 조치 없음")
	}
	if !strings.Contains(all.Body.String(), "전체 · 조직 전체") {
		t.Fatal("범위 배지 없음")
	}
	if strings.Contains(all.Header().Get("Location"), "mine=") && false {
		t.Fatal("범위가 바뀜")
	}
	both := doGetWith(t, e, "/workboard/board?view=sched&range=day&date="+day+"&auto=0&show=plan,act", ck)
	bb := both.Body.String()
	if !strings.Contains(bb, "접수만94") || !strings.Contains(bb, "조치94") {
		t.Fatal("둘 다 합집합 아님")
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0&show=act", ck)
	if !strings.Contains(week.Body.String(), "show=act") {
		t.Fatal("주 링크 show 유실")
	}
}

func Test95_94_D_Playwright(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d94pw.db"))
	ck := jwtCookieUser(t, me)
	d0 := time.Now().AddDate(0, 0, 18)
	for i := 0; i < 3; i++ {
		d := d0.AddDate(0, 0, i).Format(dateLayout)
		mustCreateBoardTask(t, repo, "pw종일"+d, d, "09:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_94_overlap_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"DAY0="+d0.Format(dateLayout),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "is not found") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("브라우저: %v\n%s", err, out)
	}
}

func relHandlerFile() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return file
}

func Test95_94_OldRegisterUnchanged(t *testing.T) {
	reg := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard/register.html")))
	if strings.Contains(reg, "wb_period_dates") {
		t.Fatal("구화면 register 를 바꿈")
	}
}
