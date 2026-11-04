package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_96_R2_RenderPostSaveSuccessAndFail(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	page := getASActionPage(t, e, asID)
	if strings.Contains(page, `data-as-draft-notice="1"`) && strings.Contains(page, "border-amber-300") {
		t.Fatal("임시 저장 안내가 오류(amber) 스타일이다")
	}
	if !strings.Contains(page, `"양기헌"`) && !strings.Contains(page, "양기헌") {
		t.Fatal("처리담당자 이름이 렌더 HTML에 없다")
	}
	if strings.Contains(page, `js-picker-names">{"`) && strings.Contains(page, `"담당자"`) {
		t.Fatal("picker 이름표에 리터럴 담당자가 들어 있다")
	}

	failForm := parseRenderedActionForm(page)
	fillRenderedSetCause(t, failForm, "server", "server.hw", "")
	failForm.Set("s1_result_code", "")
	failForm.Set("result_code", "")
	failRec := postASActionRaw(t, e, asID, failForm)
	failLoc := failRec.Header().Get("Location")
	if failRec.Code != http.StatusSeeOther || !strings.Contains(failLoc, "err=set_draft_1") {
		t.Fatalf("실패 경로 loc=%s", failLoc)
	}
	failPage := getASActionFollow(t, e, failLoc)
	if !strings.Contains(failPage, "id=\"as-action-err-banner\"") {
		t.Fatal("실패 시 상단 오류 배너가 없다")
	}
	if !strings.Contains(failPage, "결과가 선택되지 않았습니다") {
		t.Fatalf("실패 사유가 화면에 없다")
	}
	if !strings.Contains(failPage, "전원") && !strings.Contains(failPage, "RFID") && !strings.Contains(failPage, "재기동") {
		t.Fatal("실패 후 조치내용이 사라졌다")
	}
	if strings.Contains(failPage, "border-amber-300 bg-amber-50") && strings.Contains(failPage, "임시 저장된 조치") {
		t.Fatal("상태 안내가 오류와 같은 amber 스타일이다")
	}

	okForm := parseRenderedActionForm(getASActionPage(t, e, asID))
	fillRenderedSetCause(t, okForm, "server", "server.hw", "")
	okRec := postASActionRaw(t, e, asID, okForm)
	okLoc := okRec.Header().Get("Location")
	if okRec.Code != http.StatusSeeOther || strings.Contains(okLoc, "err=") {
		t.Fatalf("성공 경로 loc=%s", okLoc)
	}
	got, err := asRepo.GetByID(repository.OrgAll, asID)
	if err != nil || got == nil || got.ResultCode != model.ResultDone {
		t.Fatalf("저장 결과 %+v err=%v", got, err)
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) == 0 || sets[0].ResultCode != model.ResultDone {
		t.Fatalf("세트 미저장 %+v", sets)
	}
}

func Test95_96_R2_RenderPostKeepsReceiptAssigneeWithoutAlpine(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, `ownerId:`) || !strings.Contains(page, "양기헌") {
		t.Fatal("picker x-data 에 접수 담당자 hydrate 값이 없다")
	}
	form := parseRenderedActionForm(page)
	if form.Get("assignee_multi") != "1" {
		t.Fatal("렌더 폼에 assignee_multi 가 없다")
	}
	if _, ok := form["assignee_user_id"]; ok && strings.TrimSpace(form.Get("assignee_user_id")) != "" {
		t.Fatal("Alpine 없이 assignee_user_id 가 채워져 있으면 폴백 전제가 아니다")
	}
	form.Del("assignee_user_id")
	fillRenderedSetCause(t, form, "server", "server.hw", "")
	form.Del("assignee_external")
	form.Del("assignee_user_id")
	rec := postASActionRaw(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=assignee_required") {
		t.Fatalf("접수 담당자가 있는데 Alpine 빈 picker 로 막혔다 loc=%s", loc)
	}
	if strings.Contains(loc, "err=") {
		t.Fatalf("저장 실패 loc=%s", loc)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.ResultCode != model.ResultDone {
		t.Fatalf("결과 %s", got.ResultCode)
	}
	if got.AssignedTo != "양기헌" {
		t.Fatalf("접수 담당자가 바뀌었다 %q", got.AssignedTo)
	}
}

func Test95_96_R2_MissingAssigneeUserIDNoReceiptRejected(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	as, err := asRepo.GetByID(repository.OrgAll, asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.AssignedTo, as.AssignedUserID = "", ""
	if err := asRepo.Update(as); err != nil {
		t.Fatal(err)
	}
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	form := parseRenderedActionForm(getASActionPage(t, e, asID))
	form.Del("assignee_user_id")
	fillRenderedSetCause(t, form, "server", "server.hw", "")
	form.Del("assignee_external")
	form.Del("assignee_user_id")
	rec := postASActionRaw(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=assignee_required") {
		t.Fatalf("빈 접수 담당자를 통과했다 loc=%s", loc)
	}
	failPage := getASActionFollow(t, e, loc)
	if !strings.Contains(failPage, "처리담당자를 선택하세요") || !strings.Contains(failPage, `data-assignee-error`) {
		t.Fatal("담당자 칸에 사유가 없다")
	}
	if !strings.Contains(failPage, `id="as-action-err-banner"`) {
		t.Fatal("상단 오류 배너가 없다")
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if admin != nil && got.AssignedUserID == admin.UserID {
		t.Fatal("조직관리자 본인 ID 로 채워졌다")
	}
	if strings.TrimSpace(got.AssignedTo) != "" || strings.TrimSpace(got.AssignedUserID) != "" {
		t.Fatalf("빈 접수가 채워졌다 to=%q id=%q", got.AssignedTo, got.AssignedUserID)
	}
}

func Test95_96_R2_FormAssigneeOverridesReceipt(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	u := &model.User{
		Username: "park-sel", PasswordHash: HashPassword("pw"), FullName: "박선택",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	if err := h.AS.userRepo.Create(u); err != nil {
		t.Fatal(err)
	}
	gotU, err := h.AS.userRepo.GetByUsername("park-sel")
	if err != nil || gotU == nil {
		t.Fatal(err)
	}
	form := parseRenderedActionForm(getASActionPage(t, e, asID))
	fillRenderedSetCause(t, form, "server", "server.hw", "")
	form.Del("assignee_external")
	form.Set("assignee_user_id", gotU.UserID)
	rec := postASActionRaw(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("폼 담당자 저장 실패 loc=%s", loc)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.AssignedUserID != gotU.UserID || got.AssignedTo == "양기헌" {
		t.Fatalf("폼 값이 우선이어야 한다 to=%q id=%q want=%q", got.AssignedTo, got.AssignedUserID, gotU.UserID)
	}
}

func Test95_96_R2_BottomResultFillsEmptySet(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := parseRenderedActionForm(getASActionPage(t, e, asID))
	fillRenderedSetCause(t, form, "server", "server.hw", "")
	form.Set("s1_result_code", "")
	form.Set("result_code", model.ResultDone)
	rec := postASActionRaw(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=set_draft") {
		t.Fatalf("최종 저장 결과를 세트로 옮기지 못했다 loc=%s", loc)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.ResultCode != model.ResultDone {
		t.Fatalf("결과 %s loc=%s", got.ResultCode, loc)
	}
}

func getASActionFollow(t *testing.T, e *echo.Echo, loc string) string {
	t.Helper()
	path := loc
	if i := strings.Index(loc, "://"); i >= 0 {
		if j := strings.Index(loc[i+3:], "/"); j >= 0 {
			path = loc[i+3+j:]
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("follow GET status=%d loc=%s", rec.Code, path)
	}
	return rec.Body.String()
}
