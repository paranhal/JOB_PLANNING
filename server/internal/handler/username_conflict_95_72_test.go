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

func Test95_72_SafeUsernameDupReturnAllowlist(t *testing.T) {
	if got := safeUsernameDupReturn(""); got != usernameDupDefaultReturn {
		t.Fatalf("empty=%s", got)
	}
	if got := safeUsernameDupReturn("/admin/users"); got != "/admin/users" {
		t.Fatalf("users=%s", got)
	}
	if got := safeUsernameDupReturn("/account"); got != "/account" {
		t.Fatalf("account=%s", got)
	}
	if got := safeUsernameDupReturn("/as"); got != usernameDupDefaultReturn {
		t.Fatalf("as=%s", got)
	}
	if got := safeUsernameDupReturn("https://evil.example/admin/users"); got != usernameDupDefaultReturn {
		t.Fatalf("abs=%s", got)
	}
	if got := safeUsernameDupReturn("//evil/admin/users"); got != usernameDupDefaultReturn {
		t.Fatalf("proto=%s", got)
	}
	if got := safeUsernameDupReturn("/admin/users?next=https://x"); got != "/admin/users" {
		t.Fatalf("query stripped=%s", got)
	}
}

func Test95_72_UsernameConflictKindsAndDialog(t *testing.T) {
	e, users, acc, _ := v68App(t)
	vs := &model.User{Username: "vs72", PasswordHash: HashPassword("password10"), FullName: "비젼72", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa72", PasswordHash: HashPassword("password10"), FullName: "조직장72", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	gone := &model.User{Username: "dupgone72", PasswordHash: HashPassword("password10"), FullName: "삭제대상72", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true, Mobile: "010-9999-0000"}
	live := &model.User{Username: "duplive72", PasswordHash: HashPassword("password10"), FullName: "사용중72", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	ren := &model.User{Username: "old72http", PasswordHash: HashPassword("password10"), FullName: "변경72", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	for _, u := range []*model.User{vs, oa, gone, live, ren} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := users.SoftDelete(gone.UserID, vs.UserID, "퇴사72"); err != nil {
		t.Fatal(err)
	}
	if err := users.RenameUsername(ren.UserID, "new72http"); err != nil {
		t.Fatal(err)
	}
	if err := acc.Reserve("hold72http", "U-HOLD", "manual"); err != nil {
		t.Fatal(err)
	}

	vsCK := visionCookie(t, vs)
	oaCK := jwtOrgCookie(t, oa)
	formCT := echo.HeaderContentType
	mime := echo.MIMEApplicationForm

	postUsers := func(name string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(url.Values{
			"username": {name}, "full_name": {"새사람"}, "password": {"password10"}, "role": {model.RoleTech},
		}.Encode()))
		req.Header.Set(formCT, mime)
		req.AddCookie(vsCK)
		e.ServeHTTP(rec, req)
		return rec
	}

	createGone := postUsers("dupgone72")
	loc := createGone.Header().Get("Location")
	if createGone.Code != http.StatusSeeOther || !strings.Contains(loc, "dup=deleted") || !strings.Contains(loc, gone.UserID) {
		t.Fatalf("삭제 중복 loc=%s code=%d", loc, createGone.Code)
	}

	liveDeny := postUsers("duplive72")
	lloc := liveDeny.Header().Get("Location")
	if !strings.Contains(lloc, "err=") || strings.Contains(lloc, "dup=deleted") {
		t.Fatalf("사용 중 loc=%s", lloc)
	}
	if locUnesc, _ := url.QueryUnescape(lloc); !strings.Contains(locUnesc, "이미 사용 중인 아이디입니다") {
		t.Fatalf("사용 중 문구 loc=%s", lloc)
	}

	renDeny := postUsers("old72http")
	rloc := renDeny.Header().Get("Location")
	if !strings.Contains(rloc, "dup=renamed") || strings.Contains(rloc, "dup=deleted") {
		t.Fatalf("이름 변경 팝업 loc=%s", rloc)
	}

	holdDeny := postUsers("hold72http")
	hloc := holdDeny.Header().Get("Location")
	if !strings.Contains(hloc, "dup=") || strings.Contains(hloc, "dup=deleted") {
		t.Fatalf("예약 팝업 loc=%s", hloc)
	}

	adminUsers := doGetWith(t, e, "/admin/users?dup=deleted&id="+gone.UserID+"&return=/admin/users", vsCK)
	if adminUsers.Code != http.StatusOK {
		t.Fatalf("users %d", adminUsers.Code)
	}
	body := adminUsers.Body.String()
	if !strings.Contains(body, "dup-username-title") || !strings.Contains(body, "복구") || !strings.Contains(body, "닫기") {
		t.Fatal("비젼 팝업에 복구/닫기 없음")
	}
	if !strings.Contains(body, "삭제대상72") || !strings.Contains(body, "010-9999-0000") {
		t.Fatal("비젼에게 개인정보가 없다")
	}

	oaPage := doGetWith(t, e, "/admin/my-org/members?dup=deleted&id="+gone.UserID+"&return=/admin/my-org/members", oaCK)
	if oaPage.Code != http.StatusOK {
		t.Fatalf("members %d", oaPage.Code)
	}
	ob := oaPage.Body.String()
	if !strings.Contains(ob, "dup-username-title") || !strings.Contains(ob, "닫기") {
		t.Fatal("조직관리자 안내 팝업 없음")
	}
	if strings.Contains(ob, "삭제대상72") || strings.Contains(ob, "010-9999-0000") {
		t.Fatal("조직관리자에게 개인정보가 보인다")
	}
	if strings.Contains(ob, `action="/admin/users/deleted/`+gone.UserID+`/restore"`) {
		t.Fatal("조직관리자에게 복구 폼이 있다")
	}

	oaRestore := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+gone.UserID+"/restore", strings.NewReader(url.Values{
		"vision_password": {"vision-lock1"}, "reason": {"재입사"}, "job": {"tech"}, "org_id": {"O01"},
	}.Encode()))
	oreq.Header.Set(formCT, mime)
	oreq.AddCookie(oaCK)
	e.ServeHTTP(oaRestore, oreq)
	if oaRestore.Code == http.StatusSeeOther && strings.Contains(oaRestore.Header().Get("Location"), "ok=") {
		t.Fatal("조직관리자가 복구했다")
	}
	if oaRestore.Code != http.StatusForbidden && oaRestore.Code != http.StatusOK {
		t.Fatalf("권한 없음 code=%d loc=%s", oaRestore.Code, oaRestore.Header().Get("Location"))
	}

	gone2 := &model.User{Username: "dupgone72b", PasswordHash: HashPassword("password10"), FullName: "두번째72", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	if err := users.Create(gone2); err != nil {
		t.Fatal(err)
	}
	if err := users.SoftDelete(gone2.UserID, vs.UserID, "퇴사"); err != nil {
		t.Fatal(err)
	}

	failPW := httptest.NewRecorder()
	fpw := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+gone2.UserID+"/restore", strings.NewReader(url.Values{
		"vision_password": {"wrong-lock"}, "reason": {"재입사"}, "job": {"tech"}, "org_id": {"O01"}, "return": {"/admin/users"},
	}.Encode()))
	fpw.Header.Set(formCT, mime)
	fpw.AddCookie(vsCK)
	e.ServeHTTP(failPW, fpw)
	floc := failPW.Header().Get("Location")
	if failPW.Code != http.StatusSeeOther || !strings.Contains(floc, "err=") || !strings.Contains(floc, "dup=deleted") {
		t.Fatalf("비밀번호 실패 loc=%s", floc)
	}

	failReason := httptest.NewRecorder()
	fr := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+gone2.UserID+"/restore", strings.NewReader(url.Values{
		"vision_password": {"vision-lock1"}, "job": {"tech"}, "org_id": {"O01"}, "return": {"/admin/users"},
	}.Encode()))
	fr.Header.Set(formCT, mime)
	fr.AddCookie(vsCK)
	e.ServeHTTP(failReason, fr)
	if failReason.Code != http.StatusSeeOther || !strings.Contains(failReason.Header().Get("Location"), "err=") {
		t.Fatalf("사유 없음 loc=%s", failReason.Header().Get("Location"))
	}

	badRet := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+gone.UserID+"/restore", strings.NewReader(url.Values{
		"vision_password": {"vision-lock1"}, "reason": {"재입사"}, "job": {"support"}, "org_id": {"O01"},
		"return": {"https://evil.example/"},
	}.Encode()))
	breq.Header.Set(formCT, mime)
	breq.AddCookie(vsCK)
	e.ServeHTTP(badRet, breq)
	bloc := badRet.Header().Get("Location")
	if badRet.Code != http.StatusSeeOther || !strings.Contains(bloc, "/admin/users/deleted") || !strings.Contains(bloc, "ok=") {
		t.Fatalf("허용 목록 외 return loc=%s", bloc)
	}
	back, _ := users.GetByID(gone.UserID)
	if back == nil || back.IsDeleted() {
		t.Fatal("복구 실패")
	}

	okRet := httptest.NewRecorder()
	okreq := httptest.NewRequest(http.MethodPost, "/admin/users/deleted/"+gone2.UserID+"/restore", strings.NewReader(url.Values{
		"vision_password": {"vision-lock1"}, "reason": {"재입사"}, "job": {"tech"}, "org_id": {"O01"},
		"return": {"/admin/users"},
	}.Encode()))
	okreq.Header.Set(formCT, mime)
	okreq.AddCookie(vsCK)
	e.ServeHTTP(okRet, okreq)
	oloc := okRet.Header().Get("Location")
	if okRet.Code != http.StatusSeeOther || !strings.HasPrefix(oloc, "/admin/users?") || !strings.Contains(oloc, "ok=") {
		t.Fatalf("허용 return loc=%s", oloc)
	}
}
