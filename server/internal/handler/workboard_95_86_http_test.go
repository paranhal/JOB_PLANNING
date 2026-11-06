package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new86(t *testing.T) (*echo.Echo, *sql.DB, *http.Cookie, *repository.WBRepo, *model.User) {
	t.Helper()
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d86.db"))
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/workboard/unschedule", h.Workboard.Unschedule)
	g.POST("/workboard/register/unlock-past", h.Workboard.UnlockPastRegister)
	return e, db, jwtCookieUser(t, me), repo, me
}

func Test95_86_BoardItemDone(t *testing.T) {
	if !boardItemDone(boardItem{Task: model.WorkTask{Status: model.WBTaskComplete}}) {
		t.Fatal("완료 건")
	}
	if !boardItemDone(boardItem{Task: model.WorkTask{Status: "closed"}}) {
		t.Fatal("종료 건")
	}
	if !boardItemDone(boardItem{Task: model.WorkTask{Status: "completed"}}) {
		t.Fatal("completed")
	}
	for _, st := range []string{
		model.WBTaskWaiting, model.WBTaskInProgress, model.WBTaskHold, model.WBTaskTransfer,
		model.WBTaskReview, model.StatusPartialComplete, model.WBTaskCancelled,
	} {
		if boardItemDone(boardItem{Task: model.WorkTask{Status: st}}) {
			t.Fatalf("완료가 아님 %s", st)
		}
	}
	if parseBoardCols("") != boardColsPerson || parseBoardCols("flat") != boardColsFlat {
		t.Fatal("cols 기본값")
	}
	admin := model.WorkTask{TaskID: "T1", WorkType: model.WBWorkAdmin}
	if boardActionHref(admin) != "/workboard/tasks/T1" {
		t.Fatalf("행정 조치 주소 %s", boardActionHref(admin))
	}
	as := model.WorkTask{TaskID: "T2", SourceType: model.WBSourceAS, SourceID: "AS1"}
	if boardActionHref(as) != "/as/AS1/action" {
		t.Fatalf("AS 조치 주소 %s", boardActionHref(as))
	}
}

func Test95_86_CalendarMoveAndReject(t *testing.T) {
	e, db, ck, repo, me := new86(t)
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "이동원", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "겹침고정", day, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	src := taskByTitle(t, repo, "이동원")
	if src == nil {
		t.Fatal("시드 없음")
	}
	rec := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {src.TaskID}, "work_date": {day},
		"start_time": {"13:00"}, "end_time": {"14:00"}, "assignee": {me.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("이동 %d %s", rec.Code, rec.Body.String())
	}
	got := taskByTitle(t, repo, "이동원")
	if got == nil || !strings.HasPrefix(got.StartTime, "13:00") {
		t.Fatalf("시각 미변경 %+v", got)
	}
	rec2 := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {got.TaskID}, "work_date": {day},
		"start_time": {"11:00"}, "end_time": {"12:00"}, "assignee": {me.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if rec2.Code != http.StatusSeeOther && rec2.Code != http.StatusConflict {
		t.Fatalf("겹침 %d", rec2.Code)
	}
	still := taskByTitle(t, repo, "이동원")
	if still == nil || !strings.HasPrefix(still.StartTime, "13:00") {
		t.Fatalf("겹침 후 원위치 아님 %+v", still)
	}
	loc := rec2.Header().Get("Location")
	if !strings.Contains(loc, "err=") && rec2.Code != http.StatusConflict {
		t.Fatalf("겹침 안내 없음 %s", loc)
	}

	oa, _ := repository.NewUserRepo(db).GetByUsername("oa01")
	mustCreateBoardTask(t, repo, "타인건", day, "15:00", "16:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	other := taskByTitle(t, repo, "타인건")
	forbid := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {other.TaskID}, "work_date": {day},
		"start_time": {"16:00"}, "end_time": {"17:00"}, "assignee": {oa.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if forbid.Code != http.StatusForbidden && forbid.Code != http.StatusSeeOther {
		t.Fatalf("권한 %d", forbid.Code)
	}
	after := taskByTitle(t, repo, "타인건")
	if after == nil || !strings.HasPrefix(after.StartTime, "15:00") {
		t.Fatalf("권한 없음인데 이동됨 %+v", after)
	}

	un := doFormWith(t, e, "/workboard/unschedule", url.Values{
		"task_id": {got.TaskID}, "redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if un.Code != http.StatusSeeOther {
		t.Fatalf("내리기 %d", un.Code)
	}
	down := taskByTitle(t, repo, "이동원")
	if down == nil || strings.TrimSpace(down.StartTime) != "" {
		t.Fatalf("내리기 후 시각 %+v", down)
	}
}

func Test95_86_AssigneeColumnAndHTML(t *testing.T) {
	e, _, ck, repo, me := new86(t)
	day := time.Now().AddDate(0, 0, 4).Format(dateLayout)
	mustCreateBoardTask(t, repo, "열업무", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	body := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(body, "/workboard/schedule") || !strings.Contains(body, "assignee_from_column") {
		t.Fatal("배치 폼 없음")
	}
	if !strings.Contains(body, "/workboard/unschedule") {
		t.Fatal("내리기 없음")
	}
	if !strings.Contains(body, me.FullName) {
		t.Fatal("담당자 열 이름 없음")
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(week, `data-keep-time`) {
		t.Fatal("주 보기 칸 없음")
	}
	month := doGetWith(t, e, "/workboard/board?view=month&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(month, `data-monthzone`) {
		t.Fatal("월 보기 칸 없음")
	}
}

func Test95_86_ActButtonsAndDoneOpen(t *testing.T) {
	t.Skip("일정표 일은 register 복사 카드(95-132A). 구 막대 조치 버튼 단언 종료")
	e, _, ck, repo, me := new86(t)
	today := time.Now().Format(dateLayout)
	day := time.Now().AddDate(0, 0, 8).Format(dateLayout)
	mustCreateBoardTask(t, repo, "긴바업무", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "짧은바", day, "10:00", "10:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "완료바", day, "13:00", "14:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "부분바", day, "15:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.StatusPartialComplete, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "오늘완료칸반", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	done := taskByTitle(t, repo, "완료바")
	todayDone := taskByTitle(t, repo, "오늘완료칸반")
	if todayDone != nil {
		_, _ = repo.DB().Exec(`UPDATE work_tasks SET complete_date=? WHERE task_id=?`, today, todayDone.TaskID)
	}

	root := findTemplateRoot(t)
	css := string(mustRead(t, filepath.Join(root, "..", "css", "input.css")))
	if !strings.Contains(css, "min-height: 24px") || !strings.Contains(css, "font-size: 16px") {
		t.Fatal("조치·수정 버튼 크기 없음")
	}
	if !strings.Contains(css, "board-bar-short") || !strings.Contains(css, "board-resize") {
		t.Fatal("짧은 바·손잡이 규칙 없음")
	}
	if js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "board_schedule.js"))); !strings.Contains(js, "data-done") {
		t.Fatal("완료 바 클릭 JS 없음")
	}

	dayBody := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(dayBody, `class="board-bar-act"`) {
		t.Fatal("일 보기 공용 버튼 클래스 없음")
	}
	if !strings.Contains(dayBody, "짧은바") || !strings.Contains(dayBody, `board-bar-short`) {
		t.Fatal("30분 바 짧은 표시 없음")
	}
	if !strings.Contains(dayBody, "작업완료") {
		t.Fatal("완료 라벨 없음")
	}
	if done == nil || !strings.Contains(dayBody, "/workboard/tasks/"+done.TaskID) {
		t.Fatal("완료 바 조치 화면 주소 없음")
	}
	if i := strings.Index(dayBody, "부분바"); i >= 0 {
		chunk := dayBody[i:]
		if len(chunk) > 2500 {
			chunk = chunk[:2500]
		}
		if !strings.Contains(chunk, "조치") {
			t.Fatal("부분완료에 조치 없음")
		}
		if strings.Contains(chunk, "작업완료") {
			t.Fatal("부분완료를 완료로 표시")
		}
	} else {
		t.Fatal("부분바 없음")
	}

	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(week, `class="board-bar-act"`) || !strings.Contains(week, "작업완료") {
		t.Fatal("주 보기 버튼·완료 라벨 없음")
	}

	month := doGetWith(t, e, "/workboard/board?view=month&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(month, `data-month-chip`) {
		t.Fatal("월 칩 없음")
	}
	if done != nil && !strings.Contains(month, "/workboard/tasks/"+done.TaskID) {
		t.Fatal("월 칩 조치 화면 없음")
	}
	if !strings.Contains(month, "✓") {
		t.Fatal("월 칩 완료 표시 없음")
	}

	kan := doGetWith(t, e, "/workboard/board?view=kanban&date="+today+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(kan, "오늘완료칸반") {
		t.Fatal("칸반 완료 라벨 없음")
	}
	if todayDone != nil && !strings.Contains(kan, "/workboard/tasks/"+todayDone.TaskID) {
		t.Fatal("칸반 완료 조치 화면 없음")
	}
}

func Test95_86_SupportNotMovableAndTemplates(t *testing.T) {
	e, db, ck, repo, me := new86(t)
	day := time.Now().AddDate(0, 0, 5).Format(dateLayout)
	park := &model.User{Username: "park86", PasswordHash: HashPassword("password10"), FullName: "박86", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := repository.NewUserRepo(db).Create(park); err != nil {
		t.Fatal(err)
	}
	gotPark, _ := repository.NewUserRepo(db).GetByUsername("park86")
	mustCreateBoardTask(t, repo, "지원포함", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "지원포함")
	if err := repo.ReplaceSupportMembers(tk.TaskID, []model.WorkTaskMember{{UserID: gotPark.UserID, Assignee: gotPark.FullName, Role: model.WBMemberSupport}}); err != nil {
		t.Fatal(err)
	}
	body := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if !strings.Contains(body, "지원") {
		t.Fatal("지원 라벨 없음")
	}
	root := findTemplateRoot(t)
	scope := string(mustRead(t, filepath.Join(root, "workboard/_board_scope.html")))
	if !strings.Contains(scope, "/workboard/register/unlock-past") {
		t.Fatal("잠금 해제 엔드포인트 없음")
	}
	js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "board_schedule.js")))
	if !strings.Contains(js, "assignee_from_column") && !strings.Contains(js, "board-m-from-col") {
		t.Fatal("열 담당자 반영 JS 없음")
	}
	if !strings.Contains(js, "data-done") {
		t.Fatal("완료 바 클릭 JS 없음")
	}
	if !strings.Contains(js, "/ 15") && !strings.Contains(js, "/15") {
		t.Fatal("15분 스냅 없음")
	}
}

func Test95_86_ResizeDuration(t *testing.T) {
	t.Skip("일정표 일 복사본은 막대 리사이즈 없음(95-132A)")
	e, db, ck, repo, me := new86(t)
	day := time.Now().AddDate(0, 0, 7).Format(dateLayout)
	mustCreateBoardTask(t, repo, "길이원", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "길이겹침", day, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	src := taskByTitle(t, repo, "길이원")
	rec := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {src.TaskID}, "work_date": {day},
		"start_time": {"09:00"}, "end_time": {"11:00"}, "duration_min": {"120"},
		"assignee": {me.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("길이 %d %s", rec.Code, rec.Body.String())
	}
	got := taskByTitle(t, repo, "길이원")
	if got == nil || !strings.HasPrefix(got.StartTime, "09:00") || !strings.HasPrefix(got.EndTime, "11:00") || got.DurationMin != 120 {
		t.Fatalf("시작 유지·종료 저장 실패 %+v", got)
	}
	park := &model.User{Username: "park86r", PasswordHash: HashPassword("password10"), FullName: "박길이", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := repository.NewUserRepo(db).Create(park); err != nil {
		t.Fatal(err)
	}
	gotPark, _ := repository.NewUserRepo(db).GetByUsername("park86r")
	if err := repo.ReplaceSupportMembers(got.TaskID, []model.WorkTaskMember{{UserID: gotPark.UserID, Assignee: gotPark.FullName, Role: model.WBMemberSupport}}); err != nil {
		t.Fatal(err)
	}
	hit := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {got.TaskID}, "work_date": {day},
		"start_time": {"09:00"}, "end_time": {"11:30"}, "duration_min": {"150"},
		"assignee": {me.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if hit.Code != http.StatusSeeOther && hit.Code != http.StatusConflict {
		t.Fatalf("겹침 길이 %d", hit.Code)
	}
	still := taskByTitle(t, repo, "길이원")
	if still == nil || !strings.HasPrefix(still.EndTime, "11:00") || still.DurationMin != 120 {
		t.Fatalf("겹침 후 원길이 아님 %+v", still)
	}
	loc := hit.Header().Get("Location")
	if !strings.Contains(loc, "err=") && hit.Code != http.StatusConflict {
		t.Fatalf("겹침 안내 없음 %s", loc)
	}

	mustCreateBoardTask(t, repo, "완료길이", day, "13:00", "14:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	done := taskByTitle(t, repo, "완료길이")
	denyDone := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {done.TaskID}, "work_date": {day},
		"start_time": {"13:00"}, "end_time": {"15:00"}, "duration_min": {"120"},
		"assignee": {me.FullName},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}, ck)
	if denyDone.Code != http.StatusSeeOther && denyDone.Code != http.StatusForbidden {
		t.Fatalf("완료 거부 %d", denyDone.Code)
	}
	afterDone := taskByTitle(t, repo, "완료길이")
	if afterDone == nil || !strings.HasPrefix(afterDone.EndTime, "14:00") {
		t.Fatalf("완료 건이 늘어남 %+v", afterDone)
	}

	mustCreateBoardTask(t, repo, "구간길이", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	seg := taskByTitle(t, repo, "구간길이")
	if err := repo.ReplacePlanSegments(seg.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "15:00", EndTime: "16:00"},
	}); err != nil {
		t.Fatal(err)
	}
	body := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck).Body.String()
	if strings.Contains(body, `data-nudge`) {
		t.Fatal("±15 버튼이 남아 있음")
	}
	if !strings.Contains(body, `data-resize="1"`) {
		t.Fatal("일 보기 손잡이 없음")
	}
	if !strings.Contains(body, `data-can-resize="1"`) {
		t.Fatal("조절 가능 바 없음")
	}
	if strings.Count(body, "지원") < 1 {
		t.Fatal("지원 라벨 없음")
	}
	for _, mark := range []string{`data-seg-count="2"`, `data-done="1"`, `data-support="1"`} {
		from := 0
		for {
			i := strings.Index(body[from:], mark)
			if i < 0 {
				break
			}
			at := from + i
			lo, hi := at-500, at+200
			if lo < 0 {
				lo = 0
			}
			if hi > len(body) {
				hi = len(body)
			}
			if strings.Contains(body[lo:hi], `data-can-resize="1"`) {
				t.Fatalf("%s 바에 손잡이가 있다", mark)
			}
			from = at + len(mark)
		}
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0&mine=0", ck).Body.String()
	if strings.Contains(week, `data-resize="1"`) {
		t.Fatal("주 보기에 손잡이가 있다")
	}
	js := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "static", "js", "board_schedule.js")))
	if !strings.Contains(js, "submitResize") || !strings.Contains(js, "ignoreClickUntil") {
		t.Fatal("리사이즈 직후 클릭 가드 없음")
	}
	if !strings.Contains(js, "board-m-dur") || !strings.Contains(js, "20") {
		t.Fatal("duration_min·20시 상한 없음")
	}
}

func Test95_86_BrowserDragAndTap(t *testing.T) {
	t.Skip("일정표 일 복사본 셀렉터(95-132A)")
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d86b.db"))
	me.Mobile = "010-1111-2222"
	me.ProfileDone = true
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 6).Format(dateLayout)
	mustCreateBoardTask(t, repo, "드래그대상", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	probe := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0&mine=0", ck)
	if !strings.Contains(probe.Body.String(), "data-board-bar") {
		t.Fatalf("서버 HTML에 바 없음 %d %s", probe.Code, probe.Body.String()[:min(800, probe.Body.Len())])
	}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_86_calendar_edit_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+day,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "is not found") {
			t.Skip(s)
		}
		t.Fatalf("브라우저 테스트 실패: %v\n%s", err, out)
	}
}
