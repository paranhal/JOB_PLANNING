package handler

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

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
	if got := pickerDisplayName(model.User{FullName: "USR-1", Username: "USR-x", UserID: "USR-1"}); got != "USR-1" {
		t.Fatalf("login id %s", got)
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
}
