package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_100_ShowNotEmpty(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{OrgID: model.OrgIDLibrary, Name: "빈화면점검", ShortName: "빈점", Status: model.WBProjectActive, ProjectKind: "maintenance", ProjectKindSet: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "?ok=created", "?ok=updated"} {
		rec := doGet(t, e, "/projects/"+p.ProjectID+q)
		if rec.Code != http.StatusOK {
			t.Fatalf("상세%s status=%d", q, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "기본 정보") || !strings.Contains(body, "빈화면점검") {
			t.Fatalf("상세%s 본문이 비었다", q)
		}
		if !strings.Contains(body, "영업 없이 직접 등록") {
			t.Fatalf("상세%s 파이프라인 미연결 표시 없음", q)
		}
	}
}

func Test95_100_DirectRegisterRenderAndPost(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	form := doGet(t, e, "/projects/new")
	if form.Code != http.StatusOK {
		t.Fatalf("GET /projects/new %d loc=%s", form.Code, form.Header().Get("Location"))
	}
	if strings.Contains(form.Header().Get("Location"), "/sales/new") {
		t.Fatal("영업 등록으로 보냄")
	}
	if !strings.Contains(form.Body.String(), "사업 등록") {
		t.Fatal("직접 등록 폼이 아니다")
	}
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"직접등록파이프"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"other"},
		"org_id": {model.OrgIDLibrary}, "contract_amount": {"1500000"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=created") {
		t.Fatalf("POST 직접 등록 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	got, err := repo.Get(id)
	if err != nil || got == nil || got.SalesProjectID != "" {
		t.Fatalf("영업 없이 저장되지 않음 %+v err=%v", got, err)
	}
	show := doGet(t, e, "/projects/"+id+"?ok=created")
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "기본 정보") {
		t.Fatalf("등록 직후 상세 빈 화면 %d", show.Code)
	}
	reg := doGet(t, e, "/projects/registry")
	if !strings.Contains(reg.Body.String(), "직접등록파이프") || !strings.Contains(reg.Body.String(), "직접 등록") {
		t.Fatal("등록 관리에 직접 등록 건이 없다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK || !strings.Contains(dash.Body.String(), "직접등록파이프") {
		t.Fatalf("대시보드(파이프라인)에 안 보임 %d", dash.Code)
	}
}

func Test95_100_EditOrgErrorRenderAndPost(t *testing.T) {
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
		t.Fatalf("org_id 없는 POST status=%d body=%s", rec.Code, clipBody(rec.Body.String()))
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
	if !strings.Contains(bad.Body.String(), "id=\"form-error\"") && !strings.Contains(bad.Body.String(), "주관") {
		t.Fatalf("조직 오류가 폼에 안 보인다: %s", clipBody(bad.Body.String()))
	}
}
