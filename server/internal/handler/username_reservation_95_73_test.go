package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func Test95_73_ReservationDialogReleaseAndWsg(t *testing.T) {
	e, users, acc, _ := v68App(t)
	vs := &model.User{Username: "vs73", PasswordHash: HashPassword("password10"), FullName: "비젼73", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa73", PasswordHash: HashPassword("password10"), FullName: "조직장73", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	owner := &model.User{Username: "fa_admin", PasswordHash: HashPassword("password10"), FullName: "경영지원팀관리자", Role: model.RoleOffice, IsActive: true, OrgID: "O01", ProfileDone: true}
	tech := &model.User{Username: "wsg1", PasswordHash: HashPassword("password10"), FullName: "우섬길", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true, Permissions: model.PermASProcess}
	for _, u := range []*model.User{vs, oa, owner, tech} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := acc.Reserve("wsg", owner.UserID, model.ReserveRenamedFrom); err != nil {
		t.Fatal(err)
	}

	vsCK := visionCookie(t, vs)
	oaCK := jwtOrgCookie(t, oa)
	formCT := echo.HeaderContentType
	mime := echo.MIMEApplicationForm

	ren := httptest.NewRecorder()
	rreq := httptest.NewRequest(http.MethodPost, "/users/"+tech.UserID+"/username", strings.NewReader(url.Values{"username": {"wsg"}}.Encode()))
	rreq.Header.Set(formCT, mime)
	rreq.AddCookie(vsCK)
	e.ServeHTTP(ren, rreq)
	loc := ren.Header().Get("Location")
	if ren.Code != http.StatusSeeOther || !strings.Contains(loc, "dup=renamed") || !strings.Contains(loc, "username=wsg") || !strings.Contains(loc, tech.UserID) {
		t.Fatalf("9번 팝업 loc=%s", loc)
	}

	page := doGetWith(t, e, "/admin/users?dup=renamed&username=wsg&apply_user_id="+tech.UserID+"&return=/admin/users", vsCK)
	if page.Code != http.StatusOK {
		t.Fatalf("dialog %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "dup-username-title") || !strings.Contains(body, "autofocus") || !strings.Contains(body, "keydown.escape") {
		t.Fatal("팝업·포커스·Esc 없음")
	}
	if !strings.Contains(body, "해제하고 변경") || !strings.Contains(body, "닫기") || !strings.Contains(body, "경영지원팀관리자") || !strings.Contains(body, "fa_admin") {
		t.Fatal("비전 안내 부족")
	}
	if !strings.Contains(body, "/admin/users/reservations/release") {
		t.Fatal("해제 경로 없음")
	}

	oaPage := doGetWith(t, e, "/admin/my-org/members?dup=renamed&username=wsg&return=/admin/my-org/members", oaCK)
	if oaPage.Code != http.StatusOK {
		t.Fatalf("oa %d", oaPage.Code)
	}
	ob := oaPage.Body.String()
	if !strings.Contains(ob, model.UsernameInquiryMsg) {
		t.Fatal("조직관리자 공개 문구 없음")
	}
	if strings.Contains(ob, "아이디를 변경하면서") || strings.Contains(ob, "/admin/users/reservations/release") || strings.Contains(ob, "해제하고 변경") {
		t.Fatal("조직관리자에게 소유자·해제가 보인다")
	}

	oaRel := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/admin/users/reservations/release", strings.NewReader(url.Values{
		"username": {"wsg"}, "reason": {"무단"}, "vision_password": {"vision-lock1"},
	}.Encode()))
	oreq.Header.Set(formCT, mime)
	oreq.AddCookie(oaCK)
	e.ServeHTTP(oaRel, oreq)
	if oaRel.Code == http.StatusSeeOther && strings.Contains(oaRel.Header().Get("Location"), "ok=") {
		t.Fatal("조직관리자가 해제했다")
	}

	curDeny := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodPost, "/admin/users/reservations/release", strings.NewReader(url.Values{
		"username": {"fa_admin"}, "reason": {"잘못"}, "vision_password": {"vision-lock1"}, "return": {"/admin/users"},
	}.Encode()))
	creq.Header.Set(formCT, mime)
	creq.AddCookie(vsCK)
	e.ServeHTTP(curDeny, creq)
	if curDeny.Code != http.StatusSeeOther || !strings.Contains(curDeny.Header().Get("Location"), "err=") {
		t.Fatalf("current 해제 loc=%s", curDeny.Header().Get("Location"))
	}

	badRet := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/admin/users/reservations/release", strings.NewReader(url.Values{
		"username": {"wsg"}, "reason": {"우섬길 실사용"}, "vision_password": {"vision-lock1"},
		"apply_user_id": {tech.UserID}, "return": {"https://evil.example/"}, "dup": {"renamed"},
	}.Encode()))
	breq.Header.Set(formCT, mime)
	breq.AddCookie(vsCK)
	e.ServeHTTP(badRet, breq)
	bloc := badRet.Header().Get("Location")
	if badRet.Code != http.StatusSeeOther || !strings.Contains(bloc, "/admin/users/deleted") || !strings.Contains(bloc, "ok=") {
		t.Fatalf("return 허용 목록 loc=%s", bloc)
	}
	back, _ := users.GetByID(tech.UserID)
	if back == nil || back.Username != "wsg" || back.Permissions != model.PermASProcess {
		t.Fatalf("변경 후 %+v", back)
	}

	tab := doGetWith(t, e, "/admin/users/deleted?tab=reservations", vsCK)
	if tab.Code != http.StatusOK || !strings.Contains(tab.Body.String(), "예약된 아이디") || !strings.Contains(tab.Body.String(), "wsg1") {
		t.Fatalf("탭 %d %s", tab.Code, tab.Body.String())
	}
}
