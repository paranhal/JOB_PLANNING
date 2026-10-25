package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func pickerCtx(form url.Values, org, userID, role string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", org)
	c.Set("user_id", userID)
	c.Set("role", role)
	c.Set("user_name", "t")
	return c
}

func TestParseSelectionGroupLeaderOwnerAndExclude(t *testing.T) {
	_, fx := newMnt9514AServer(t)
	form := url.Values{}
	form.Set("assignee_multi", "1")
	form.Add("assignee_group_id", fx.gLib)
	form.Add("assignee_exclude_user_id", fx.b.UserID)
	c := pickerCtx(form, model.OrgIDLibrary, fx.oa.UserID, model.RoleOrgAdmin)
	deps := PickerDeps{Users: fx.users, Groups: repository.NewWorkGroupRepo(fx.repo.DB())}
	sel, err := ParseSelection(c, deps, PickerSpec{AllowGroups: true, AllowExternal: true})
	if err != nil {
		t.Fatal(err)
	}
	if sel.PrimaryUserID != fx.a.UserID {
		t.Fatalf("그룹장 주담당 want %s got %s", fx.a.UserID, sel.PrimaryUserID)
	}
	for _, id := range sel.UserIDs {
		if id == fx.b.UserID {
			t.Fatal("제외한 구성원이 포함됨")
		}
	}
	if sel.Empty() {
		t.Fatal("empty")
	}
}

func TestParseSelectionRequiredAndForeignOrg(t *testing.T) {
	_, fx := newMnt9514AServer(t)
	form := url.Values{}
	form.Set("assignee_multi", "1")
	c := pickerCtx(form, model.OrgIDLibrary, fx.oa.UserID, model.RoleOrgAdmin)
	deps := PickerDeps{Users: fx.users, Groups: repository.NewWorkGroupRepo(fx.repo.DB())}
	_, err := ParseSelection(c, deps, PickerSpec{Required: true, AllowGroups: true})
	if err == nil {
		t.Fatal("필수 누락 허용")
	}
	form2 := url.Values{}
	form2.Set("assignee_multi", "1")
	form2.Add("assignee_user_id", fx.siU.UserID)
	c2 := pickerCtx(form2, model.OrgIDLibrary, fx.oa.UserID, model.RoleOrgAdmin)
	_, err = ParseSelection(c2, deps, PickerSpec{AllowGroups: true})
	if err == nil {
		t.Fatal("타 조직 사용자 허용")
	}
}

func TestParseSelectionLegacyAssigneeName(t *testing.T) {
	_, fx := newMnt9514AServer(t)
	form := url.Values{}
	form.Set("assignee", fx.a.FullName)
	c := pickerCtx(form, model.OrgIDLibrary, fx.oa.UserID, model.RoleOrgAdmin)
	deps := PickerDeps{Users: fx.users, Groups: repository.NewWorkGroupRepo(fx.repo.DB())}
	sel, err := ParseSelection(c, deps, PickerSpec{AllowGroups: true, AllowExternal: true})
	if err != nil {
		t.Fatal(err)
	}
	if sel.PrimaryName != fx.a.FullName {
		t.Fatalf("구 폼 이름 호환 got %q", sel.PrimaryName)
	}
}

func TestParseSelectionSingleMode(t *testing.T) {
	_, fx := newMnt9514AServer(t)
	form := url.Values{}
	form.Set("assignee_multi", "1")
	form.Add("assignee_user_id", fx.a.UserID)
	form.Add("assignee_user_id", fx.c.UserID)
	form.Set("owner_user_id", fx.a.UserID)
	c := pickerCtx(form, model.OrgIDLibrary, fx.oa.UserID, model.RoleOrgAdmin)
	deps := PickerDeps{Users: fx.users, Groups: repository.NewWorkGroupRepo(fx.repo.DB())}
	sel, err := ParseSelection(c, deps, PickerSpec{Mode: PickerSingle, AllowGroups: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.People) != 1 || sel.PrimaryUserID != fx.a.UserID {
		t.Fatalf("single %+v", sel)
	}
}

func TestParseMultiSiteModeIgnoresWhenFewSites(t *testing.T) {
	e := echo.New()
	form := url.Values{}
	form.Set("multi_site_mode", "joint")
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	c := e.NewContext(req, httptest.NewRecorder())
	if got := parseMultiSiteMode(c, 1); got != "separate" {
		t.Fatalf("got %s", got)
	}
	if got := parseMultiSiteMode(c, 2); got != "joint" {
		t.Fatalf("got %s", got)
	}
}

func TestWorkboardModalsUseAssigneePicker(t *testing.T) {
	e, _ := newWorkboardServer(t, "picker-modal.db")
	rec := doGet(t, e, "/workboard/register")
	body := rec.Body.String()
	if !strings.Contains(body, `data-assignee-picker="1"`) {
		t.Fatal("register 모달에 picker 없음")
	}
	if strings.Contains(body, `<select name="assignee"`) && strings.Contains(body, "주담당") {
		t.Fatal("구 주담당 select 가 남음")
	}
	if !strings.Contains(body, "site_multi_picker") && !strings.Contains(body, "js-site-multi-picker") {
		t.Fatal("사이트 다중 선택 없음")
	}
	if !strings.Contains(body, "multi_site_mode") {
		t.Fatal("처리 방식 라디오 없음")
	}
}
