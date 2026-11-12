package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_175C_ReportHTTP(t *testing.T) {
	e, repo, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"보고사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	bare := doGet(t, e, "/projects/"+id).Body.String()
	if strings.Contains(bare, "data-progress-report") {
		t.Fatal("구조 없는 사업에 진행 보고")
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "구축", Status: model.PhasePlanned}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	if _, err := repo.CreatePackage(phases[0].PhaseID, "현장", "site"); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/projects/"+id+"?report=week#project-phases").Body.String()
	if !strings.Contains(page, "data-progress-report") || !strings.Contains(page, "이번 주") || !strings.Contains(page, "data-copy-line") || !strings.Contains(page, "일 실적이 없습니다") {
		t.Fatal("진행 보고 박스")
	}
	if !strings.Contains(page, "data-milestone") {
		t.Fatal("마일스톤")
	}
	clevel := jwtCookieUser(t, &model.User{UserID: "cl175c", Username: "cl175c", Role: model.RoleCLevel, OrgID: model.OrgIDLibrary})
	deny := postPhase(t, e, "/projects/"+id+"/milestones", url.Values{"package_id": {"없음"}}, clevel)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("임원 마일스톤 %d", deny.Code)
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK || !strings.Contains(dash.Body.String(), "현재 단계") || !strings.Contains(dash.Body.String(), "단계 진행") {
		t.Fatalf("대시보드 %d", dash.Code)
	}
}
