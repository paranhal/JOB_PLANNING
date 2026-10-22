package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func postWGFetch(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMETextHTML)
	req.Header.Set("X-Requested-With", "fetch")
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestWorkGroupPopupStayOpenFetchAndBulkMembers(t *testing.T) {
	e, repo, oa, tech, _, off := newWGServer(t)
	oaCK := jwtCookieUserClaims(t, oa)

	create := postForm947(t, e, "/work-groups", url.Values{"name": {"연속그룹"}}, oaCK)
	if create.Code != http.StatusSeeOther {
		t.Fatalf("fallback create %d", create.Code)
	}
	items, _ := repo.ListGroups(model.OrgIDLibrary, "")
	id := items[0].GroupID

	add := postWGFetch(t, e, "/work-groups/"+id+"/members", url.Values{
		"member_ids[]": {tech.UserID, off.UserID},
	}, oaCK)
	if add.Code != http.StatusOK {
		t.Fatalf("fetch add %d loc=%s body=%s", add.Code, add.Header().Get(echo.HeaderLocation), add.Body.String())
	}
	if add.Header().Get(echo.HeaderLocation) != "" {
		t.Fatal("fetch 추가가 리다이렉트됨")
	}
	body := add.Body.String()
	if !strings.Contains(body, "기술원") || !strings.Contains(body, "사무원") {
		t.Fatal("갱신 조각에 구성원 없음")
	}
	if !strings.Contains(body, "명을 추가했습니다") {
		t.Fatal("상태 메시지 없음")
	}
	if strings.Contains(body, `id="wg-add"`) {
		t.Fatal("단일 select 추가가 남아 있음")
	}
	if !strings.Contains(body, "선택한") || !strings.Contains(body, "member_ids") {
		t.Fatal("체크박스 일괄 추가 없음")
	}

	dup := postWGFetch(t, e, "/work-groups/"+id+"/members", url.Values{"member_ids[]": {tech.UserID}}, oaCK)
	if dup.Code != http.StatusOK {
		t.Fatalf("이미 소속 %d", dup.Code)
	}
	if !strings.Contains(dup.Body.String(), "이미") && !strings.Contains(dup.Body.String(), "건너") {
		t.Fatal("이미 소속 안내 없음")
	}

	lead := postWGFetch(t, e, "/work-groups/"+id+"/leader", url.Values{"user_id": {tech.UserID}}, oaCK)
	if lead.Code != http.StatusOK {
		t.Fatalf("fetch leader %d", lead.Code)
	}

	rm := postWGFetch(t, e, "/work-groups/"+id+"/members/"+off.UserID+"/remove", url.Values{}, oaCK)
	if rm.Code != http.StatusOK {
		t.Fatalf("fetch remove %d", rm.Code)
	}
	if strings.Contains(rm.Body.String(), `>사무원</td>`) || strings.Contains(rm.Body.String(), `<td class="p-2">사무원</td>`) {
		t.Fatal("제거 후에도 구성원 표시")
	}

	fb := postForm947(t, e, "/work-groups/"+id+"/members", url.Values{"user_id": {off.UserID}, "member_ids[]": {off.UserID}}, oaCK)
	if fb.Code != http.StatusSeeOther {
		t.Fatalf("폴백 303 아님 %d", fb.Code)
	}
}

func TestWorkGroupCreateFormMembersLeaderTxn(t *testing.T) {
	e, repo, oa, tech, _, off := newWGServer(t)
	oaCK := jwtCookieUserClaims(t, oa)

	form := doGetWith(t, e, "/work-groups/new", oaCK)
	if form.Code != http.StatusOK {
		t.Fatalf("new %d", form.Code)
	}
	nb := form.Body.String()
	for _, s := range []string{"member_ids", "leader_id", "start_date", "end_date"} {
		if !strings.Contains(nb, s) {
			t.Fatalf("생성 폼 필드 없음: %s", s)
		}
	}

	_ = postWGFetch(t, e, "/work-groups", url.Values{
		"name":         {"롤백그룹"},
		"member_ids[]": {tech.UserID},
		"leader_id":    {off.UserID},
	}, oaCK)
	items, _ := repo.ListGroups(model.OrgIDLibrary, "")
	for _, g := range items {
		if g.Name == "롤백그룹" {
			t.Fatal("실패 생성 롤백 안 됨")
		}
	}

	ok := postWGFetch(t, e, "/work-groups", url.Values{
		"name":         {"한거래그룹"},
		"purpose":      {"점검"},
		"start_date":   {"2026-01-01"},
		"end_date":     {"2026-12-31"},
		"member_ids[]": {tech.UserID, off.UserID},
		"leader_id":    {tech.UserID},
	}, oaCK)
	if ok.Code != http.StatusOK {
		t.Fatalf("create fetch %d %s", ok.Code, ok.Body.String())
	}
	if ok.Header().Get(echo.HeaderLocation) != "" {
		t.Fatal("생성 후 팝업이 닫히는 리다이렉트")
	}
	if !strings.Contains(ok.Body.String(), "한거래그룹") {
		t.Fatal("생성 후 상세 조각 아님")
	}
	got, _ := repo.ListGroups(model.OrgIDLibrary, "")
	var g *model.WorkGroup
	for i := range got {
		if got[i].Name == "한거래그룹" {
			g = &got[i]
		}
	}
	if g == nil {
		t.Fatal("그룹 없음")
	}
	ms, _ := repo.MembersOn(g.GroupID, "")
	if len(ms) != 2 {
		t.Fatalf("구성원 %d", len(ms))
	}
	if g.LeaderID != tech.UserID {
		t.Fatalf("그룹장 %s", g.LeaderID)
	}
}

func TestWorkGroupFormJSStayOpenAndCheckbox(t *testing.T) {
	root := findTemplateRoot(t)
	form, err := os.ReadFile(filepath.Join(root, "work_group", "_form.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(form)
	if strings.Contains(s, `id="wg-add"`) {
		t.Fatal("단일 select 구성원 추가")
	}
	if !strings.Contains(s, "member_ids") || !strings.Contains(s, `type="checkbox"`) {
		t.Fatal("체크박스 후보 없음")
	}
	if !strings.Contains(s, "그룹장으로 지정") {
		t.Fatal("행 단위 그룹장 단추 없음")
	}
	js, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "work-group.js"))
	if err != nil {
		t.Fatal(err)
	}
	jsS := string(js)
	if !strings.Contains(jsS, "X-Requested-With") || !strings.Contains(jsS, "fetch") {
		t.Fatal("폼 fetch 제출 없음")
	}
}

func findStaticRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "web", "static")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("static 없음")
	return ""
}
