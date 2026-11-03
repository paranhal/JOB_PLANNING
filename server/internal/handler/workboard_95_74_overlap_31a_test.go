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

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func jwtCookieVerified(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "org_id": u.OrgID, "is_readonly": u.IsReadOnly,
		"profile_ok": true, "verified_at": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func taskByTitle(t *testing.T, repo interface {
	ListTasks() ([]model.WorkTask, error)
}, title string) *model.WorkTask {
	t.Helper()
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for i := range tasks {
		if tasks[i].Title == title {
			return &tasks[i]
		}
	}
	return nil
}

func overlapCreateJSON(t *testing.T, e *echo.Echo, ck *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return doFormJSONWith(t, e, "/workboard/tasks", form, ck)
}

func Test95_74_Overlap31A_FourChoices(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-31a.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 6).Format(dateLayout)
	redir := "/workboard/board?view=day&date=" + day

	mustCreateBoardTask(t, repo, "기존-취소", day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oldCancel := taskByTitle(t, repo, "기존-취소")
	cancel := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새-취소"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
		"overlap_action": {"cancel"}, "conflict_task_id": {oldCancel.TaskID}, "redirect": {redir},
	})
	if cancel.Code != http.StatusOK || !strings.Contains(cancel.Body.String(), `"cancelled":true`) {
		t.Fatalf("취소 %d %s", cancel.Code, cancel.Body.String())
	}
	if taskByTitle(t, repo, "새-취소") != nil {
		t.Fatal("취소인데 새 업무가 저장됐다")
	}
	got := taskByTitle(t, repo, "기존-취소")
	if got == nil || got.Assignee != me.FullName {
		t.Fatal("취소 후 기존 업무가 바뀌었다")
	}

	mustCreateBoardTask(t, repo, "기존-수정", day, "11:00", "11:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oldEdit := taskByTitle(t, repo, "기존-수정")
	edit := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새-수정"}, "work_date": {day}, "due_date": {day},
		"start_time": {"11:00"}, "end_time": {"11:30"}, "assignee": {me.FullName},
		"overlap_action": {"edit"}, "conflict_task_id": {oldEdit.TaskID}, "redirect": {redir},
	})
	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), "/workboard/tasks/"+oldEdit.TaskID+"/edit") {
		t.Fatalf("수정 %d %s", edit.Code, edit.Body.String())
	}
	if !strings.Contains(edit.Body.String(), "notice=overlap_edit") {
		t.Fatalf("수정 안내 없음 %s", edit.Body.String())
	}
	if taskByTitle(t, repo, "새-수정") != nil {
		t.Fatal("기존 계획 수정인데 새 업무가 저장됐다")
	}
	notice := doGetWith(t, e, "/workboard/tasks/"+oldEdit.TaskID+"/edit?notice=overlap_edit", ck)
	if notice.Code != http.StatusOK || !strings.Contains(notice.Body.String(), "새 업무는 저장하지 않았습니다") {
		t.Fatalf("수정 화면 안내 없음 code=%d", notice.Code)
	}

	mustCreateBoardTask(t, repo, "기존-삭제", day, "13:00", "13:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oldDel := taskByTitle(t, repo, "기존-삭제")
	del := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새-삭제"}, "work_date": {day}, "due_date": {day},
		"start_time": {"13:00"}, "end_time": {"13:30"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {oldDel.TaskID}, "redirect": {redir},
	})
	if del.Code != http.StatusSeeOther && del.Code != http.StatusOK {
		t.Fatalf("삭제 %d %s", del.Code, del.Body.String())
	}
	if loc := del.Header().Get("Location"); loc != "" && !strings.Contains(loc, "/workboard/board") {
		t.Fatalf("삭제 후 이동 %s", loc)
	}
	if taskByTitle(t, repo, "기존-삭제") != nil {
		t.Fatal("기존 업무가 삭제되지 않았다")
	}
	if taskByTitle(t, repo, "새-삭제") == nil {
		t.Fatal("삭제 후 새 업무가 없다")
	}
	var delLogs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='work_tasks' AND entity_id=? AND action='delete'`, oldDel.TaskID).Scan(&delLogs); err != nil || delLogs < 1 {
		t.Fatalf("삭제 이력 n=%d err=%v", delLogs, err)
	}

	mustCreateBoardTask(t, repo, "기존-제외", day, "15:00", "15:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	oldLeave := taskByTitle(t, repo, "기존-제외")
	leave := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"새-제외"}, "work_date": {day}, "due_date": {day},
		"start_time": {"15:00"}, "end_time": {"15:30"}, "assignee": {me.FullName},
		"overlap_action": {"leave"}, "conflict_task_id": {oldLeave.TaskID}, "redirect": {redir},
	})
	if leave.Code != http.StatusSeeOther && leave.Code != http.StatusOK {
		t.Fatalf("제외 %d %s", leave.Code, leave.Body.String())
	}
	if loc := leave.Header().Get("Location"); loc != "" && !strings.Contains(loc, "/workboard/board") {
		t.Fatalf("제외 후 이동 %s", loc)
	}
	left := taskByTitle(t, repo, "기존-제외")
	if left == nil {
		t.Fatal("제외인데 기존 업무가 사라졌다")
	}
	if strings.TrimSpace(left.Assignee) == me.FullName {
		t.Fatal("기존 업무에서 담당자가 빠지지 않았다")
	}
	if taskByTitle(t, repo, "새-제외") == nil {
		t.Fatal("제외 후 새 업무가 없다")
	}
	var leaveLogs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='work_tasks' AND entity_id=? AND action='update'`, oldLeave.TaskID).Scan(&leaveLogs); err != nil || leaveLogs < 1 {
		t.Fatalf("제외 이력 n=%d err=%v", leaveLogs, err)
	}
}

func Test95_74_Overlap31A_BatchRecheckGuardsSitesRecurrence(t *testing.T) {
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-31b.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 7).Format(dateLayout)
	endDay := time.Now().AddDate(0, 0, 10).Format(dateLayout)

	mustCreateBoardTask(t, repo, "겹침A", day, "09:00", "09:40", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "겹침B", day, "09:20", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	a := taskByTitle(t, repo, "겹침A")
	b := taskByTitle(t, repo, "겹침B")
	batch := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"일괄제외"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:10"}, "end_time": {"09:50"}, "assignee": {me.FullName},
		"overlap_action": {"leave"}, "conflict_task_id": {a.TaskID, b.TaskID},
	})
	if batch.Code != http.StatusSeeOther && batch.Code != http.StatusOK {
		t.Fatalf("일괄 %d %s", batch.Code, batch.Body.String())
	}
	if strings.TrimSpace(taskByTitle(t, repo, "겹침A").Assignee) == me.FullName || strings.TrimSpace(taskByTitle(t, repo, "겹침B").Assignee) == me.FullName {
		t.Fatal("여러 건 제외가 일부만 적용됐다")
	}
	if taskByTitle(t, repo, "일괄제외") == nil {
		t.Fatal("일괄 제외 후 새 업무 없음")
	}

	mustCreateBoardTask(t, repo, "재검사용", day, "16:00", "16:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	stale := taskByTitle(t, repo, "재검사용")
	re := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"재검사새"}, "work_date": {day}, "due_date": {day},
		"start_time": {"16:00"}, "end_time": {"16:30"}, "assignee": {me.FullName},
		"overlap_action": {"leave"}, "conflict_task_id": {stale.TaskID, "WT-GONE"},
	})
	if re.Code != http.StatusConflict {
		t.Fatalf("재검사 409 아님 %d %s", re.Code, re.Body.String())
	}
	if strings.TrimSpace(taskByTitle(t, repo, "재검사용").Assignee) != me.FullName {
		t.Fatal("재검사 실패인데 기존이 바뀌었다")
	}
	if taskByTitle(t, repo, "재검사새") != nil {
		t.Fatal("재검사 실패인데 새 업무가 저장됐다")
	}

	mustCreateBoardTask(t, repo, "완료겹침", day, "17:00", "17:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	done := taskByTitle(t, repo, "완료겹침")
	doneDel := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"완료삭제새"}, "work_date": {day}, "due_date": {day},
		"start_time": {"17:00"}, "end_time": {"17:30"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {done.TaskID},
	})
	if doneDel.Code != http.StatusBadRequest || !strings.Contains(doneDel.Body.String(), "완료된 업무는 삭제할 수 없습니다") {
		t.Fatalf("완료 삭제 %d %s", doneDel.Code, doneDel.Body.String())
	}

	mustCreateBoardTask(t, repo, "원본겹침", day, "18:00", "18:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	src := taskByTitle(t, repo, "원본겹침")
	if _, err := db.Exec(`UPDATE work_tasks SET source_type=?, source_id=? WHERE task_id=?`, model.WBSourceAS, "AS-LINK", src.TaskID); err != nil {
		t.Fatal(err)
	}
	srcDel := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"원본삭제새"}, "work_date": {day}, "due_date": {day},
		"start_time": {"18:00"}, "end_time": {"18:30"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {src.TaskID},
	})
	if srcDel.Code != http.StatusBadRequest || !strings.Contains(srcDel.Body.String(), "원본에 연결된 업무는 삭제할 수 없습니다") {
		t.Fatalf("원본 삭제 %d %s", srcDel.Code, srcDel.Body.String())
	}

	mustCreateBoardTask(t, repo, "타조직겹침", day, "19:00", "19:30", other.OrgID, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	otherT := taskByTitle(t, repo, "타조직겹침")
	orgDel := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"타조직새"}, "work_date": {day}, "due_date": {day},
		"start_time": {"19:00"}, "end_time": {"19:30"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {otherT.TaskID},
	})
	if orgDel.Code != http.StatusBadRequest || !strings.Contains(orgDel.Body.String(), "다른 조직") {
		t.Fatalf("조직 격리 %d %s", orgDel.Code, orgDel.Body.String())
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c74-a','사이트A','사이트A',1,?), ('c74-b','사이트B','사이트B',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "사이트겹침", day, "08:00", "08:20", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	siteOld := taskByTitle(t, repo, "사이트겹침")
	sites := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"사이트새"}, "work_date": {day}, "due_date": {day},
		"start_time": {"08:00"}, "end_time": {"08:20"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {siteOld.TaskID},
		"site_id": {"c74-a", "c74-b"},
	})
	if sites.Code != http.StatusSeeOther && sites.Code != http.StatusOK {
		t.Fatalf("사이트 %d %s", sites.Code, sites.Body.String())
	}
	nw := taskByTitle(t, repo, "사이트새")
	if nw == nil {
		t.Fatal("사이트 다건 새 업무 없음")
	}
	listed, err := repo.ListTaskSites(nw.TaskID)
	if err != nil || len(listed) < 2 {
		t.Fatalf("사이트 n=%d err=%v", len(listed), err)
	}

	mustCreateBoardTask(t, repo, "반복겹침", day, "07:00", "07:20", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	recOld := taskByTitle(t, repo, "반복겹침")
	rec := overlapCreateJSON(t, e, ck, url.Values{
		"work_type": {"admin"}, "title": {"반복새"}, "work_date": {day}, "due_date": {endDay},
		"start_time": {"07:00"}, "end_time": {"07:20"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {recOld.TaskID},
		"recurrence_enabled": {"1"}, "start_date": {day}, "end_date": {endDay},
		"rule_type": {"daily"}, "holiday_policy": {"as_is"}, "complete_policy": {"manual"},
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("반복 %d %s", rec.Code, rec.Body.String())
	}
	parent := taskByTitle(t, repo, "반복새")
	if parent == nil {
		t.Fatal("반복 새 업무 없음")
	}
	n, _ := repo.CountOccurrences(parent.TaskID)
	if rec.Header().Get("Location") != "" && strings.Contains(rec.Header().Get("Location"), "ok=rec_gen") && n < 1 {
		t.Fatal("반복 실행 작업이 없다")
	}
}

func Test95_74_Overlap31A_JSBusyAndModalError(t *testing.T) {
	js := string(mustRead(t, filepath.Join(findTemplateRoot(t), "..", "static", "js", "overlap_resolve.js")))
	for _, want := range []string{"처리 중…", "overlap-resolve-err", "showModalError", "setBusy", "hideOverlay(false)", "conflict_task_id"} {
		if !strings.Contains(js, want) {
			t.Fatalf("JS에 %s 없음", want)
		}
	}
	if strings.Contains(js, "location.assign(hit.href)") {
		t.Fatal("수정이 저장 POST 없이 이동한다")
	}
	base := string(mustRead(t, filepath.Join(findTemplateRoot(t), "layout", "base.html")))
	if !strings.Contains(base, `id="overlap-resolve-err"`) || !strings.Contains(base, `id="overlap-resolve-busy"`) {
		t.Fatal("선택 화면 오류·처리 중 표시 없음")
	}
}

func Test95_74_Overlap31A_BrowserFocusAndChoices(t *testing.T) {
	skipIfNoPlaywrightBrowser(t)
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d74-31c.db"))
	if _, err := db.Exec(`UPDATE users SET mobile='010-1111-2222', profile_done=1 WHERE user_id=?`, me.UserID); err != nil {
		t.Fatal(err)
	}
	me.Mobile = "010-1111-2222"
	me.ProfileDone = true
	ck := jwtCookieVerified(t, me)
	day := time.Now().AddDate(0, 0, 8).Format(dateLayout)
	mustCreateBoardTask(t, repo, "브-취소", day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "브-수정", day, "11:00", "11:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "브-삭제", day, "13:00", "13:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "브-제외", day, "15:00", "15:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	e.GET("/__t74/form", func(c echo.Context) error {
		return c.HTML(http.StatusOK, overlap31AFixtureHTML(day, me.FullName, me.UserID))
	})

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cmd := exec.Command("node", "95_74_overlap_31a_browser.mjs")
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

	if taskByTitle(t, repo, "브새-취소") != nil {
		t.Fatal("브라우저 취소 후 새 업무가 있다")
	}
	if taskByTitle(t, repo, "브새-수정") != nil {
		t.Fatal("브라우저 수정 후 새 업무가 있다")
	}
	if taskByTitle(t, repo, "브-삭제") != nil {
		t.Fatal("브라우저 삭제 후 기존이 남았다")
	}
	if taskByTitle(t, repo, "브새-삭제") == nil {
		t.Fatal("브라우저 삭제 후 새 업무 없음")
	}
	left := taskByTitle(t, repo, "브-제외")
	if left == nil || strings.TrimSpace(left.Assignee) == me.FullName {
		t.Fatal("브라우저 제외 후 담당자가 그대로다")
	}
	if taskByTitle(t, repo, "브새-제외") == nil {
		t.Fatal("브라우저 제외 후 새 업무 없음")
	}
}

func overlap31AFixtureHTML(day, assignee, uid string) string {
	return `<!DOCTYPE html><html lang="ko"><head><meta charset="utf-8">
<title>겹침 선택</title>
<style>
.modal-backdrop{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;padding:1rem;background:rgba(0,0,0,.45)}
.hidden{display:none!important}
.z-50{z-index:50}.z-\[80\]{z-index:80}
.modal-panel{background:#fff;padding:1.2rem;width:min(32rem,100%)}
.btn{padding:.5rem .75rem}
</style></head><body>
<div id="new-work" class="modal-backdrop z-50">
<form id="nw-form" class="js-keep-modal modal-panel" method="post" action="/workboard/tasks">
<input type="hidden" name="work_type" value="admin">
<input type="hidden" name="redirect" value="/workboard/board?view=day&date=` + day + `">
<input type="hidden" name="assignee" value="` + assignee + `">
<input type="hidden" name="assignee_user_id" value="` + uid + `">
<label>업무명 <input id="title" name="title" value="KEEP"></label>
<input id="work_date" type="hidden" name="work_date" value="` + day + `">
<input id="due_date" type="hidden" name="due_date" value="` + day + `">
<input id="start" type="hidden" name="start_time" value="09:00">
<input id="end" type="hidden" name="end_time" value="09:30">
<button id="nw-save" type="submit">저장</button>
</form>
</div>
<div id="overlap-resolve" hidden class="hidden modal-backdrop z-[80] flex items-center justify-center p-4" role="dialog" aria-modal="true">
  <div class="modal-panel" tabindex="-1">
    <h2 id="overlap-resolve-title">시간이 겹칩니다</h2>
    <p id="overlap-resolve-msg"></p>
    <p id="overlap-resolve-err" hidden role="alert"></p>
    <p id="overlap-resolve-busy" hidden>처리 중…</p>
    <ul id="overlap-conflict-list"></ul>
    <div class="grid">
      <button type="button" data-overlap-action="edit">기존 계획 수정</button>
      <button type="button" data-overlap-action="delete">기존 계획 삭제</button>
      <button type="button" data-overlap-action="leave">담당자만 빼고 새 계획 등록</button>
      <button type="button" data-overlap-action="cancel">취소</button>
    </div>
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
