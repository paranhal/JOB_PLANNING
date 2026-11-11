package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_174_ProjectLinkOpensModalAndPresetsProject(t *testing.T) {
	e, db, me, other, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "174.db"))
	ck := jwtCookieUser(t, me)
	projects := repository.NewProjectRepo(db)
	own := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "도서관사업174", Status: model.WBProjectActive, ProjectKind: "build", ProjectKindSet: 1}
	if err := projects.Create(own); err != nil {
		t.Fatal(err)
	}
	foreign := &model.WorkProject{OrgID: "O02", Name: "타조직사업174", Status: model.WBProjectActive, ProjectKind: "build", ProjectKindSet: 1}
	if err := projects.Create(foreign); err != nil {
		t.Fatal(err)
	}
	related := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "유관사업174", Status: model.WBProjectActive, ProjectKind: "construction", ProjectKindSet: 1}
	if err := projects.Create(related); err != nil {
		t.Fatal(err)
	}
	if err := projects.ReplaceRelatedOrgs(related.ProjectID, me.UserID, []string{"O02"}, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	ro := &model.User{Username: "ro174", PasswordHash: HashPassword("password10"), FullName: "읽기174", Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(ro); err != nil {
		t.Fatal(err)
	}
	roUser, _ := users.GetByUsername("ro174")

	wantHref := workboardNewWorkURL(model.WBWorkProjectWork, own.ProjectID)
	wantHTML := strings.ReplaceAll(wantHref, "&", "&amp;")
	page := doGetWith(t, e, "/projects/"+own.ProjectID, ck)
	if page.Code != http.StatusOK {
		t.Fatalf("사업 상세 %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, `href="`+wantHTML+`"`) {
		t.Fatalf("등록 링크가 모달 주소가 아니다: %s", wantHref)
	}
	if strings.Contains(body, "/workboard/new-work/form") {
		t.Fatal("사업 상세가 조각 주소를 가리킨다")
	}
	roPage := doGetWith(t, e, "/projects/"+own.ProjectID, jwtCookieUser(t, roUser))
	if roPage.Code != http.StatusOK {
		t.Fatalf("읽기 전용 상세 %d", roPage.Code)
	}
	if strings.Contains(roPage.Body.String(), "이 사업으로 업무 등록") || strings.Contains(roPage.Body.String(), "/workboard/new-work/form") {
		t.Fatal("읽기 전용에 등록 링크가 있다")
	}
	if !strings.Contains(roPage.Body.String(), "이 사업에 업무를 등록할 권한이 없습니다") {
		t.Fatal("읽기 전용 안내 문구가 없다")
	}
	otherPage := doGetWith(t, e, "/projects/"+own.ProjectID, jwtCookieUser(t, other))
	if otherPage.Code != http.StatusOK {
		t.Fatalf("타 조직 상세 %d", otherPage.Code)
	}
	if strings.Contains(otherPage.Body.String(), "이 사업으로 업무 등록") {
		t.Fatal("유관이 아닌 조직에 등록 링크가 있다")
	}
	if !strings.Contains(otherPage.Body.String(), "이 사업에 업무를 등록할 권한이 없습니다") {
		t.Fatal("타 조직 안내 문구가 없다")
	}
	relPage := doGetWith(t, e, "/projects/"+related.ProjectID, jwtCookieUser(t, other))
	if !strings.Contains(relPage.Body.String(), `href="`+strings.ReplaceAll(workboardNewWorkURL(model.WBWorkProjectWork, related.ProjectID), "&", "&amp;")+`"`) {
		t.Fatal("유관 조직에 등록 링크가 없다")
	}

	form := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&project_id="+own.ProjectID, ck)
	if form.Code != http.StatusOK {
		t.Fatalf("폼 %d %s", form.Code, form.Body.String())
	}
	formBody := form.Body.String()
	if !strings.Contains(formBody, "도서관사업174") || !strings.Contains(formBody, `value="`+own.ProjectID+`"`) {
		t.Fatal("권한 있는 사업이 미리 선택되지 않았다")
	}
	if !strings.Contains(formBody, "project-picked") || !strings.Contains(formBody, "project_kind") || !strings.Contains(formBody, "DispatchSelected") {
		t.Fatal("선택 사업의 구분 제안 이벤트가 없다")
	}
	if !strings.Contains(formBody, `name="redirect" value="/workboard/board"`) {
		t.Fatal("저장 후 복귀가 일정표가 아니다")
	}
	if strings.Contains(formBody, `/projects/`+own.ProjectID) {
		t.Fatal("허용되지 않은 사업 상세 복귀가 들어 있다")
	}

	denied := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&project_id="+foreign.ProjectID, ck)
	if denied.Code != http.StatusOK {
		t.Fatalf("권한 없는 사업 폼 %d", denied.Code)
	}
	if strings.Contains(denied.Body.String(), `"Selected":"`+foreign.ProjectID+`"`) || strings.Contains(denied.Body.String(), "타조직사업174") {
		t.Fatal("권한 없는 사업이 선택되었다")
	}
	missing := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&project_id=WP-NONE", ck)
	if missing.Code != http.StatusOK || strings.Contains(missing.Body.String(), `"Selected":"WP-NONE"`) {
		t.Fatal("없는 사업이 선택되거나 폼이 깨졌다")
	}
	bad := doGetWith(t, e, "/workboard/new-work/form?kind=nope&project_id="+own.ProjectID, ck)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("잘못된 kind %d", bad.Code)
	}

	raw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/workboard/new-work/form?kind=project_work&project_id="+own.ProjectID, nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(ck)
	e.ServeHTTP(raw, req)
	if raw.Code != http.StatusSeeOther {
		t.Fatalf("직접 열기 %d", raw.Code)
	}
	if raw.Header().Get("Location") != wantHref {
		t.Fatalf("직접 열기 Location=%s", raw.Header().Get("Location"))
	}

	home := doGetWith(t, e, wantHref, ck)
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/workboard/board?new=1&kind=project_work&project_id="+own.ProjectID {
		t.Fatalf("홈 전달 %d %s", home.Code, home.Header().Get("Location"))
	}
	board := doGetWith(t, e, home.Header().Get("Location"), ck)
	if board.Code != http.StatusOK {
		t.Fatalf("일정표 %d", board.Code)
	}
	boardBody := board.Body.String()
	for _, s := range []string{
		`rel="stylesheet"`, "maybeOpenNewFromQuery", "project_work", "project_id", "<aside",
	} {
		if !strings.Contains(boardBody, s) {
			t.Fatalf("일정표에 %q 없음", s)
		}
	}
	if got := safeWorkboardRedirect("/projects/"+own.ProjectID, "/workboard/board"); got != "/workboard/board" {
		t.Fatalf("사업 상세 redirect 가 허용됨: %s", got)
	}
}
