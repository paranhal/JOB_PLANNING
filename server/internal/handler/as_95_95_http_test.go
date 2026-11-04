package handler

import (
	"encoding/json"
	"html"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_95_T3_PickerDisplayName(t *testing.T) {
	if got := pickerDisplayName(model.User{FullName: "양현모", Username: "hmyang", UserID: "USR-1"}); got != "양현모" {
		t.Fatalf("full_name %s", got)
	}
	if got := pickerDisplayName(model.User{FullName: "", Username: "hmyang", UserID: "USR-1"}); got != "hmyang" {
		t.Fatalf("username %s", got)
	}
	if got := pickerDisplayName(model.User{FullName: "USR-1", Username: "USR-x", UserID: "USR-1"}); got != "이름 미등록" {
		t.Fatalf("both USR %s", got)
	}
	if got := pickerDisplayName(model.User{}); got != "이름 미등록" {
		t.Fatalf("empty %s", got)
	}
	if pickerDisplayName(model.User{FullName: "담당자", Username: "USR-x", UserID: "USR-9"}) == "담당자" {
		t.Fatal("담당자 단독 반환")
	}
}

func Test95_95_T4_ActionPickerNamesOutsideAssignable(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	out := &model.User{
		Username: "clevel95", PasswordHash: HashPassword("password10"), FullName: "박선택",
		Role: model.RoleCLevel, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := h.AS.userRepo.Create(out); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET assigned_user_id=?, assigned_to=? WHERE as_id=?`, out.UserID, out.FullName, asID); err != nil {
		t.Fatal(err)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got == nil || got.AssignedUserID != out.UserID {
		t.Fatalf("배정 실패 %+v", got)
	}
	assignable, err := h.AS.userRepo.ListAssignable(model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range assignable {
		if u.UserID == out.UserID {
			t.Fatal("C-Level 이 ListAssignable 에 있으면 T4가 아니다")
		}
	}
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, "js-picker-names") {
		t.Fatal("PickerNames JSON 없음")
	}
	if !strings.Contains(body, "박선택") {
		t.Fatal("Assignees 밖 선택 담당자 이름이 렌더에 없다")
	}
	start := strings.Index(body, `class="js-picker-names"`)
	if start < 0 {
		t.Fatal("js-picker-names 없음")
	}
	chunk := body[start:]
	gt := strings.Index(chunk, ">")
	end := strings.Index(chunk, "</template>")
	if gt < 0 || end < 0 {
		t.Fatal("json 구간 없음")
	}
	raw := html.UnescapeString(strings.TrimSpace(chunk[gt+1 : end]))
	names := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		t.Fatalf("json %v %s", err, raw)
	}
	if names[out.UserID] != "박선택" {
		t.Fatalf("PickerNames %v", names)
	}
	if !strings.Contains(body, `x-text="summaryLabel()">박선택<`) {
		t.Fatal("처리담당자 버튼 SSR 에 박선택이 없다")
	}
}

func assertPickerButtonShows(t *testing.T, body, name string) {
	t.Helper()
	if !strings.Contains(body, "js-picker-names") {
		t.Fatal("PickerNames JSON 없음")
	}
	if !strings.Contains(body, name) {
		t.Fatalf("렌더에 이름 %s 없음", name)
	}
	if !strings.Contains(body, `x-text="summaryLabel()">`+name+`<`) {
		t.Fatalf("선택기 버튼 SSR 에 %s 없음", name)
	}
	if strings.Contains(body, `x-text="summaryLabel()">담당자<`) {
		t.Fatal("선택기 버튼이 이름 대신 담당자이다")
	}
}

func Test95_95_T3_PickerButtonLabel(t *testing.T) {
	names := map[string]string{"U1": "김현우"}
	if got := pickerButtonLabel(names, "U1", []string{"U1"}, ""); got != "김현우" {
		t.Fatalf("id %s", got)
	}
	if got := pickerButtonLabel(nil, "", nil, "양기헌"); got != "양기헌" {
		t.Fatalf("external %s", got)
	}
	if got := pickerButtonLabel(map[string]string{"U1": "담당자"}, "U1", nil, "담당자"); got != "— 선택 —" {
		t.Fatalf("담당자 차단 %s", got)
	}
}

func Test95_95_WorkNewAndEditPickerName(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d95-95-wb.db"))
	ck := jwtCookieUser(t, me)
	nw := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date="+time.Now().Format(dateLayout), ck)
	if nw.Code != http.StatusOK {
		t.Fatalf("등록 %d", nw.Code)
	}
	assertPickerButtonShows(t, nw.Body.String(), me.FullName)

	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "95-95수정", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == "95-95수정" {
			id = tk.TaskID
			break
		}
	}
	if id == "" {
		t.Fatal("업무 없음")
	}
	ed := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", ck)
	if ed.Code != http.StatusOK {
		t.Fatalf("수정 %d %s", ed.Code, ed.Body.String())
	}
	assertPickerButtonShows(t, ed.Body.String(), me.FullName)
}

func Test95_106_ActionShowsNameNotUserID(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	u := &model.User{
		Username: "realname106", PasswordHash: HashPassword("password10"), FullName: "김실명",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := h.AS.userRepo.Create(u); err != nil {
		t.Fatal(err)
	}
	gotU, err := h.AS.userRepo.GetByUsername("realname106")
	if err != nil || gotU == nil {
		t.Fatal(err)
	}
	if _, err := h.AS.db.Exec(`UPDATE as_receipts SET assigned_user_id=?, assigned_to=? WHERE as_id=?`, gotU.UserID, gotU.UserID, asID); err != nil {
		t.Fatal(err)
	}
	body := getASActionPage(t, e, asID)
	assertPickerButtonShows(t, body, "김실명")
	if strings.Contains(body, `x-text="summaryLabel()">`+gotU.UserID+`<`) {
		t.Fatal("처리담당자 버튼이 아이디다")
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got == nil || got.AssignedTo != gotU.UserID {
		t.Fatalf("저장 전 원본이 아이디여야 한다 %+v", got)
	}
	form := parseRenderedActionForm(body)
	fillRenderedSetCause(t, form, "server", "server.hw", "")
	form.Del("assignee_external")
	form.Del("assignee_user_id")
	form.Set("assignee_multi", "1")
	form.Set("assignee_user_id", gotU.UserID)
	rec := postASActionRaw(t, e, asID, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ = asRepo.GetByID(repository.OrgAll, asID)
	if got == nil || got.AssignedTo != "김실명" {
		t.Fatalf("저장 후 이름 아님 %+v", got)
	}
}
