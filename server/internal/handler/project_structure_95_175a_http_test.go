package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_175A_StructureHTTP(t *testing.T) {
	t.Setenv("VIEW_AS_ENABLED", "1")
	e, repo, _, users := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"구조사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	bare := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(bare, "수행 구조") || !strings.Contains(bare, "data-site-progress-tab") || strings.Contains(bare, "data-package-id") || strings.Contains(bare, "미분류 작업") {
		t.Fatal("구조 없는 사업에 하부 업무 줄이 있다")
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "구축", Status: model.PhasePlanned}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	pkg, err := repo.CreatePackage(phases[0].PhaseID, "현장", "site")
	if err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, "현장") || !strings.Contains(page, "계획 미배정") || !strings.Contains(page, "미분류 작업") || !strings.Contains(page, "data-plan-form") {
		t.Fatal("수행 구조 트리")
	}
	opt := doGet(t, e, "/projects/"+id+"/structure/options")
	if opt.Code != http.StatusOK || !strings.Contains(opt.Body.String(), `"has":true`) {
		t.Fatalf("옵션 %d %s", opt.Code, opt.Body.String())
	}
	obs := jwtCookieUser(t, &model.User{UserID: "obs175", Username: "obs175", Role: model.RoleObserver, IsReadOnly: true, OrgID: model.OrgIDLibrary})
	deny := postPhase(t, e, "/projects/"+id+"/plans", url.Values{"package_id": {pkg}, "name": {"금지"}}, obs)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("옵저버 %d", deny.Code)
	}
	viewed := &model.User{Username: "view175", PasswordHash: "x", FullName: "열람", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(viewed); err != nil {
		t.Fatal(err)
	}
	view := postPhase(t, e, "/projects/"+id+"/plans", url.Values{"package_id": {pkg}, "name": {"열람계획"}}, jwtCookie(t), &http.Cookie{Name: viewAsCookie, Value: viewed.UserID, Path: "/"})
	if view.Code != http.StatusForbidden {
		t.Fatalf("view-as %d", view.Code)
	}
	other := &model.WorkProject{Name: "타조직175", Status: "active", OrgID: "O02"}
	if err := repo.Create(other); err != nil {
		t.Fatal(err)
	}
	got := doForm(t, e, "/projects/"+other.ProjectID+"/plans", url.Values{"package_id": {pkg}, "name": {"타조직계획"}})
	if got.Code != http.StatusNotFound && got.Code != http.StatusForbidden {
		t.Fatalf("다른 조직 %d", got.Code)
	}

	wb, _ := newWorkboardServer(t, "wb175.db")
	reg := doGet(t, wb, "/workboard/register")
	if reg.Code != http.StatusOK || strings.Contains(reg.Body.String(), "수행 단계") {
		t.Fatal("구조 없는 등록 폼에 수행 단계")
	}
}
