package handler

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_100_T1_ShowFlashAndBody(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "T1본문사업", Status: model.WBProjectActive, ProjectKind: "maintenance", ProjectKindSet: 1, ContractAmount: 1_000_000}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "?ok=created", "?ok=updated", "?ok=moved", "?ok=promoted"} {
		rec := doGet(t, e, "/projects/"+p.ProjectID+q)
		if rec.Code != http.StatusOK {
			t.Fatalf("T1 상세%s status=%d", q, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "T1본문사업") || !strings.Contains(body, "기본 정보") {
			t.Fatalf("T1 상세%s 본문 없음", q)
		}
	}
}

func Test95_100_T2_EditOrgErrorRenderAndPost(t *testing.T) {
	e, _, repo, ck := newProjectOrgFilterHTTP(t)
	p := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "조직수정점검", Status: model.WBProjectActive, ProjectKind: "maintenance", ProjectKindSet: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/projects/"+p.ProjectID+"/edit", ck.oa01)
	if page.Code != http.StatusOK {
		t.Fatalf("수정 렌더 %d", page.Code)
	}
	if strings.Contains(page.Body.String(), "소속 조직을 선택하세요") {
		t.Fatal("수정 화면이 조직 오류로 열린다")
	}
	rec := doFormWith(t, e, "/projects/"+p.ProjectID, url.Values{
		"name": {"조직수정점검2"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"maintenance"},
	}, ck.oa01)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("org_id 없는 POST status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "소속 조직을 선택하세요") {
		t.Fatal("수정 POST 가 조직 오류 화면이다")
	}
	got, err := repo.Get(p.ProjectID)
	if err != nil || got.OrgID != model.OrgIDLibrary || got.Name != "조직수정점검2" {
		t.Fatalf("조직이 깨지거나 이름이 안 남음 %+v err=%v", got, err)
	}
	bad := doFormWith(t, e, "/projects/"+p.ProjectID, url.Values{
		"name": {"조직수정점검3"}, "status": {"active"}, "org_id": {model.OrgIDLibrary}, "related_org_ids": {model.OrgIDLibrary},
		"project_kind": {"maintenance"},
	}, ck.oa01)
	if bad.Code != http.StatusOK {
		t.Fatalf("유관=주관 거절은 폼으로 돌아가야 함 %d", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), `id="form-error"`) && !strings.Contains(bad.Body.String(), "주관") {
		t.Fatalf("조직 오류가 폼에 안 보인다")
	}
}

func Test95_100_T3_EditPermissionMatrix(t *testing.T) {
	e, _, repo, ck := newProjectOrgFilterHTTP(t)
	p := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "T3권한사업", Status: model.WBProjectActive, ProjectKind: "maintenance", ProjectKindSet: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	path := "/projects/" + p.ProjectID + "/edit"
	if rec := doGetWith(t, e, path, ck.oa01); rec.Code != http.StatusOK {
		t.Fatalf("주관 부서 수정 %d", rec.Code)
	}
	if rec := doGetWith(t, e, path, ck.oa02); rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 수정 want 403 got %d", rec.Code)
	}
	if rec := doGetWith(t, e, path, ck.vs); rec.Code != http.StatusOK {
		t.Fatalf("비전관리자 수정 %d", rec.Code)
	}
	if rec := doGetWith(t, e, path, ck.cl); rec.Code != http.StatusForbidden {
		t.Fatalf("C-Level 수정 want 403 got %d", rec.Code)
	}
}

func TestProjectShowRenderRolesAndListRows(t *testing.T) {
	e, db, repo, ck := newProjectOrgFilterHTTP(t)

	direct := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "직접등록상세", Status: model.WBProjectActive, ProjectKind: "other", ProjectKindSet: 1, ContractAmount: 1_000_000}
	if err := repo.Create(direct); err != nil {
		t.Fatal(err)
	}
	linked := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "영업연결상세", Status: model.WBProjectActive, ProjectKind: "other", ProjectKindSet: 1, ContractAmount: 1_000_000}
	if err := repo.Create(linked); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sales_projects (sales_id, name, stage, status, org_id) VALUES ('SP-HOTFIX','영업연결원본','closed','promoted',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET sales_project_id='SP-HOTFIX' WHERE project_id=?`, linked.ProjectID); err != nil {
		t.Fatal(err)
	}
	missing := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "영업결번상세", Status: model.WBProjectActive, ProjectKind: "other", ProjectKindSet: 1, ContractAmount: 1_000_000}
	if err := repo.Create(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET sales_project_id='SP-GONE' WHERE project_id=?`, missing.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRelatedOrgs(direct.ProjectID, "oa01-2d", []string{"O03"}, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}

	assertShow := func(t *testing.T, path, name string, cookie *http.Cookie, want, notWant []string) {
		t.Helper()
		rec := doGetWith(t, e, path, cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, name) {
			t.Fatalf("%s 사업명 없음", path)
		}
		for _, s := range want {
			if !strings.Contains(body, s) {
				t.Fatalf("%s %q 없음", path, s)
			}
		}
		for _, s := range notWant {
			if strings.Contains(body, s) {
				t.Fatalf("%s %q 가 보이면 안 됨", path, s)
			}
		}
	}

	workBtns := []string{"일일 업무 현황", "이 사업으로 업무 등록"}
	noReg := []string{"일일 업무 현황", "이 사업에 업무를 등록할 권한이 없습니다"}
	assertShow(t, "/projects/"+linked.ProjectID, "영업연결상세", ck.oa01, append(workBtns, "SP-HOTFIX", `href="/sales/SP-HOTFIX"`), nil)
	assertShow(t, "/projects/"+direct.ProjectID, "직접등록상세", ck.oa01, append(workBtns, "영업 없이 직접 등록 · 파이프라인 미연결", "수정"), nil)
	assertShow(t, "/projects/"+missing.ProjectID, "영업결번상세", ck.oa01, []string{"연결 번호는 있으나 영업 건이 없습니다"}, nil)

	assertShow(t, "/projects/"+direct.ProjectID, "직접등록상세", ck.oa03, append(workBtns, "유관"), []string{`href="/projects/` + direct.ProjectID + `/edit"`})
	assertShow(t, "/projects/"+direct.ProjectID, "직접등록상세", ck.cl, noReg, []string{`href="/projects/` + direct.ProjectID + `/edit"`, "이 사업으로 업무 등록"})
	assertShow(t, "/projects/"+direct.ProjectID, "직접등록상세", ck.vs, append(workBtns, "수정"), nil)
	assertShow(t, "/projects/"+direct.ProjectID, "직접등록상세", ck.oa02, noReg, []string{`href="/projects/` + direct.ProjectID + `/edit"`, "이 사업으로 업무 등록"})

	list := doGetWith(t, e, "/projects/registry", ck.vs)
	if list.Code != http.StatusOK {
		t.Fatalf("목록 %d", list.Code)
	}
	ids := uniqueProjectIDs(list.Body.String())
	if len(ids) == 0 {
		t.Fatal("목록에 사업 행이 없다")
	}
	for _, id := range ids {
		rec := doGetWith(t, e, "/projects/"+id, ck.vs)
		if rec.Code != http.StatusOK {
			t.Fatalf("목록 행 GET /projects/%s status=%d", id, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), id) {
			t.Fatalf("/projects/%s 본문에 사업번호 없음", id)
		}
	}
}

var projectShowIDRe = regexp.MustCompile(`/projects/(WP-[0-9]+)`)

func uniqueProjectIDs(html string) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, m := range projectShowIDRe.FindAllStringSubmatch(html, -1) {
		id := m[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
