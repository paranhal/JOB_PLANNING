package handler

import (
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
)

func Test95_78_SplitInsertAndShift(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 9).Format(dateLayout)
	mustCreateBoardTask(t, repo, "OO", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oo := taskByTitle(t, repo, "OO")
	if oo == nil {
		t.Fatal("OO 없음")
	}
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"XX"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:00"}, "end_time": {"14:00"}, "assignee": {me.FullName},
		"overlap_action": {"split"}, "conflict_task_id": {oo.TaskID}, "split_travel_min": {"0"},
		"redirect": {"/workboard/board?view=day&date=" + day},
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("끼워넣기 %d %s", rec.Code, rec.Body.String())
	}
	xx := taskByTitle(t, repo, "XX")
	if xx == nil {
		t.Fatal("XX 없음")
	}
	segs, err := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "OO"))
	if err != nil || len(segs) != 2 {
		t.Fatalf("OO 구간 %d err=%v", len(segs), err)
	}
	if segs[0].StartTime[:5] != "09:00" || segs[0].EndTime[:5] != "10:00" {
		t.Fatalf("앞 구간 %+v", segs[0])
	}
	if segs[1].StartTime[:5] != "14:00" || segs[1].EndTime[:5] != "18:00" {
		t.Fatalf("뒤 구간 %+v", segs[1])
	}
	if err := repo.ReplaceActualSegments(xx.TaskID, []model.WorkTaskSegment{{
		Kind: model.SegKindActual, WorkDate: day, StartTime: "10:30", EndTime: "14:30",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ShiftPlanAfter(taskByTitle(t, repo, "OO"), "14:30", xx.TaskID); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "OO"))
	if len(after) != 2 || after[0].EndTime[:5] != "10:00" || after[1].StartTime[:5] != "14:45" && after[1].StartTime[:5] != "15:00" {
		if len(after) == 2 {
			if after[1].StartTime[:5] != "14:45" && after[1].StartTime[:5] != "15:00" {
				t.Fatalf("후속 구간 시작 %s want 14:45/15:00", after[1].StartTime)
			}
		} else {
			t.Fatalf("후속 %+v", after)
		}
	}

	hosts, _ := repo.TasksInterruptedBy(xx.TaskID)
	if len(hosts) == 0 {
		t.Fatal("interrupt 연결 없음")
	}
	if err := repo.DeleteTask(xx.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MergeInterruptSegments(oo.TaskID, xx.TaskID); err != nil {
		t.Fatal(err)
	}
	merged, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "OO"))
	if len(merged) != 1 {
		t.Fatalf("합친 뒤 %d", len(merged))
	}
}

func Test95_78_CaseDNoOverlap(t *testing.T) {
	e, _, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78d.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 10).Format(dateLayout)
	mustCreateBoardTask(t, repo, "다른사람", day, "09:00", "18:00", "O02", other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	dup := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"내계획"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:00"}, "end_time": {"14:00"}, "assignee": {me.FullName},
		"redirect": {"/workboard/board"},
	})
	if dup.Code == http.StatusConflict {
		t.Fatalf("작업자 다른데 겹침 모달 %s", dup.Body.String())
	}
}

func Test95_78_CrossTypeSplitInsert(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78x.db"))
	ck := jwtCookieUser(t, me)

	hosts := []struct {
		title, workType, sourceType, sourceID string
	}{
		{"호스트AS", model.WBWorkAS, model.WBSourceAS, "AS-78-host"},
		{"호스트점검", model.WBWorkMaintenance, model.WBSourceMaintenance, "MNT-78-host"},
		{"호스트작업", model.WBWorkProjectWork, "", ""},
		{"호스트지원", model.WBWorkSupport, "", ""},
		{"호스트행정", model.WBWorkAdmin, "", ""},
	}
	for i, h := range hosts {
		day := time.Now().AddDate(0, 0, 16+i).Format(dateLayout)
		tk := &model.WorkTask{
			Title: h.title, WorkType: h.workType, SourceType: h.sourceType, SourceID: h.sourceID,
			Status: model.WBTaskWaiting, Assignee: me.FullName, AssigneeUserID: me.UserID,
			WorkDate: day, DueDate: day, StartTime: "09:00", EndTime: "18:00", DurationMin: 540,
			OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
		}
		if err := repo.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
		got := taskByTitle(t, repo, h.title)
		probe := overlapCreateJSON(t, e, ck, url.Values{
			"work_type": {"admin"}, "title": {"겹침확인" + h.title}, "work_date": {day}, "due_date": {day},
			"start_time": {"10:00"}, "end_time": {"11:00"}, "assignee": {me.FullName},
			"redirect": {"/workboard/board"},
		})
		if probe.Code != http.StatusConflict {
			t.Fatalf("%s 유형 간 겹침 미검출 %d %s", h.workType, probe.Code, probe.Body.String())
		}
		if !strings.Contains(probe.Body.String(), `"can_split":true`) {
			t.Fatalf("%s 끼워넣기 불가 %s", h.workType, probe.Body.String())
		}
		rec := overlapCreateJSON(t, e, ck, url.Values{
			"work_type": {"admin"}, "title": {"끼움" + h.title}, "work_date": {day}, "due_date": {day},
			"start_time": {"10:00"}, "end_time": {"11:00"}, "assignee": {me.FullName},
			"overlap_action": {"split"}, "conflict_task_id": {got.TaskID}, "split_travel_min": {"0"},
			"redirect": {"/workboard/board"},
		})
		if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
			t.Fatalf("%s 나누기 %d %s", h.workType, rec.Code, rec.Body.String())
		}
		segs, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, h.title))
		if len(segs) != 2 {
			t.Fatalf("%s 구간 %d (i=%d)", h.workType, len(segs), i)
		}
	}

	day := time.Now().AddDate(0, 0, 22).Format(dateLayout)
	mustCreateBoardTask(t, repo, "행정호스트", day, "12:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	host := taskByTitle(t, repo, "행정호스트")
	as := &model.WorkTask{
		WorkType: model.WBWorkAS, SourceType: model.WBSourceAS, SourceID: "AS-78-insert",
		Title: "끼울AS", Status: model.WBTaskWaiting, Assignee: me.FullName, AssigneeUserID: me.UserID,
		WorkDate: day, DueDate: day, OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(as); err != nil {
		t.Fatal(err)
	}
	asGot := taskByTitle(t, repo, "끼울AS")
	place := doFormJSONWith(t, e, "/workboard/schedule", url.Values{
		"kind": {model.WBSourceAS}, "ref_id": {asGot.SourceID}, "work_date": {day},
		"start_time": {"13:00"}, "end_time": {"14:00"}, "assignee": {me.FullName},
		"overlap_action": {"split"}, "conflict_task_id": {host.TaskID}, "split_travel_min": {"0"},
	}, ck)
	if place.Code != http.StatusSeeOther && place.Code != http.StatusOK {
		t.Fatalf("AS 끼워넣기 배치 %d %s", place.Code, place.Body.String())
	}
	hSegs, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "행정호스트"))
	if len(hSegs) != 2 {
		t.Fatalf("행정에 AS 끼운 뒤 구간 %d", len(hSegs))
	}
}

func Test95_78_PartialMoveDeparture(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78p.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 11).Format(dateLayout)
	mustCreateBoardTask(t, repo, "공동OO", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oo := taskByTitle(t, repo, "공동OO")
	_ = repo.ReplaceSupportMembers(oo.TaskID, []model.WorkTaskMember{{Assignee: "김지원", Role: model.WBMemberSupport}})
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"일부이동"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:00"}, "end_time": {"12:00"}, "assignee": {me.FullName},
		"overlap_action": {"split"}, "conflict_task_id": {oo.TaskID}, "split_travel_min": {"0"},
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("일부 이동 %d %s", rec.Code, rec.Body.String())
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*taskByTitle(t, repo, "공동OO"))
	if len(segs) != 1 {
		t.Fatalf("공동 구간이 쪼개지면 안 됨 %d", len(segs))
	}
	deps, _ := repo.ListDepartures(oo.TaskID)
	if len(deps) == 0 {
		t.Fatal("이탈 기록 없음")
	}
}

func Test95_78_IncompleteItemsForcePartial(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "항목작업")
	_ = repo.ReplaceItems(id, []model.WorkTaskItem{{Title: "배선", Status: model.ItemTodo}, {Title: "마감", Status: model.ItemDone}})
	form := pwActionForm()
	form.Set("item_title", "배선")
	form.Add("item_title", "마감")
	form.Set("item_status", "todo")
	form.Add("item_status", "done")
	form.Set("create_followup", "1")
	form.Set("next_date", "2026-10-22")
	form.Set("followup_note", "남음")
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 %d %s", rec.Code, rec.Body.String())
	}
	tk, _ := repo.GetTask(id)
	if tk.Status != model.StatusPartialComplete {
		t.Fatalf("부분완료가 아님 %s", tk.Status)
	}
}

func Test95_78_HTMLSplitAndSegments(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout/base.html")))
	if !strings.Contains(base, `data-overlap-action="split"`) || !strings.Contains(base, "끼워 넣기") {
		t.Fatal("겹침 모달 끼워 넣기 없음")
	}
	if !strings.Contains(base, "overlap-travel-min") {
		t.Fatal("이동 시간 없음")
	}
	day := string(mustRead(t, filepath.Join(root, "workboard/board_day.html")))
	if !strings.Contains(day, "SegLabel") && !strings.Contains(day, ".SegLabel") {
		t.Fatal("일 보기 구간 표시 없음")
	}
	list := string(mustRead(t, filepath.Join(root, "workboard/board_list.html")))
	if !strings.Contains(list, "┗") {
		t.Fatal("리스트 조각 행 없음")
	}
	act := string(mustRead(t, filepath.Join(root, "workboard/project_work_action.html")))
	if !strings.Contains(act, "work_segments_form") && !strings.Contains(act, "작업 구간") {
		t.Fatal("작업 구간 표 없음")
	}
	if !strings.Contains(act, "work_task_items") && !strings.Contains(act, "data-work-items") {
		t.Fatal("작업 리스트 없음")
	}
	js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "overlap_resolve.js")))
	if !strings.Contains(js, "split_travel_min") {
		t.Fatal("JS 이동 시간 없음")
	}
}

func Test95_78_BoardCountStaysTaskBased(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78c.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 12).Format(dateLayout)
	mustCreateBoardTask(t, repo, "막대OO", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oo := taskByTitle(t, repo, "막대OO")
	_ = repo.ReplacePlanSegments(oo.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00"},
	})
	page := doGetWith(t, e, "/workboard/board?view=list&date="+day+"&auto=0", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("list %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "전체 1건") && !strings.Contains(body, "총 1개의 업무") {
		t.Fatal("건수가 업무 1건이 아님")
	}
	if !strings.Contains(body, "┗") && !strings.Contains(body, `data-seg-frag="1"`) {
		t.Fatal("접힌 조각 행 없음")
	}
}

func Test95_78_BrowserInsert(t *testing.T) {
	skipIfNoPlaywrightBrowser(t)
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78b.db"))
	me.Mobile = "010-1111-2222"
	me.ProfileDone = true
	ck := jwtCookieVerified(t, me)
	day := time.Now().AddDate(0, 0, 13).Format(dateLayout)
	mustCreateBoardTask(t, repo, "브OO", day, "09:00", "18:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	e.GET("/__t78/form", func(c echo.Context) error {
		return c.HTML(http.StatusOK, overlap78FixtureHTML(day, me.FullName, me.UserID))
	})
	e.GET("/workboard/board", func(c echo.Context) error {
		return hBoardOK(c)
	})
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_78_overlap_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+day,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("브라우저 테스트 실패: %v\n%s", err, out)
	}
	if taskByTitle(t, repo, "브XX") == nil {
		t.Fatal("브라우저 끼워넣기 후 XX 없음")
	}
}

func hBoardOK(c echo.Context) error {
	return c.HTML(http.StatusOK, "<html><body>ok</body></html>")
}

func overlap78FixtureHTML(day, assignee, uid string) string {
	return `<!DOCTYPE html><html lang="ko"><head><meta charset="utf-8"><title>끼워넣기</title>
<style>.modal-backdrop{position:fixed;inset:0;display:flex;align-items:center;justify-content:center}
.hidden{display:none!important}.z-\[80\]{z-index:80}.modal-panel{background:#fff;padding:1rem;width:min(32rem,100%)}</style>
</head><body>
<form id="nw-form" class="js-keep-modal modal-panel" method="post" action="/workboard/tasks">
<input type="hidden" name="work_type" value="admin">
<input type="hidden" name="redirect" value="/workboard/board?view=day&date=` + day + `">
<input type="hidden" name="assignee" value="` + assignee + `">
<input type="hidden" name="assignee_user_id" value="` + uid + `">
<label>업무명 <input id="title" name="title" value="브XX"></label>
<input type="hidden" name="work_date" value="` + day + `">
<input type="hidden" name="due_date" value="` + day + `">
<input type="hidden" name="start_time" value="10:00">
<input type="hidden" name="end_time" value="14:00">
<button id="nw-save" type="submit">저장</button>
</form>
<div id="overlap-resolve" hidden class="hidden modal-backdrop z-[80]" role="dialog">
  <div class="modal-panel" tabindex="-1">
    <h2>시간이 겹칩니다</h2>
    <p id="overlap-resolve-msg"></p>
    <p id="overlap-resolve-err" hidden></p>
    <ul id="overlap-conflict-list"></ul>
    <input type="number" id="overlap-travel-min" value="0">
    <div id="overlap-split-preview"></div>
    <button type="button" data-overlap-action="split">끼워 넣기</button>
    <button type="button" data-overlap-action="edit">기존 계획 수정</button>
    <button type="button" data-overlap-action="delete">기존 계획 삭제</button>
    <button type="button" data-overlap-action="leave">담당자만 빼고 새 계획 등록</button>
    <button type="button" data-overlap-action="cancel">취소</button>
  </div>
</div>
<script src="/static/js/overlap_resolve.js"></script>
<script>
document.addEventListener('submit', function (e) {
  var form = e.target;
  if (!form.classList.contains('js-keep-modal')) return;
  e.preventDefault();
  var fd = new FormData(form);
  fetch(form.action, {method:'POST', body:fd, credentials:'same-origin',
    headers:{'Accept':'application/json','X-Requested-With':'fetch'}})
    .then(function (r) { return window.handleKeepModalResponse(r, form, fd); });
});
</script>
</body></html>`
}

func Test95_78_CanSplitPlanRefuse(t *testing.T) {
	_, _, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d78r.db"))
	day := time.Now().AddDate(0, 0, 14).Format(dateLayout)
	mustCreateBoardTask(t, repo, "완료된", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set("user_id", other.UserID)
	c.Set("user_name", other.FullName)
	c.Set("role", other.Role)
	c.Set("org_id", other.OrgID)
	c.Set("is_readonly", true)
	t0 := taskByTitle(t, repo, "완료된")
	if canSplitPlan(c, t0, nil) {
		t.Fatal("읽기전용·타조직·완료는 거절해야 함")
	}
}
