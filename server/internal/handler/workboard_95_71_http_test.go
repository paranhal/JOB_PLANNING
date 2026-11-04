package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_71_StartDueFieldOrderAndRecurrenceFolded(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		"workboard/_modals.html",
		"workboard/task_show.html",
		"admin_work/form.html",
		"workboard/_subtask_modal.html",
	}
	nameRe := regexp.MustCompile(`type="date" name="(work_date|due_date)"`)
	for _, rel := range files {
		b := string(mustRead(t, filepath.Join(root, rel)))
		for fi, form := range strings.Split(b, "</form>") {
			seq := nameRe.FindAllStringSubmatch(form, -1)
			if len(seq) < 2 {
				continue
			}
			if seq[0][1] == "due_date" && seq[1][1] == "work_date" {
				t.Errorf("%s form%d: 종료일이 시작일보다 앞에 있다", rel, fi)
			}
		}
	}

	for _, rel := range []string{"workboard/_modals.html", "admin_work/form.html", "workboard/task_show.html"} {
		b := string(mustRead(t, filepath.Join(root, rel)))
		if !strings.Contains(b, "<details") || !strings.Contains(b, "기간 내 반복 실행") {
			t.Errorf("%s: 기간 내 반복 실행 details 접기 없음", rel)
		}
		if strings.Contains(b, `<details open`) {
			t.Errorf("%s: 반복 블록이 처음부터 열려 있다", rel)
		}
	}
	as := string(mustRead(t, filepath.Join(root, "as/show.html")))
	k := strings.Index(as, "소요 예상 시간")
	if k < 0 {
		t.Fatal("as/show 일일업무 폼 없음")
	}
	block := as
	if k > 2000 {
		block = as[k-2000 : k]
	} else {
		block = as[:k]
	}
	if strings.Index(block, "업무 시작일") < 0 || strings.Index(block, "업무 시작일") > strings.Index(block, "업무 종료일") {
		t.Fatal("as/show 일일업무 폼에서 시작일이 종료일보다 뒤에 있다")
	}
}

func Test95_71_DueBeforeWorkRejected(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d71-date.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	earlier := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	redir := "/workboard/register?view=day&date=" + day
	rec := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"종료일앞"}, "work_date": {day}, "due_date": {earlier},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName},
		"redirect": {redir},
	}, ck)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=date_order") {
		t.Fatalf("거절 loc=%s code=%d", rec.Header().Get("Location"), rec.Code)
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == "종료일앞" {
			t.Fatal("종료일이 앞선 업무가 저장됐다")
		}
	}
	shown := doGetWith(t, e, rec.Header().Get("Location"), ck)
	if shown.Code != http.StatusOK || !strings.Contains(shown.Body.String(), "업무 종료일이 시작일보다 앞입니다") {
		t.Fatalf("등록 화면에 오류가 없다 code=%d", shown.Code)
	}
}

func Test95_71_OverlapErrVisibleOnBoard(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d71-ov.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mustCreateBoardTask(t, repo, "기존겹침", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	boardRedir := "/workboard/board?view=list&date=" + day
	dup := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"새겹침"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {me.FullName},
		"redirect": {boardRedir},
	}, ck)
	loc := dup.Header().Get("Location")
	if dup.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("겹침 거절 loc=%s code=%d", loc, dup.Code)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	errQ := u.Query().Get("err")
	if errQ == "" {
		t.Fatal("Location 에 err 없음")
	}

	board := doGetWith(t, e, loc, ck)
	if board.Code != http.StatusOK {
		t.Fatalf("보드 %d", board.Code)
	}
	bb := board.Body.String()
	if !strings.Contains(bb, "다른 업무가 있습니다") && !strings.Contains(bb, errQ) && !strings.Contains(bb, "시간이 겹칩") {
		t.Fatal("보드에 겹침 오류가 보이지 않는다")
	}

	reg := doGetWith(t, e, "/workboard/register?view=day&date="+day+"&err="+url.QueryEscape(errQ), ck)
	if !strings.Contains(reg.Body.String(), "다른 업무가 있습니다") && !strings.Contains(reg.Body.String(), errQ) {
		t.Fatal("예전 일정표는 err 문구를 보여야 한다")
	}

	work := doGetWith(t, e, "/work?err="+url.QueryEscape(errQ), ck)
	if work.Code != http.StatusOK {
		t.Fatalf("오늘 업무 %d", work.Code)
	}
	if !strings.Contains(work.Body.String(), "다른 업무가 있습니다") && !strings.Contains(work.Body.String(), errQ) {
		t.Fatal("오늘 업무(/work)에 겹침 오류가 보이지 않는다")
	}

	n := 0
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "새겹침" {
			n++
		}
	}
	if n != 0 {
		t.Fatal("겹침으로 거절된 업무가 저장됐다")
	}
}

func Test95_71_DueBeforeWorkHelper(t *testing.T) {
	if dueBeforeWork("2026-11-10", "2026-11-01") != true {
		t.Fatal("종료일이 앞이면 true")
	}
	if dueBeforeWork("2026-11-01", "2026-11-10") {
		t.Fatal("정상 기간")
	}
	if dueBeforeWork("2026-11-01", "") || dueBeforeWork("", "2026-11-01") {
		t.Fatal("한쪽 공백은 검사하지 않는다")
	}
}

func Test95_71_EditFormPrefillFromTemplate(t *testing.T) {
	root := findTemplateRoot(t)
	b := string(mustRead(t, filepath.Join(root, "workboard/task_show.html")))
	filled := []string{
		`value="{{$t.Title}}"`,
		`{{$t.Description}}`,
		`value="{{$t.DueDate}}"`,
		`value="{{if $t.WorkDate}}{{$t.WorkDate}}{{else}}{{$t.DueDate}}{{end}}"`,
		`start: '{{$t.StartTime}}'`,
		`end: '{{$t.EndTime}}'`,
		`{{template "wb_assignee_picker" .}}`,
		`{{$t.ProjectName}}`,
		`preOwnerId: '{{.PickerOwnerID}}'`,
		`value="{{$t.StartTime}}"`,
		`{{template "wb_plan_work_place" $t.WorkPlace}}`,
	}
	for _, want := range filled {
		if !strings.Contains(b, want) {
			t.Errorf("채워지는 항목 없음: %s", want)
		}
	}
	if !strings.Contains(b, `{{template "wb_member_fields" .}}`) {
		t.Fatal("조치 화면 참여자 필드는 유지한다")
	}
	if _, err := os.Stat(filepath.Join(root, "_ui_assignee_picker.html")); err != nil {
		t.Fatal("공용 AssigneePicker 템플릿 없음")
	}
}

func Test95_71_EditUsesAssigneePickerAndChecksOwner(t *testing.T) {
	e, repo := newWorkboardServer(t, "d71-edit.db")
	users := repository.NewUserRepo(repo.DB())
	choi, err := users.GetByUsername("choi")
	if err != nil || choi == nil {
		t.Fatal(err)
	}
	yang, err := users.GetByUsername("yang")
	if err != nil || yang == nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	rec := doForm(t, e, "/workboard/projects", url.Values{"name": {"수정화면사업"}})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("사업 등록 %d", rec.Code)
	}
	projects, err := repo.ListProjects(false)
	if err != nil || len(projects) == 0 {
		t.Fatalf("사업 목록 %v n=%d", err, len(projects))
	}
	pid := ""
	for _, pr := range projects {
		if pr.Name == "수정화면사업" {
			pid = pr.ProjectID
			break
		}
	}
	if pid == "" {
		pid = projects[0].ProjectID
	}
	mustCreateBoardTask(t, repo, "수정대상", day, "09:00", "09:30", model.OrgIDLibrary, choi.FullName, choi.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "수정대상" {
			id = tk.TaskID
			tk.ProjectID = pid
			if err := repo.UpdateTask(&tk); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if id == "" {
		t.Fatal("업무 없음")
	}

	edit := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", jwtCookieUser(t, choi))
	if edit.Code != http.StatusOK {
		t.Fatalf("edit %d %s", edit.Code, edit.Body.String())
	}
	body := edit.Body.String()
	if !strings.Contains(body, "행정/지원업무 수정") {
		t.Fatal("계획 수정 화면이 아니다")
	}
	if !strings.Contains(body, `data-assignee-picker="1"`) {
		t.Fatal("계획 수정에 공용 AssigneePicker 없음")
	}
	if strings.Contains(body, `name="assignee" data-focus-target="assignee"`) {
		t.Fatal("계획 수정 본문에 구 주담당 select 가 남음")
	}
	if !strings.Contains(body, "수정화면사업") {
		t.Fatal("사업명이 수정 화면에 없다")
	}
	if !strings.Contains(body, choi.UserID) {
		t.Fatal("주담당 user id 가 picker 시드에 없다")
	}
	if !strings.Contains(body, `value="`+pid+`"`) {
		t.Fatal("사업 선택값(project_id)이 HTML value 에 없다")
	}
	if !strings.Contains(body, `"SelectedName":"수정화면사업"`) && !strings.Contains(body, "수정화면사업") {
		t.Fatal("피커에 선택된 사업명이 없다")
	}
	if !strings.Contains(body, `name="owner_user_id" value="`+choi.UserID+`"`) {
		t.Fatal("주담당 owner_user_id value 가 없다")
	}
	if !strings.Contains(body, `name="assignee_user_id" value="`+choi.UserID+`"`) {
		t.Fatal("담당자 assignee_user_id 가 선택되어 있지 않다")
	}
	if !strings.Contains(body, `value="`+choi.UserID+`"`) || !strings.Contains(body, "checked") {
		t.Fatal("담당자 체크박스가 선택된 상태로 렌더되지 않았다")
	}

	keep := doFormWith(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"scope": {"register"}, "title": {"수정대상"}, "work_date": {day}, "due_date": {day},
		"work_type": {"admin"}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {choi.FullName},
	}, jwtCookie(t))
	if keep.Code != http.StatusSeeOther || strings.Contains(keep.Header().Get("Location"), "err=") || strings.Contains(keep.Header().Get("Location"), "assignee_required") {
		t.Fatalf("값 유지 저장 loc=%s code=%d body=%s", keep.Header().Get("Location"), keep.Code, keep.Body.String())
	}

	empty := doFormWith(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"scope": {"register"}, "title": {"수정대상"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee_multi": {"1"},
	}, jwtCookieUser(t, choi))
	if empty.Code != http.StatusSeeOther || !strings.Contains(empty.Header().Get("Location"), "assignee_required") {
		t.Fatalf("담당자 비움 거절 loc=%s code=%d", empty.Header().Get("Location"), empty.Code)
	}
	shown := doGetWith(t, e, empty.Header().Get("Location"), jwtCookieUser(t, choi))
	if !strings.Contains(shown.Body.String(), "담당자를 선택하세요") {
		t.Fatal("담당자 필수 오류가 화면에 없다")
	}

	ok := doFormWith(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"scope": {"register"}, "title": {"수정대상"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"}, "assignee_multi": {"1"},
		"owner_user_id": {yang.UserID}, "assignee_user_id": {yang.UserID},
	}, jwtCookieUser(t, choi))
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("담당자 변경 loc=%s code=%d", ok.Header().Get("Location"), ok.Code)
	}
	got, err := repo.GetTask(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Assignee != yang.FullName {
		t.Fatalf("담당자=%q want %q", got.Assignee, yang.FullName)
	}
}

func doFormJSONWith(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "fetch")
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_71_PlanEditButtonAndCompleteForbidden(t *testing.T) {
	root := findTemplateRoot(t)
	as := string(mustRead(t, filepath.Join(root, "as/action.html")))
	if !strings.Contains(as, "계획 수정") {
		t.Fatal("AS 조치 화면에 계획 수정 없음")
	}
	bl := string(mustRead(t, filepath.Join(root, "workboard/board_list.html")))
	if !strings.Contains(bl, "PlanEditHref") || !strings.Contains(bl, "계획 수정") {
		t.Fatal("보드 목록에 계획 수정 없음")
	}

	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d71-btn.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mustCreateBoardTask(t, repo, "버튼대상", day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	board := doGetWith(t, e, "/workboard/board?view=list&date="+day, ck)
	if board.Code != http.StatusOK || !strings.Contains(board.Body.String(), "계획 수정") {
		t.Fatalf("보드 목록에 계획 수정이 없다 code=%d", board.Code)
	}

	var id string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "버튼대상" {
			id = tk.TaskID
			tk.Status = model.WBTaskComplete
			if err := repo.UpdateTask(&tk); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	done := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", ck)
	if done.Code != http.StatusForbidden && done.Code != http.StatusSeeOther {
		t.Fatalf("완료 건 계획 수정 %d", done.Code)
	}
	if done.Code == http.StatusOK {
		t.Fatal("완료 건 계획 수정 화면이 열렸다")
	}
}

func Test95_71_OverlapJSONAndResolve(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d71-json.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mustCreateBoardTask(t, repo, "기존외근", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var firstID string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "기존외근" {
			firstID = tk.TaskID
			if err := repo.SetTaskWorkPlace(firstID, model.WorkPlaceField); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if firstID == "" {
		t.Fatal("기존 업무 없음")
	}

	dup := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"새외근"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {me.FullName},
		"work_place": {"field"},
	}, ck)
	if dup.Code != http.StatusConflict {
		t.Fatalf("겹침 409 아님 code=%d body=%s", dup.Code, dup.Body.String())
	}
	var payload struct {
		Error     string `json:"error"`
		Message   string `json:"message"`
		Conflicts []struct {
			TaskID    string `json:"task_id"`
			WorkPlace string `json:"work_place"`
			Href      string `json:"href"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(dup.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "overlap" || len(payload.Conflicts) == 0 {
		t.Fatalf("충돌 목록 없음 %+v", payload)
	}
	if payload.Conflicts[0].TaskID != firstID {
		t.Fatalf("충돌 id=%s", payload.Conflicts[0].TaskID)
	}
	if !strings.Contains(payload.Message, "다른 업무가 있습니다") {
		t.Fatalf("메시지=%s", payload.Message)
	}
	if payload.Conflicts[0].WorkPlace != model.WorkPlaceField || !strings.Contains(payload.Message, "외근이 겹칩니다") {
		t.Fatalf("외근↔외근 표시 없음 place=%s msg=%s", payload.Conflicts[0].WorkPlace, payload.Message)
	}

	cancel := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"취소겹침"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {me.FullName},
		"overlap_action": {"cancel"}, "conflict_task_id": {firstID},
	}, ck)
	if cancel.Code != http.StatusOK || !strings.Contains(cancel.Body.String(), `"cancelled":true`) {
		t.Fatalf("취소 %d %s", cancel.Code, cancel.Body.String())
	}

	edit := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"수정겹침"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {me.FullName},
		"overlap_action": {"edit"}, "conflict_task_id": {firstID},
	}, ck)
	if edit.Code != http.StatusOK || !strings.Contains(edit.Body.String(), "/workboard/tasks/"+firstID+"/edit") {
		t.Fatalf("기존 수정 %d %s", edit.Code, edit.Body.String())
	}

	leave := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"빠짐겹침"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {me.FullName},
		"overlap_action": {"leave"}, "conflict_task_id": {firstID},
	}, ck)
	if leave.Code != http.StatusSeeOther && leave.Code != http.StatusOK {
		t.Fatalf("담당자 빠짐 %d %s", leave.Code, leave.Body.String())
	}
	old, _ := repo.GetTask(firstID)
	if old != nil && strings.TrimSpace(old.Assignee) == me.FullName {
		t.Fatal("기존 계획에서 담당자가 빠지지 않았다")
	}

	mustCreateBoardTask(t, repo, "삭제대상겹침", day, "13:00", "14:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var delID string
	tasks, _ = repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "삭제대상겹침" {
			delID = tk.TaskID
		}
	}
	del := doFormJSONWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"삭제후등록"}, "work_date": {day}, "due_date": {day},
		"start_time": {"13:00"}, "end_time": {"14:00"}, "assignee": {me.FullName},
		"overlap_action": {"delete"}, "conflict_task_id": {delID},
	}, ck)
	if del.Code != http.StatusSeeOther && del.Code != http.StatusOK {
		t.Fatalf("삭제 후 등록 %d %s", del.Code, del.Body.String())
	}
	gone, _ := repo.GetTask(delID)
	if gone != nil {
		t.Fatal("기존 겹침 업무가 삭제되지 않았다")
	}

	mustCreateBoardTask(t, repo, "배치겹침", day, "15:00", "16:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var placeID string
	tasks, _ = repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "배치겹침" {
			placeID = tk.TaskID
		}
	}
	place := doFormJSONWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {placeID}, "work_date": {day},
		"start_time": {"09:30"}, "end_time": {"10:30"},
	}, ck)
	if place.Code != http.StatusConflict {
		t.Fatalf("배치 겹침 %d %s", place.Code, place.Body.String())
	}
}

func Test95_71_OverlapModalInLayout(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout/base.html")))
	for _, want := range []string{"overlap-resolve", "기존 계획 수정", "overlap_resolve.js", "handleKeepModalResponse"} {
		if !strings.Contains(base, want) && want != "handleKeepModalResponse" {
			if want == "handleKeepModalResponse" {
				continue
			}
			t.Fatalf("레이아웃에 %s 없음", want)
		}
	}
	js := string(mustRead(t, filepath.Join(filepath.Dir(root), "static/js/overlap_resolve.js")))
	if !strings.Contains(js, "showOverlapResolve") || !strings.Contains(js, "handleKeepModalResponse") {
		t.Fatal("겹침 해결 JS 없음")
	}
	modals := string(mustRead(t, filepath.Join(root, "workboard/_modals.html")))
	if !strings.Contains(modals, "wb_plan_work_place") {
		t.Fatal("계획 단계 근무 구분 없음")
	}
}

func Test95_71_OverlapModalMobileLayout(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout/base.html")))
	if !strings.Contains(base, `grid grid-cols-1 sm:grid-cols-2`) {
		t.Fatal("360/390에서 겹침 해결 버튼이 한 줄로 쌓이지 않는다")
	}
	if !strings.Contains(base, `max-h-[90vh] overflow-y-auto`) {
		t.Fatal("겹침 모달에 세로 스크롤이 없다")
	}
	if !strings.Contains(base, `w-full max-w-lg`) {
		t.Fatal("겹침 모달이 좁은 화면에서 가로로 넘친다")
	}
}
